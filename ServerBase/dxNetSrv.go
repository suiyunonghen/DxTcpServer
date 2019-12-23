package ServerBase

import (
	"bytes"
	"encoding/binary"
	"github.com/suiyunonghen/DxCommonLib"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)


type DxTcpServer struct {
	DxCommonLib.GDxBaseObject
	listener        		net.Listener
	encoder					IConCoder
	isActivetag				int32
	OnRecvData				GOnRecvDataEvent
	OnClientConnect			func(con *DxNetConnection)interface{}
	OnClientDisConnected	GConnectEvent
	OnSrvClose				func()
	AfterClientDisConnected	GConnectEvent
	OnSendData				GOnSendDataEvent
	AfterEncodeData			GOnSendDataEvent
	TimeOutSeconds			int32
	curidx					uint
	clients 				map[uint]*DxNetConnection
	RequestCount			uint64
	SendRequestCount		uint64
	LimitSendPkgCount		uint8  //每个连接限制的发送包的个数，防止发送过快
	SendDataSize	 		DxDiskSize
	RecvDataSize			DxDiskSize
	MaxDataBufCount			uint16		//最大缓存数量
	SyncSendData			bool
	SrvLogger				LoggerInterface
	bufferPool				sync.Pool
	srvCloseChan			chan struct{}
	waitg					sync.WaitGroup
	BeforeRead				GConReadEvent
	AfterRead				GConReadEvent
	sync.RWMutex
}
type GIterateClientFunc func(con *DxNetConnection)bool
func (srv *DxTcpServer)Open(addr string) error {
	ls, err := net.Listen("tcp", addr)
	if nil != err {
		return err
	}
	srv.listener = ls
	srv.isActivetag = 1
	DxCommonLib.Post(srv)
	return nil
}

func (srv *DxTcpServer)BeforePackageRead(con *DxNetConnection)(error)  {
	if srv.BeforeRead!=nil{
		return srv.BeforeRead(con)
	}
	return nil
}

func (srv *DxTcpServer)AfterPackageRead(con *DxNetConnection)(error)  {
	if srv.AfterRead!=nil{
		return srv.AfterRead(con)
	}
	return nil
}


func (srv *DxTcpServer)GetCoder()IConCoder{
	return srv.encoder
}

func (srv *DxTcpServer) SubInit() {
	srv.GDxBaseObject.SubInit(srv)
}

func (srv *DxTcpServer)Close()  {
	if !atomic.CompareAndSwapInt32(&srv.isActivetag,1,0){
		return
	}
	if srv.OnSrvClose != nil{
		srv.OnSrvClose()
	}
	if nil != srv.listener {
		srv.listener.Close()
	}
	close(srv.srvCloseChan)
	srv.waitg.Wait()
	srv.clients = nil
}

func (srv *DxTcpServer)Logger()LoggerInterface  {
	return srv.SrvLogger
}


func (srv *DxTcpServer)AddRecvDataLen(datalen uint32){
	srv.Lock()
	srv.RecvDataSize.AddByteSize(datalen)
	srv.Unlock()
}

func (srv *DxTcpServer)AddSendDataLen(datalen uint32){
	srv.Lock()
	srv.SendDataSize.AddByteSize(datalen)
	srv.Unlock()
}

func (srv *DxTcpServer) Done()<-chan struct{}  {
	return srv.srvCloseChan
}

func (srv *DxTcpServer)CustomRead(con *DxNetConnection,targetData interface{})bool  {
	return false
}

func (srv *DxTcpServer)AfterDisConnected(con *DxNetConnection)   {
	if srv.AfterClientDisConnected!=nil{
		srv.AfterClientDisConnected(con)
	}
}

func (srv *DxTcpServer)Run()  {
	srv.srvCloseChan = make(chan struct{})
	var host IConHost
	if lastedHost := srv.LastedSubChild();lastedHost != nil{
		if Ahost,ok := lastedHost.(IConHost);!ok{
			host = srv
		}else{
			host = Ahost
		}
	}else{
		host = srv
	}
	for{
		conn, err := srv.listener.Accept()
		if err != nil {
			srv.listener = nil
			return
		}
		srv.waitg.Add(1)
		dxcon := GetConnection()
		dxcon.con = conn
		atomic.StoreInt32(&dxcon.unActive,0)
		dxcon.selfcancelchan = make(chan struct{})
		dxcon.LimitSendPkgCout = srv.LimitSendPkgCount
		dxcon.LoginTime = time.Now() //登录时间
		dxcon.ConHandle = uint(uintptr(unsafe.Pointer(dxcon)))
		dxcon.conHost = host
		if srv.clients == nil{
			srv.clients = make(map[uint]*DxNetConnection,100)
		}
		srv.Lock()
		srv.clients[dxcon.ConHandle] = dxcon
		srv.Unlock()
		dxcon.protocol = nil
		if srv.encoder != nil{
			if protocol,ok := srv.encoder.(IProtocol);ok{
				dxcon.protocol = protocol
			}
		}
		dxcon.init()
		result := srv.HandleConnectEvent(dxcon)
		DxCommonLib.PostFunc(dxcon.run,result)//连接开始执行接收消息和发送消息的处理线程
	}
}

func (srv *DxTcpServer)HandleDisConnectEvent(con *DxNetConnection) {
	if srv.OnClientDisConnected != nil{
		srv.OnClientDisConnected(con)
	}
	srv.Lock()
	con.SetUseData(nil)
	delete(srv.clients,con.ConHandle)
	srv.Unlock()
	srv.waitg.Done()
}

