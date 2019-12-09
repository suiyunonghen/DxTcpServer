package ServerBase

import (
	"net"
	"time"
	"unsafe"
	"bytes"
	"encoding/binary"
	"sync/atomic"
	"github.com/suiyunonghen/DxCommonLib"
)

type DxTcpClient struct {
	DxCommonLib.GDxBaseObject
	Clientcon  	DxNetConnection
	encoder		IConCoder
	OnRecvData	GOnRecvDataEvent
	OnSendHeart	GConnectEvent
	OnClientconnect	 func(con *DxNetConnection)interface{}
	OnClientDisConnected	GConnectEvent
	AfterClientDisConnected GConnectEvent
	OnSendData	GOnSendDataEvent
	TimeOutSeconds	int32
	ClientLogger LoggerInterface
	AfterRead					GConReadEvent
	BeforeRead					GConReadEvent
	sendBuffer	*bytes.Buffer
}

func (client *DxTcpClient)Active() bool {
	return !client.Clientcon.UnActive()
}

func (client *DxTcpClient) SubInit() {
	client.GDxBaseObject.SubInit(client)
}

func (client *DxTcpClient)CustomRead(con *DxNetConnection,targetData interface{})bool  {
	return false
}

func (client *DxTcpClient)Done()<-chan struct{}  {
	return  client.Clientcon.Done()
}

func (client *DxTcpClient)BeforePackageRead(con *DxNetConnection)(error)  {
	if client.BeforeRead!=nil{
		return client.BeforeRead(con)
	}
	return nil
}

func (client *DxTcpClient)AfterPackageRead(con *DxNetConnection)(error){
	if client.AfterRead != nil{
		return client.AfterRead(con)
	}
	return nil
}

func (client *DxTcpClient)Connect(addr string)error {
	if client.Active(){
		client.Close()
	}
	if tcpAddr, err := net.ResolveTCPAddr("tcp4", addr);err == nil{
		if conn, err := net.DialTCP("tcp", nil, tcpAddr);err == nil{ //创建一个TCP连接:TCPConn
			var host IConHost
			if lastedHost := client.LastedSubChild();lastedHost != nil{
				if Ahost,ok := lastedHost.(IConHost);!ok{
					host = client
				}else{
					host = Ahost
				}
			}else{
				host = client
			}
			atomic.StoreInt32(&client.Clientcon.unActive,0)
			client.Clientcon.con = conn
			client.Done()
			client.Clientcon.LoginTime = time.Now() //登录时间
			client.Clientcon.ConHandle = uint(uintptr(unsafe.Pointer(client)))
			client.Clientcon.conHost = host
			client.Clientcon.IsClientcon = true
			client.Clientcon.protocol = nil
			if client.encoder != nil{
				if protocol,ok := client.encoder.(IProtocol);ok{
					client.Clientcon.protocol = protocol
				}
			}
			result := client.HandleConnectEvent(&client.Clientcon)
			DxCommonLib.PostFunc(client.Clientcon.run,result)//连接开始执行接收消息和发送消息的处理线程
			return nil
		}else{
			return err
		}
	}else {
		return err
	}
}

func (client *DxTcpClient)AddRecvDataLen(datalen uint32){

}

func (client *DxTcpClient)AddSendDataLen(datalen uint32){

}

func (client *DxTcpClient)Logger()LoggerInterface  {
	return client.ClientLogger
}

func (client *DxTcpClient)HandleConnectEvent(con *DxNetConnection) interface{} {
	if client.OnClientconnect!=nil{
		return client.OnClientconnect(con)
	}
	return nil
}

func (client *DxTcpClient)HandleDisConnectEvent(con *DxNetConnection) {
	if client.OnClientDisConnected != nil{
		client.OnClientDisConnected(con)
	}
}

func (client *DxTcpClient)AfterDisConnected(con *DxNetConnection)   {
	if client.AfterClientDisConnected != nil{
		client.AfterClientDisConnected(con)
	}
}

