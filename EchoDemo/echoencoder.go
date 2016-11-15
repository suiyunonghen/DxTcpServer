package EchoDemo

import (
	"suiyunonghen/DxTcpServer"
	"bytes"
	"fmt"
)

type EchoCoder struct {

}

func (coder *EchoCoder)Encode(obj interface{},buf *bytes.Buffer) error  {
	switch obj.(type) {
	case []byte:
		buf.Write(obj.([]byte))
	case string:
		buf.Write([]byte(obj.(string)))
	}
	return nil
}

func (coder *EchoCoder)Decode(bytes []byte)(result interface{},ok bool)  {
	return bytes,true
}

func (coder *EchoCoder)HeadBufferLen()uint16  {
	return 2
}

func (coder *EchoCoder)MaxBufferLen()uint16  {
	return 1024
}

func NewEchoServer()*dxserver.DxTcpServer{
	srv := new(dxserver.DxTcpServer)
	srv.LimitSendPkgCount = 20
	coder := new(EchoCoder)
	srv.SetCoder(coder)
	srv.OnRecvData = func(con *dxserver.DxNetConnection,recvData interface{}) {
		//直接发送回去
		con.WriteObject(recvData)
	}
	/*srv.OnClientConnect = func(con *dxserver.DxNetConnection){
		//客户端登录了
		fmt.Println("登录客户",srv.ClientCount())
	}*/
	return srv
}

func NewEchoClient()*dxserver.DxTcpClient  {
	client := new(dxserver.DxTcpClient)
	coder := new(EchoCoder)
	client.SetCoder(coder)
	client.OnRecvData = func(con *dxserver.DxNetConnection,recvData interface{}) {
		fmt.Println(string(recvData.([]byte)))
	}
	return client
}