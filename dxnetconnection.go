package DxTcpServer

import (
	"net"
	"encoding/binary"
	"time"
	"fmt"
	"io"
	"github.com/golang-plus/log"
	"github.com/suiyunonghen/DxCommonLib"
)

type GOnRecvDataEvent func(con *DxNetConnection,recvData interface{})
type GConnectEvent func(con *DxNetConnection)
type GOnSendDataEvent func(con *DxNetConnection,Data interface{},sendlen int,sendok bool)
type IConHost interface {
	GetCoder()IConCoder //编码器
	HandleRecvEvent(netcon *DxNetConnection,recvData interface{}) //回收事件
	HandleDisConnectEvent(con *DxNetConnection)
	HandleConnectEvent(con *DxNetConnection)
	HeartTimeOutSeconds() int32 //设定的心跳间隔超时响应时间
	EnableHeartCheck() bool //是否启用心跳检查
	SendHeart(con *DxNetConnection) //发送心跳
	SendData(con *DxNetConnection,DataObj interface{})bool
	Logger()*log.Logger
	AddRecvDataLen(datalen uint32)
}

//编码器
type IConCoder interface {
	Encode(obj interface{},w io.Writer) error //编码对象
	Decode(bytes []byte)(result interface{},ok bool) //解码数据到对应的对象
	HeadBufferLen()uint16  //编码器的包头大小
	MaxBufferLen()uint16 //允许的最大缓存
	UseLitterEndian()bool //是否采用小结尾端
}

//协议
type IProtocol interface{
	ProtoName()string
	ParserProtocol(r io.Reader)(parserOk bool,datapkg interface{})		//解析协议，如果解析成功，返回true，根据情况可以设定返回协议数据包
	PacketObject(objpkg interface{})([]byte,error)  //将发送的内容打包
}


type DataPackage struct {
	PkgObject interface{}
	pkglen	uint32
}


type DxNetConnection struct {
	con net.Conn
	localAddr	            string
	remoteAddr	            string
	conHost		    	    IConHost  //连接宿主
	protocol				IProtocol
	LastValidTime		    time.Time //最后一次有效数据处理时间
	LoginTime		    	time.Time //登录时间
	ConHandle		   		uint
	conDisconnect		    chan struct{}
	unActive				bool //已经关闭了
	SendDataLen		    	DxDiskSize
	ReciveDataLen		    DxDiskSize
	sendDataQueue	      	chan interface{}
	recvDataQueue		    chan interface{}
	LimitSendPkgCout	    uint8
	IsClientcon		    	bool
	useData			    	interface{} //用户数据
}

func (con *DxNetConnection)SetUseData(v interface{})  {
	con.useData = v
}

func (con *DxNetConnection)GetUseData()interface{}  {
	return con.useData
}

func (con *DxNetConnection)Run()  {
	//开始进入获取数据信息
	con.LastValidTime = time.Now()
	con.recvDataQueue = make(chan interface{},5)
	//心跳或发送数据
	con.conDisconnect = make(chan struct{})
	DxCommonLib.PostFunc(con.checkHeartorSendData,true) //接收
	if con.LimitSendPkgCout != 0{
		con.sendDataQueue = make(chan interface{}, con.LimitSendPkgCout)
		DxCommonLib.PostFunc(con.checkHeartorSendData,false)  //发送
	}
	if con.conHost.GetCoder() == nil{
		con.conCustomRead()
		return
	}
	con.conRead()
}

func (con *DxNetConnection)writeBytes(wbytes []byte)bool  {
	haswrite := 0
	loger := con.conHost.Logger()
	lenb := len(wbytes)
	for {
		if wln,err := con.con.Write(wbytes[haswrite:lenb]);err != nil{
			if loger != nil{
				loger.SetPrefix("[Error]")
				loger.Debugln(fmt.Sprintf("写入远程客户端%s失败，程序准备断开：%s",con.RemoteAddr(),err.Error()))
			}
			con.Close()
			return false
		}else{
			con.LastValidTime = time.Now()
			haswrite+=wln
			if haswrite == lenb{
				return true
			}
		}
	}
}

