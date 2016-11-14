package dxserver

import (
	"net"
	"encoding/binary"
	"bytes"
	"time"
)

type GOnRecvDataEvent func(con *DxNetConnection,recvData interface{})
type GConnectEvent func(con *DxNetConnection)
type GOnSendDataEvent func(con *DxNetConnection,Data interface{},sendlen int,sendok bool)
type IConHost interface {
	GetCoder()IConCoder //编码器
	HandleRecvEvent(netcon *DxNetConnection,recvData interface{},recvDataLen uint16) //回收事件
	HandleDisConnectEvent(con *DxNetConnection)
	HandleConnectEvent(con *DxNetConnection)
	HeartTimeOutSeconds() int32 //设定的心跳间隔超时响应时间
	EnableHeartCheck() bool //是否启用心跳检查
	SendData(con *DxNetConnection,DataObj interface{})bool
}

//编码器
type IConCoder interface {
	Encode(obj interface{},buf *bytes.Buffer) error //编码对象
	Decode(bytes []byte)(result interface{},ok bool) //解码数据到对应的对象
	HeadBufferLen()uint16  //编码器的包头大小
	MaxBufferLen()uint16 //允许的最大缓存
}


type DataPackage struct {
	PkgObject interface{}
	pkglen	uint16
}

type DxNetConnection struct {
	con net.Conn
	localAddr           string
	remoteAddr          string
	conHost	    	    IConHost  //连接宿主
	LastValidTime	    time.Time //最后一次有效数据处理时间
	LoginTime	    time.Time //登录时间
	ConHandle	    uint
	conDisconnect	    chan bool
	SendDataLen	DxDiskSize
	ReciveDataLen	DxDiskSize
	sendDataQueue      chan *DataPackage
	recvDataQueue	chan *DataPackage
	LimitSendPkgCout uint8
}

//连接运行
func (con *DxNetConnection)run()  {
	go con.connectionRun()
}


func (con *DxNetConnection)connectionRun()  {
	if con.LimitSendPkgCout != 0{
		con.sendDataQueue = make(chan *DataPackage, con.LimitSendPkgCout)
	}
	con.recvDataQueue = make(chan *DataPackage,5)
	//心跳或发送数据
	con.conDisconnect = make(chan bool)
	go con.checkHeartorSendData()
	//开始进入获取数据信息
	con.LastValidTime = time.Now()
	con.conRead()
}

func (con *DxNetConnection)checkHeartorSendData()  {
	heartTimoutSenconts := con.conHost.HeartTimeOutSeconds()
	checkfor:
	for{
		select {
		case data, ok := <-con.sendDataQueue:
			if !ok || data.PkgObject == nil{
				break checkfor
			}
			con.conHost.SendData(con,data.PkgObject)
		case data,ok := <-con.recvDataQueue:
			if !ok || data.PkgObject == nil{
				break checkfor
			}
			con.conHost.HandleRecvEvent(con,data.PkgObject,data.pkglen)
		case <-con.conDisconnect:
			break checkfor
		case <-time.After(time.Millisecond * 300):
			if heartTimoutSenconts == 0 && con.conHost.EnableHeartCheck() &&
				time.Now().Sub(con.LastValidTime).Seconds() > 120{//时间间隔的秒数,超过2分钟无心跳，关闭连接
				go con.Close()
			}
		}
	}
}


func (con *DxNetConnection)Close()  {
	if con.conDisconnect !=nil{
		con.conDisconnect<-true
	}
	con.con.Close()
	con.conHost.HandleDisConnectEvent(con)
}

func (con *DxNetConnection)conRead()  {
	var timeout int32
	if con.conHost.EnableHeartCheck(){
		timeout = con.conHost.HeartTimeOutSeconds()
	}else{
		timeout = 0
	}
	encoder := con.conHost.GetCoder()
	if encoder == nil{
		con.Close()
		return
	}
	pkgHeadLen := encoder.HeadBufferLen() //包头长度
	if pkgHeadLen < 2 {
		pkgHeadLen = 2
	}
	maxbuflen := encoder.MaxBufferLen()
	buf := make([]byte, maxbuflen)
	var ln,lastReadBufLen uint16=0,0
	var rln,lastread int
	var err error
	var readbuf,tmpBuffer []byte
	for{
		if timeout != 0{
			con.con.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
		}
		if rln,err = con.con.Read(buf[:pkgHeadLen]);err !=nil || rln ==0{//获得实际的包长度的数据
			con.Close()
			return
		}
		if rln < 3{
			ln = binary.BigEndian.Uint16(buf[:rln])
		}else{
			ln = uint16(binary.BigEndian.Uint32(buf[:rln]))
		}
		pkglen := uint16(ln)//包长度
		if pkglen > maxbuflen{
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
				if rln,err = con.con.Read(readbuf[lastread:pkglen]);err !=nil {
					con.Close()
					return
				}
				lastread += rln
				if uint16(lastread) >= pkglen {
					break
				}
			}
			//读取成功，解码数据
			if obj,ok := encoder.Decode(readbuf[:pkglen]);ok{
				pkg := new(DataPackage)
				pkg.PkgObject = obj
				pkg.pkglen = pkglen
				con.recvDataQueue <- pkg //发送到执行回收事件的解析队列中去
			}else{
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

func (con *DxNetConnection)WriteObject(obj interface{})bool  {
	if con.LimitSendPkgCout == 0{
		return con.conHost.SendData(con,obj)
	}else{ //放到Chan列表中去发送
		pkg := new(DataPackage)
		pkg.PkgObject = obj
		select {
		case con.sendDataQueue <- pkg:
			{
				return true
			}
		case <-time.After(time.Millisecond * 100)://等100毫秒
			{
				con.Close()
				return false
			}
		}
	}
}

func (con *DxNetConnection)Address()string  {
	if con.localAddr == ""{
		con.localAddr = con.con.LocalAddr().String()
	}
	return con.localAddr
}
