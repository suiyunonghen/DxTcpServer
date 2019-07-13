package main

import (
	"github.com/suiyunonghen/DxValue"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"github.com/suiyunonghen/DxTcpServer/RPC"
	"time"
	"github.com/suiyunonghen/DxCommonLib"
	"fmt"
)

var(
	c chan struct{}
	client RPC.RpcClient
)
func ClientTime(con *ServerBase.DxNetConnection,pkg *RPC.RpcPkg)  {
	t := time.Now()
	pkg.SetResult(DxCommonLib.Time2DelphiTime(&t))
}
func testResult(connection *ServerBase.DxNetConnection, pkg *RPC.RpcPkg)  {
	fmt.Println("Test函数返回结果：",pkg.PkgData().AsString("Result",""))
}
func maxresult(connection *ServerBase.DxNetConnection, pkg *RPC.RpcPkg) {
	//result := pkg.PkgData().AsInt("Result", 0)
	rbase := pkg.Result()
	result,_ := rbase.AsInt()
	params := DxValue.NewRecord()
	params.SetInt("x", result+1)
	params.SetInt("x", result+2)
	fmt.Println("max的结果为: ", result)

	if result > 1000 {
		close(c)
	}else{
		DxCommonLib.Sleep(time.Second*5)
		client.Execute(&client.Clientcon,"max",params, maxresult)
	}
}
func main()  {
	client.Handle("ClientTime",ClientTime)
	client.Connect(":9988",512)
	params := DxValue.NewRecord()
	params.SetString("x1","测试支付成员")
	params.SetString("x2","测试支付成员2")
	params.SetString("x3","测试支付成员3")
	client.Execute(&client.Clientcon,"Test",params, testResult)

	c = make(chan struct{})
	params = DxValue.NewRecord()
	params.SetInt("x",0)
	params.SetInt("y",1)
	client.Execute(&client.Clientcon,"max",params, maxresult)
	select{
	case <-c:
		fmt.Println("退出")
	}
}
