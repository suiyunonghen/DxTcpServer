package main

import (
	"github.com/suiyunonghen/DxCommonLib"
	"time"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"fmt"
	"github.com/suiyunonghen/DxTcpServer/RPC"
)

func testfunc(con *ServerBase.DxNetConnection,pkg *RPC.RpcPkg){
	params := pkg.Params()
	rec,_:= params.AsRecord()
	pkg.PkgData().SetString("Result",fmt.Sprintf("%s_%s_%s",
		rec.AsString("x1",""),
		rec.AsString("x2",""),
		rec.AsString("x1","")))
	rec.ClearValue(false)
}

func maxfunc(con *ServerBase.DxNetConnection,pkg *RPC.RpcPkg){
	params := pkg.Params()
	rec,_:= params.AsRecord()
	x := rec.AsInt("x",0)
	y := rec.AsInt("y",0)
	if x > y{
		pkg.PkgData().SetInt("Result",x)
	}else{
		pkg.PkgData().SetInt("Result",y)
	}
}

func main()  {
	var srv RPC.RpcServer
	srv.Handle("Test",testfunc)
	srv.Handle("max",maxfunc)
	srv.ListenAndServe(":9988",512)
	for{
		select{
		case <-DxCommonLib.After(time.Second*5):
			cons := srv.GetClients()
			for _,con := range cons{
				srv.Execute(con,"ClientTime",nil, func(connection *ServerBase.DxNetConnection, pkg *RPC.RpcPkg) {
					t := pkg.PkgData().AsDateTime("Result",0)
					fmt.Println(con.RemoteAddr(),"上的当前时间为：",t.ToTime().Format("15:04:05"))
				})
			}
		}
	}
}
