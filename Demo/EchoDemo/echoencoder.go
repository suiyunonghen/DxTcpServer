package EchoDemo

import (
	"io"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"encoding/binary"
	"bytes"
	"fmt"
)

type EchoCoder struct {

}

func (coder *EchoCoder)Encode(obj interface{},w io.Writer)error  {
	return nil
}

func (coder *EchoCoder)Decode(bytes []byte)(result interface{},ok bool)  {
	return bytes,true
}

func (coder *EchoCoder)HeadBufferLen()uint16  {
	return 0
}

func (coder *EchoCoder)UseLitterEndian()bool  {
	return false
}

func (coder *EchoCoder)MaxBufferLen()uint16  {
	return 0
}

//实现Echo协议接口
func (coder *EchoCoder)ProtoName()string  {
	return "ECHO"
}

func (coder *EchoCoder)ParserProtocol(r *ServerBase.DxReader,con *ServerBase.DxNetConnection)(parserOk bool,datapkg interface{},e error) { //解析协议，如果解析成功，返回true，根据情况可以设定返回协议数据包
	count := r.Buffered()
	if count > 0{
		if !con.IsClientcon{
			r.WriteTo(con,count) //直接写入
			datapkg = nil
		}else{
			bt := make([]byte,count)
			r.Read(bt)
			datapkg = bt
		}
		parserOk = true
	}else{
		datapkg = nil
		parserOk = false
	}
	return parserOk,datapkg,nil
}
func (coder *EchoCoder)PacketObject(objpkg interface{},buffer *bytes.Buffer)([]byte,error) { //将发送的内容打包
	switch v := objpkg.(type){
	case string:
		return ([]byte)(v),nil
	case []byte:
		return v,nil
	default:
		binary.Write(buffer,binary.BigEndian,objpkg)
		return buffer.Bytes(),nil
	}
}

func NewEchoServer()*ServerBase.DxTcpServer{
	srv := new(ServerBase.DxTcpServer)
	srv.LimitSendPkgCount = 20
	coder := new(EchoCoder)
	srv.SetCoder(coder)
	srv.OnRecvData = func(con *ServerBase.DxNetConnection,recvData interface{}) {
		//直接发送回去
		fmt.Println("客户端发送：",string(recvData.([]byte)))
		con.WriteObject(recvData)
	}
	srv.OnClientConnect = func(con *ServerBase.DxNetConnection)interface{}{
		//客户端登录了
		fmt.Println("登录客户",srv.ClientCount())
		return nil
	}
	return srv
}

func NewEchoClient()*ServerBase.DxTcpClient  {
	client := new(ServerBase.DxTcpClient)
	client.SetCoder(new(EchoCoder))
	client.OnRecvData = func(con *ServerBase.DxNetConnection,recvData interface{}) {
		//直接发送回去
		fmt.Println("服务端回复：",string(recvData.([]byte)))
	}
	return client
}