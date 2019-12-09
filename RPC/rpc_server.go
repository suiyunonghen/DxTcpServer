package RPC

import (
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
)

type RpcServer struct {
	ServerBase.DxTcpServer
	RpcHandler
}

func (server *RpcServer)ListenAndServe(addr string,maxPkgSize uint16) error {
	if server.Active(){
		return nil
	}
	if server.MaxDataBufCount <= 0{
		server.MaxDataBufCount = 500
	}
	server.SetCoder(&RpcCoder{maxPkgSize})
	server.OnRecvData = server.serverPkg
	server.OnSendData = server.onSendData
	return server.Open(addr)
}