func (srv *DxTcpServer)SendHeart(con *DxNetConnection)  {

}

func (srv *DxTcpServer)GetBuffer(bufsize int)(retbuf *bytes.Buffer)  {
	var ok bool
	if retbuf,ok = srv.bufferPool.Get().(*bytes.Buffer);!ok{
		retbuf = bytes.NewBuffer(make([]byte,0,srv.GetCoder().MaxBufferLen()))
	}
	return
}

func (srv *DxTcpServer)ReciveBuffer(buf *bytes.Buffer)bool  {
	buf.Reset()
	srv.bufferPool.Put(buf)
	return true
}

func (srv *DxTcpServer)doOnSendData(params ...interface{})  {
	if params[4].(bool){
		srv.OnSendData(params[0].(*DxNetConnection),params[1],params[2].(int),params[3].(bool))
	}else{
		srv.AfterEncodeData(params[0].(*DxNetConnection),params[1],params[2].(int),params[3].(bool))
	}
}

func (srv *DxTcpServer)SendData(con *DxNetConnection,DataObj interface{})bool  {
	if con.UnActive(){
		return false
	}
	coder := srv.encoder
	sendok := false
	var haswrite int = 0
	if con.protocol == nil && coder!=nil{
		var retbytes []byte
		sendBuffer := srv.GetBuffer(0)
		headLen := coder.HeadBufferLen()
		if headLen > 2{
			headLen = 4
		}else{
			headLen = 2
		}
		//先写入数据内容长度进去
		if headLen <= 2{
			binary.Write(sendBuffer,binary.LittleEndian,uint16(1))
		}else{
			binary.Write(sendBuffer,binary.LittleEndian,uint32(1))
		}
		if err := coder.Encode(DataObj,sendBuffer);err==nil{
			if srv.OnSendData == nil && srv.AfterEncodeData != nil{
				srv.doOnSendData(con,DataObj,0,false,false)
			}
			retbytes = sendBuffer.Bytes()
			lenb := len(retbytes)
			objbuflen := lenb-int(headLen)
			//然后写入实际长度
			if headLen <= 2{
				if coder.UseLitterEndian(){
					binary.LittleEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
				}else{
					binary.BigEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
				}
			}else{
				if coder.UseLitterEndian(){
					binary.LittleEndian.PutUint32(retbytes[0:headLen],uint32(objbuflen))
				}else{
					binary.BigEndian.PutUint32(retbytes[0:headLen],uint32(objbuflen))
				}
			}
			sendok = con.writeBytes(retbytes)
		}else if srv.OnSendData == nil && srv.AfterEncodeData != nil{
			//DxCommonLib.PostFunc(srv.doOnSendData,con,DataObj,0,false,false)
			srv.doOnSendData(con,DataObj,0,false,false)
		}
		srv.ReciveBuffer(sendBuffer)//回收
	}else if con.protocol != nil{
		switch v := DataObj.(type){
		case *bytes.Buffer:
			sendok = con.writeBytes(v.Bytes())
		case bytes.Buffer:

		case []byte:
			sendok = con.writeBytes(v)
		default:
			sendBuffer := srv.GetBuffer(0)
			if retbytes,err := con.protocol.PacketObject(DataObj,sendBuffer);err==nil{
				sendok = con.writeBytes(retbytes)
			}else{
				sendok = false
				if srv.SrvLogger != nil{
					srv.SrvLogger.ErrorMsg("协议打包失败：%s",err.Error())
				}
			}
			srv.ReciveBuffer(sendBuffer)//回收
		}
	}
	if srv.OnSendData != nil{
		//DxCommonLib.PostFunc(srv.doOnSendData,con,DataObj,haswrite,sendok,true)
		srv.doOnSendData(con,DataObj,haswrite,sendok,true)
	}
	if sendok{
		atomic.AddUint64(&srv.SendRequestCount,1) //增加回复的请求数量
	}
	return sendok
}

func (srv *DxTcpServer)HandleConnectEvent(con *DxNetConnection)interface{}  {
	if srv.OnClientConnect!=nil{
		return srv.OnClientConnect(con)
	}
	return nil
}

func (srv *DxTcpServer)EnableHeartCheck() bool {
	return true
}

func (srv *DxTcpServer)ClientCount()(result int) {
	srv.Lock()
	result = len(srv.clients)
	srv.Unlock()
	return
}

//遍历客户端数据连接
func (srv *DxTcpServer)ClientIterate(iteratefunc GIterateClientFunc)  {
	srv.Lock()
	for _, c := range srv.clients {
		if !iteratefunc(c){
			break
		}
	}
	srv.Unlock()
}

//获取所有客户端map
func (srv *DxTcpServer)GetClients()map[uint]*DxNetConnection{
	return srv.clients
}


func (srv *DxTcpServer)HandleRecvEvent(con *DxNetConnection,recvData interface{})  {
	atomic.AddUint64(&srv.RequestCount,1) //增加接收的请求数量
	if srv.OnRecvData!=nil{
		srv.OnRecvData(con,recvData)
	}
}

func (srv *DxTcpServer)HeartTimeOutSeconds() int32 {
	return srv.TimeOutSeconds
}
//设置编码解码器
func (srv *DxTcpServer)SetCoder(encoder IConCoder)  {
	if srv.Active(){
		srv.Close()
	}
	srv.encoder = encoder
}

func (srv *DxTcpServer)Active()bool  {
	activeflag := atomic.LoadInt32(&srv.isActivetag)
	return activeflag != 0
}

