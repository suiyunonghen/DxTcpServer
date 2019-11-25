package RPC

import (
	"github.com/suiyunonghen/DxCommonLib"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"github.com/suiyunonghen/DxValue"
	"time"
)

type ReconnectEvent = func(clientErr error)

type RpcClient struct {
	ServerBase.DxTcpClient
	RpcHandler
	ReconRqv		byte						//间隔重连的频率，分钟单位
	fServerAddr 	string
	OnReconnect		ReconnectEvent
}

func (client *RpcClient)heart(con *ServerBase.DxNetConnection)  {
	client.Notify(con,"SendHeart",nil)
}

func (client *RpcClient)disconnect(con *ServerBase.DxNetConnection)  {
	client.DoReconnect()
}

func (client *RpcClient)DoReconnect()  {
	DxCommonLib.PostFunc(client.reConnect)
}

func (client *RpcClient)ExecuteWait(MethodName string,Params *DxValue.DxRecord,WaitTime int32)(result *DxValue.DxBaseValue,err string){
	return client.RpcHandler.ExecuteWait(&client.Clientcon,MethodName,Params,WaitTime)
}


func (client *RpcClient)reConnect(data ...interface{})  {
	frefcount := 1
	timeoutchan := DxCommonLib.After(time.Second * 3) //第一次3秒之后重连
reconnect:
	for{
		select {
		case <-timeoutchan:
			if err:=client.DxTcpClient.Connect(client.fServerAddr);err==nil{
				if client.OnReconnect != nil{ //重连事件
					client.OnReconnect(nil)
				}
				return
			}else if client.OnReconnect != nil{
				client.OnReconnect(err)
			}
			timeoutchan = DxCommonLib.After(time.Second * 10 * time.Duration(frefcount))
		}
		if frefcount++;frefcount>6{
			break
		}
	}
	//连续连接5次连接失败，开始执行按照指定的频率重连
	for{
		select {
		case <-DxCommonLib.After(time.Minute * time.Duration(client.ReconRqv)):
			frefcount = 1
			goto reconnect
		}
	}
}

func (client *RpcClient)Connect(serverAddr string,maxPkgSize uint16) error {
	if client.Active(){
		return nil
	}
	if client.ReconRqv <= 0{
		client.ReconRqv = 3
	}
	client.SetCoder(&RpcCoder{maxPkgSize})
	if client.OnSendHeart == nil{
		client.OnSendHeart = client.heart
	}
	client.OnRecvData = client.serverPkg
	client.OnSendData = client.onSendData
	client.OnClientDisConnected = client.disconnect

	err := client.DxTcpClient.Connect(serverAddr)
	if err != nil{
		client.fServerAddr = serverAddr
	}
	return err
}