func (client *DxTcpClient)HeartTimeOutSeconds() int32 {
	return client.TimeOutSeconds
}

func (client *DxTcpClient)Close()  {
	if client.Active(){
		client.Clientcon.Close()
	}
}

func (client *DxTcpClient)EnableHeartCheck()bool  {
	return  true
}

func (client *DxTcpClient)SendHeart(con *DxNetConnection)  {
	if client.Active() && client.OnSendHeart !=nil{
		client.OnSendHeart(con)
	}
}

func (client *DxTcpClient)HandleRecvEvent(con *DxNetConnection,recvData interface{})  {
	if client.OnRecvData!=nil{
		client.OnRecvData(con,recvData)
	}
}

//设置编码解码器
func (client *DxTcpClient)SetCoder(encoder IConCoder)  {
	client.Close()
	client.encoder = encoder
}

func (client *DxTcpClient)GetCoder() IConCoder {
	return client.encoder
}


func (client *DxTcpClient)doOnSendData(params ...interface{})  {
	client.OnSendData(params[0].(*DxNetConnection),params[1],params[2].(int),params[3].(bool))
}

func (client *DxTcpClient)SendBytes(b []byte) bool {
	if !client.Active(){
		return false
	}
	return client.Clientcon.writeBytes(b)
}

func (client *DxTcpClient)SendData(con *DxNetConnection,DataObj interface{})bool{
	if !client.Active(){
		return false
	}
	sendok := false
	var haswrite int = 0
	if con.protocol == nil && client.encoder!=nil{
		var retbytes []byte
		if client.sendBuffer == nil{
			client.sendBuffer = bytes.NewBuffer(make([]byte,0,client.encoder.MaxBufferLen()))
		}
		headLen := client.encoder.HeadBufferLen()
		if headLen > 2{
			headLen = 4
		}else{
			headLen = 2
		}
		//先写入数据内容长度进去
		if headLen <= 2{
			binary.Write(client.sendBuffer,binary.LittleEndian,uint16(1))
		}else{
			binary.Write(client.sendBuffer,binary.LittleEndian,uint32(1))
		}
		if err := client.encoder.Encode(DataObj,client.sendBuffer);err==nil{
			retbytes = client.sendBuffer.Bytes()
			lenb := len(retbytes)
			objbuflen := lenb-int(headLen)
			//然后写入实际长度
			if headLen <= 2{
				if client.encoder.UseLitterEndian(){
					binary.LittleEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
				}else{
					binary.BigEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
				}
			}else{
				if client.encoder.UseLitterEndian(){
					binary.LittleEndian.PutUint32(retbytes[0:headLen],uint32(objbuflen))
				}else{
					binary.BigEndian.PutUint32(retbytes[0:headLen],uint32(objbuflen))
				}
			}
			sendok = con.writeBytes(retbytes)
			con.LastValidTime.Store(time.Now().UnixNano())
		}
		client.sendBuffer.Reset()
		if client.sendBuffer.Cap() > int(client.encoder.MaxBufferLen()){
			client.sendBuffer = nil //超过最大数据长度，就清理掉本次的
		}
	}else if con.protocol != nil{
		if client.sendBuffer == nil{
			client.sendBuffer = bytes.NewBuffer(make([]byte,0,client.encoder.MaxBufferLen()))
		}
		if retbytes,err := con.protocol.PacketObject(DataObj,client.sendBuffer);err==nil{
			sendok = con.writeBytes(retbytes)
		}else{
			sendok = false
			if client.ClientLogger != nil{
				client.ClientLogger.ErrorMsg("协议打包失败：%s",err.Error())
			}
		}
	}
	if client.OnSendData != nil{
		//DxCommonLib.PostFunc(client.doOnSendData,con,DataObj,haswrite,sendok)
		client.doOnSendData(con,DataObj,haswrite,sendok)
	}
	return sendok
}
