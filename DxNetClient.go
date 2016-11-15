package dxserver

import (
	"net"
	"time"
	"unsafe"
	"bytes"
	"encoding/binary"
)

type DxTcpClient struct {
	Clientcon  	DxNetConnection
	encoder		IConCoder
	OnRecvData	GOnRecvDataEvent
	OnSendHeart	GConnectEvent
	OnClientconnect	GConnectEvent
	OnClientDisConnected	GConnectEvent
	OnSendData	GOnSendDataEvent
	Active		bool
	TimeOutSeconds	int32
	sendBuffer	[]byte
}

func (client *DxTcpClient)Connect(addr string)error {
	if client.Active{
		client.Close()
	}
	if tcpAddr, err := net.ResolveTCPAddr("tcp4", addr);err == nil{
		if conn, err := net.DialTCP("tcp", nil, tcpAddr);err == nil{ //创建一个TCP连接:TCPConn
			client.Clientcon.con = conn
			client.Clientcon.LoginTime = time.Now() //登录时间
			client.Clientcon.ConHandle = uint(uintptr(unsafe.Pointer(client)))
			client.Clientcon.conHost = client
			client.Clientcon.IsClientcon = true
			client.HandleConnectEvent(&client.Clientcon)
			client.Clientcon.run() //连接开始执行接收消息和发送消息的处理线程
			client.Active = true
			return nil
		}else{
			return err
		}
	}else {
		return err
	}
}

func (client *DxTcpClient)HandleConnectEvent(con *DxNetConnection)  {
	if client.OnClientconnect!=nil{
		client.OnClientconnect(con)
	}
}

func (client *DxTcpClient)HandleDisConnectEvent(con *DxNetConnection) {
	client.Active = false
	if client.OnClientDisConnected != nil{
		client.OnClientDisConnected(con)
	}
}

func (client *DxTcpClient)HeartTimeOutSeconds() int32 {
	return client.TimeOutSeconds
}

func (client *DxTcpClient)Close()  {
	if client.Active{
		client.Clientcon.Close()
		client.Active = false
	}
}

func (client *DxTcpClient)EnableHeartCheck()bool  {
	return  true
}

func (client *DxTcpClient)SendHeart(con *DxNetConnection)  {
	if client.OnSendHeart !=nil{
		client.OnSendHeart(con)
	}
}

func (client *DxTcpClient)HandleRecvEvent(con *DxNetConnection,recvData interface{},recvDataLen uint16)  {
	if client.OnRecvData!=nil{
		client.OnRecvData(con,recvData)
	}
}

//设置编码解码器
func (client *DxTcpClient)SetCoder(encoder IConCoder)  {
	if client.Active{
		client.Close()
	}
	client.encoder = encoder
}

func (client *DxTcpClient)GetCoder() IConCoder {
	return client.encoder
}

func (client *DxTcpClient)SendData(con *DxNetConnection,DataObj interface{})bool{
	coder := client.encoder
	sendok := false
	var haswrite int = 0
	if coder!=nil{
		var retbytes []byte
		if client.sendBuffer == nil{
			client.sendBuffer = make([]byte,coder.MaxBufferLen())
		}
		headLen := coder.HeadBufferLen()
		retbytes = client.sendBuffer[0:headLen]
		buf := bytes.NewBuffer(retbytes[:headLen])
		if err := coder.Encode(DataObj,buf);err==nil{
			retbytes = buf.Bytes()
			objbuflen := uint16(buf.Len()) - headLen
			binary.BigEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
			lenb := len(retbytes)
			buf = nil
			for {
				con.LastValidTime = time.Now()
				if wln,err := con.con.Write(retbytes[haswrite:lenb]);err != nil{
					con.Close()
					break
				}else{
					haswrite+=wln
					if haswrite == lenb{
						sendok =true
						break
					}
				}
			}
			//写入发送了多少数据
			con.LastValidTime = time.Now()
			con.SendDataLen.AddByteSize(uint32(lenb))
		}
	}
	if client.OnSendData != nil{
		client.OnSendData(con,DataObj,haswrite,sendok)
	}
	return sendok
}
