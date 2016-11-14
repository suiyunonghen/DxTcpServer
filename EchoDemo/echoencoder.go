package EchoDemo

import (
	"encoding/binary"
	"DxFileServer/src/dxserver"
)

type EchoCoder struct {

}

func (coder *EchoCoder)Encode(obj interface{})(bytes []byte,ok bool)  {
	l := uint32(len(obj.([]byte)))
	headlen := coder.HeadBufferLen()
	bytes = make([]byte,headlen)
	if headlen <3{
		binary.BigEndian.PutUint16(bytes[:headlen],uint16(l))
	}else{
		binary.BigEndian.PutUint32(bytes[:headlen],l)
	}
	bytes = append(bytes,obj.([]byte)...)
	return bytes,true
}

func (coder *EchoCoder)Decode(bytes []byte)(result interface{},ok bool)  {
	return bytes,true
}

func (coder *EchoCoder)HeadBufferLen()uint16  {
	return 2
}

func (coder *EchoCoder)MaxBufferLen()uint16  {
	return 256
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