//执行自定义数据包格式的处理规则
func (con *DxNetConnection)conCustomRead()  {
	reader := NewDxReader(con.con,4096)
	for{
		rlen,e,_ := reader.ReadAppend()
		if e!=nil || rlen==0{
			loger := con.conHost.Logger()
			if loger != nil{
				loger.SetPrefix("[Error]")
				if con.IsClientcon{
					loger.Debugln("读取失败，程序准备断开：",e.Error())
				}else{
					loger.Debugln(fmt.Sprintf("远程客户端%s，读取失败，程序准备断开：%s",con.RemoteAddr(),e.Error()))
				}
			}
			con.Close()
			return
		}
		con.ReciveDataLen.AddByteSize(uint32(rlen))
		con.conHost.AddRecvDataLen(uint32(rlen))
		for{
			markidx,markOffset := reader.MarkIndex()
			pok,pkg := con.protocol.ParserProtocol(reader)//解析出实际的协议宝
			if !pok{
				reader.RestoreMark(markidx,markOffset)
				break
			}else{
				//如果协议包不为空，就发送出去，然后等待处理
				con.recvDataQueue <- pkg //发送到执行回收事件的解析队列中去
				reader.ClearRead()
			}
		}

	}
}


func (con *DxNetConnection)checkHeartorSendData(data ...interface{})  {
	IsRecvFunc := data[0].(bool)
	if IsRecvFunc{ //接收函数
		heartTimoutSenconts := con.conHost.HeartTimeOutSeconds()
		timeoutChan := DxCommonLib.After(time.Second*2)
		for{
			select {
			case data,ok := <-con.recvDataQueue:
				if !ok || data == nil{
					return
				}
				con.conHost.HandleRecvEvent(con,data)
			case <-con.conDisconnect:
				return
			case <-timeoutChan:
				if con.IsClientcon{ //客户端连接
					if heartTimoutSenconts == 0 && con.conHost.EnableHeartCheck() &&
						time.Now().Sub(con.LastValidTime).Seconds() > 60{ //60秒发送一次心跳
						con.conHost.SendHeart(con)
					}
				}else if heartTimoutSenconts == 0 && con.conHost.EnableHeartCheck() &&
					time.Now().Sub(con.LastValidTime).Seconds() > 120{//时间间隔的秒数,超过2分钟无心跳，关闭连接
					loger := con.conHost.Logger()
					if loger != nil{
						loger.SetPrefix("[Debug]")
						loger.Debugln(fmt.Sprintf("远程客户端连接%s，超过2分钟未获取心跳，连接准备断开",con.RemoteAddr()))
					}
					con.Close()
					return
				}
				timeoutChan = DxCommonLib.After(time.Second*2) //继续下一次的判定
			}
		}
	}else{
		for{
			select{
			case data, ok := <-con.sendDataQueue:
				if !ok || data == nil{
					return
				}
				con.conHost.SendData(con,data)
			case <-con.conDisconnect:
				return
			}
		}
	}
}


func (con *DxNetConnection)Close()  {
	if con.unActive{
		return
	}
	if con.conDisconnect !=nil{
		close(con.conDisconnect)
		con.conDisconnect = nil
	}
	con.con.Close()
	con.conHost.HandleDisConnectEvent(con)
	con.unActive = true
	if con.recvDataQueue != nil{
		close(con.recvDataQueue)
	}
	if con.sendDataQueue != nil{
		close(con.sendDataQueue)
	}
}

