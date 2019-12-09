package RPC

import (
	"fmt"
	"github.com/suiyunonghen/DxCommonLib"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"github.com/suiyunonghen/DxValue"
	"sync"
	"time"
)

type GSendDataEvent func(con *ServerBase.DxNetConnection,Data *RpcPkg,sendlen int,sendok bool)
type RpcHandler struct {
	fhandlers					sync.Map			 //客户请求的方法处理
	fRpcWaitReturnMethods		sync.Map			 //等待返回的RPC方法
	fclientResponseHandlers		sync.Map			 //自己向客户端请求发送的方法，客户端结果返回处理
	AfterSendData				GSendDataEvent
}


//向客户端请求
func (rhandle *RpcHandler)Execute(con *ServerBase.DxNetConnection, MethodName string,Params *DxValue.DxRecord,resultHandle MethodHandler)  {
	methodid := SnowFlakeID()
	method := GetMethod(MethodName,false,methodid)
	method.fresultHandler = resultHandle
	method.pkgData.SetRecordValue("Params",Params)
	if resultHandle != nil{
		rhandle.fRpcWaitReturnMethods.Store(methodid,method) //有指定返回函数的，存放一下,获取了执行之后，自动回调
	}
	con.WriteObject(method)
}

//通知客户端
func (rhandle *RpcHandler)Notify(con *ServerBase.DxNetConnection, MethodName string,Params *DxValue.DxRecord)  {
	method := GetMethod(MethodName,true,SnowFlakeID())
	method.pkgData.SetRecordValue("Params",Params)
	con.WriteObject(method)
	return
}

func (rhandle *RpcHandler)NotifyDirect(con *ServerBase.DxNetConnection, MethodName string,Params *DxValue.DxRecord)  {
	method := GetMethod(MethodName,true,SnowFlakeID())
	method.pkgData.SetRecordValue("Params",Params)
	con.WriteObjectDirect(method)
	return
}


func (rhandle *RpcHandler)ExecuteWait(con *ServerBase.DxNetConnection, MethodName string,Params *DxValue.DxRecord,WaitTime int32)(result *DxValue.DxBaseValue,err string)  {
	methodid := SnowFlakeID()
	if WaitTime<=0{
		WaitTime = 5000
	}
	method := GetMethod(MethodName,false,methodid)
	method.pkgData.SetRecordValue("Params",Params)
	waitchan := method.WaitChan()
	rhandle.fRpcWaitReturnMethods.Store(methodid,method)
	if ! con.WriteObject(method){
		FreeMethod(method)
		return nil,"执行远程请求失败，连接可能已经关闭"
	}
	select {
	case resultData,ok := <-waitchan:
		//返回了
		if ok{
			err = resultData.AsString("Err","")
			result = resultData.ExtractValue("Result")
		}
	case <-DxCommonLib.After(time.Millisecond * time.Duration(WaitTime)):
		//超时了
		result = nil
		rhandle.fRpcWaitReturnMethods.Delete(methodid)
		err = "TimeOut"
	}
	FreeMethod(method)
	return
}


func (rpHandle *RpcHandler)serverPkg(con *ServerBase.DxNetConnection,recvData interface{})  {
	methodpkg := recvData.(*RpcPkg)
	defer func(){
		if err := recover();err!=nil{
			methodpkg.pkgData.Delete("Params")
			methodpkg.pkgData.Delete("Result")
			methodpkg.fReturnResult = true//立即返回
			methodpkg.pkgData.SetString("Err",fmt.Sprintf("%v",err))
		}
	}()
	methodName := methodpkg.MethodName()
	pktType := RpcPkgType(methodpkg.pkgData.AsInt("Type",int(RPT_UnKnown)))
	if  pktType == RPT_Result{ //是返回结果，做结果处理
		//获取结果，返回
		methodid := methodpkg.MethodID()
		var (
			runMethod *RpcPkg
			resulthandler MethodHandler
		)
		if vhandle,ok := rpHandle.fRpcWaitReturnMethods.Load(methodid);ok{
			runMethod = vhandle.(*RpcPkg)
		}
		resultFuncHooked := false
		if runMethod != nil {
			willFree := true
			if runMethod.CloseWaitChan(methodpkg.pkgData){
				methodpkg.pkgData = nil
				//ExecuteWait返回释放
				resultFuncHooked = true
				willFree = false
			}else if runMethod.fresultHandler != nil{
				resultFuncHooked = true
				runMethod.fresultHandler(con,methodpkg)
			}
			rpHandle.fRpcWaitReturnMethods.Delete(methodid)
			if willFree{
				FreeMethod(runMethod)
			}
		}
		if !resultFuncHooked{
			if vresulthandler,ok := rpHandle.fclientResponseHandlers.Load(methodName);ok{
				resulthandler = vresulthandler.(MethodHandler)
				if resulthandler != nil {//执行结果处理函数
					resulthandler(con,methodpkg)
				}
			}
		}
		FreeMethod(methodpkg)
		return
	}
	if vresulthandler,ok :=  rpHandle.fhandlers.Load(methodName);ok{
		handler := vresulthandler.(MethodHandler)
		if pktType == RPT_Notify{ //通知，不用返回的
			methodpkg.fReturnResult = true //默认就是允许回收的，如果不允许回收，可以设置 methodpkg.SetCanRecive(false)
			handler(con,methodpkg)
			if methodpkg.fReturnResult{
				FreeMethod(methodpkg)
			}
			return
		}
		handler(con,methodpkg)
		if methodpkg.ReturnResult(){
			methodpkg.pkgData.SetInt("Type",int(RPT_Result)) //作为结果返回
			methodpkg.pkgData.Delete("Params") //删除参数节点
			con.WriteObject(methodpkg) //发送结果回去
		}
	}else{
		//logger.Debugln("未处理的函数：",methodpkg.MethodName)
		FreeMethod(methodpkg)
	}
}

func (rpHandle *RpcHandler)Handle(methodName string,handler MethodHandler)  {
	rpHandle.fhandlers.Store(methodName,handler)
}

func (rpHandle *RpcHandler)HandleResponse(methodName string,handler MethodHandler)  {
	rpHandle.fclientResponseHandlers.Store(methodName,handler)
}


func (rpHandle *RpcHandler)onSendData(con *ServerBase.DxNetConnection,Data interface{},sendlen int,sendok bool){
	//回收结果数据
	resultpkg := Data.(*RpcPkg)
	if rpHandle.AfterSendData != nil && !sendok{
		rpHandle.AfterSendData(con,resultpkg,sendlen,sendok)
	}
	if !resultpkg.fHasWait && resultpkg.fresultHandler == nil || resultpkg.pkgData.AsInt("Type",0) == int(RPT_Result) {
		if !con.IsClientcon{
			if !sendok{ //编码完成就释放
				FreeMethod(resultpkg)
			}
		}else{
			FreeMethod(resultpkg)
		}
	}
}