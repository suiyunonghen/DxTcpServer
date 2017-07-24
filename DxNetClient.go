package DxTcpServer

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
			client.Clientcon.unActive = false
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

func (client *DxTcpClient)HandleRecvEvent(con *DxNetConnection,recvData interface{},recvDataLen uint32)  {
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
	sendok := false
	var haswrite int = 0
	if client.encoder!=nil{
		var retbytes []byte
		if client.sendBuffer == nil{
			client.sendBuffer = make([]byte,client.encoder.MaxBufferLen())
		}
		headLen := client.encoder.HeadBufferLen()
		if headLen > 2{
			headLen = 4
		}else{
			headLen = 2
		}
		retbytes = client.sendBuffer[0:headLen]
		lenb := int(headLen)
		buf := bytes.NewBuffer(client.sendBuffer[headLen:headLen])
		if err := client.encoder.Encode(DataObj,buf);err==nil{
			if headLen <= 2{
				objbuflen := uint16(buf.Len())
				lenb += int(objbuflen) //实际要发送的数据内容长度
				if client.encoder.UseLitterEndian(){
					binary.LittleEndian.PutUint16(retbytes,objbuflen)
				}else{
					binary.BigEndian.PutUint16(retbytes,objbuflen)
				}
			}else{
				objbuflen := uint32(buf.Len())
				lenb += int(objbuflen) //实际要发送的数据内容长度
				if client.encoder.UseLitterEndian(){
					binary.LittleEndian.PutUint32(retbytes,objbuflen)
				}else{
					binary.BigEndian.PutUint32(retbytes,objbuflen)
				}
			}
			retbytes = client.sendBuffer[0:lenb]//实际要发送的数据内容
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