func (con *DxNetConnection)conRead()  {
	encoder := con.conHost.GetCoder()
	var timeout int32
	if con.conHost.EnableHeartCheck(){
		timeout = con.conHost.HeartTimeOutSeconds()
	}else{
		timeout = 0
	}
	pkgHeadLen := encoder.HeadBufferLen() //包头长度
	if pkgHeadLen <= 2 {
		pkgHeadLen = 2
	}else{
		pkgHeadLen = 4
	}
	maxbuflen := encoder.MaxBufferLen()
	buf := make([]byte, maxbuflen)
	var ln,lastReadBufLen uint32=0,0
	var rln,lastread int
	var err error
	var readbuf,tmpBuffer []byte
	for{
		if timeout != 0{
			con.con.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
		}
		if rln,err = con.con.Read(buf[:pkgHeadLen]);err !=nil || rln ==0{//获得实际的包长度的数据
			loger := con.conHost.Logger()
			if loger != nil{
				loger.SetPrefix("[Error]")
				if con.IsClientcon{
					loger.Debugln("读取失败，程序准备断开：",err.Error())
				}else{
					loger.Debugln(fmt.Sprintf("远程客户端%s，读取失败，程序准备断开：%s",con.RemoteAddr(),err.Error()))
				}
			}
			con.Close()
			return
		}
		if rln < 3{
			if encoder.UseLitterEndian(){
				ln = uint32(binary.LittleEndian.Uint16(buf[:rln]))
			}else{
				ln = uint32(binary.BigEndian.Uint16(buf[:rln]))
			}
		}else{
			if encoder.UseLitterEndian(){
				ln = binary.LittleEndian.Uint32(buf[:rln])
			}else{
				ln = binary.BigEndian.Uint32(buf[:rln])
			}
		}
		pkglen := ln//包长度
		if pkglen > uint32(maxbuflen){
			if lastReadBufLen < pkglen{
				tmpBuffer = make([]byte,pkglen)
			}
			readbuf = tmpBuffer
			lastReadBufLen = pkglen
		}else{
			readbuf = buf
		}
		lastread = 0
		if pkglen > 0{
			for{
				if timeout != 0{
					con.con.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
				}
				if rln,err = con.con.Read(readbuf[lastread:pkglen]);err !=nil || rln ==0 {
					loger := con.conHost.Logger()
					if loger != nil{
						loger.SetPrefix("[Error]")
						if con.IsClientcon{
							loger.Debugln("读取失败，程序准备断开：",err.Error())
						}else{
							loger.Debugln(fmt.Sprintf("远程客户端连接%s，读取失败，程序准备断开：%s",con.RemoteAddr(),err.Error()))
						}
					}
					con.Close()
					return
				}
				lastread += rln
				if uint32(lastread) >= pkglen {
					break
				}
			}
			//读取成功，解码数据
			if obj,ok := encoder.Decode(readbuf[:pkglen]);ok{
				con.conHost.AddRecvDataLen(uint32(pkglen))
				con.recvDataQueue <- obj //发送到执行回收事件的解析队列中去
			}else{
				loger := con.conHost.Logger()
				if loger != nil{
					loger.SetPrefix("[Error]")
					if con.IsClientcon{
						loger.Debugln("无效的数据包，异常，程序准备断开：")
					}else{
						loger.Debugln(fmt.Sprintf("远程客户端%s，读取失败，程序准备断开",con.RemoteAddr()))
					}
				}
				con.Close()//无效的数据包
				return
			}
		}
		con.LastValidTime = time.Now()
		if timeout != 0{
			con.con.SetReadDeadline(time.Time{})
		}
	}
}


func (con *DxNetConnection)RemoteAddr()string  {
	if con.remoteAddr == ""{
		con.remoteAddr = con.con.RemoteAddr().String()
	}
	return con.remoteAddr
}

func (con *DxNetConnection)synSendData(data ...interface{})  {
	con.conHost.SendData(con,data[1])
}

func (con *DxNetConnection)WriteObjectSync(obj interface{})  {
	DxCommonLib.PostFunc(con.synSendData,obj)
}

func (con *DxNetConnection)WriteObjectDirect(obj interface{})bool  {
	return con.conHost.SendData(con,obj)
}

func (con *DxNetConnection)WriteObject(obj interface{})bool  {
	if con.LimitSendPkgCout == 0{
		return con.conHost.SendData(con,obj)
	}else{ //放到Chan列表中去发送
		select {
		case con.sendDataQueue <- obj:
				return true
		case <-DxCommonLib.After(time.Millisecond*500):
				con.Close()
				return false
		}
	}
}

func (con *DxNetConnection)Address()string  {
	if con.localAddr == ""{
		con.localAddr = con.con.LocalAddr().String()
	}
	return con.localAddr
}
