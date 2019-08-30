package RPC

import (
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"sync"
	"reflect"
	"github.com/suiyunonghen/DxValue"
)

type rpcMethod struct {
	Name				string
	paramType			[]reflect.Type
	InstanceV			*reflect.Value
	funcValue			reflect.Value
}

func setarrV(params []reflect.Value, arr *DxValue.DxArray,paramidx,arridx int,tp reflect.Type)  {
	switch tp.Kind() {
	case reflect.Int:
		params[paramidx] = reflect.ValueOf(arr.AsInt(arridx,0))
	case reflect.Uint:
		params[paramidx] = reflect.ValueOf(uint(arr.AsInt(arridx,0)))
	case reflect.Int8:
		params[paramidx] = reflect.ValueOf(int8(arr.AsInt(arridx,0)))
	case reflect.Uint8:
		params[paramidx] = reflect.ValueOf(uint8(arr.AsInt(arridx,0)))
	case reflect.Uint16:
		params[paramidx] = reflect.ValueOf(uint16(arr.AsInt(arridx,0)))
	case reflect.Int16:
		params[paramidx] = reflect.ValueOf(int16(arr.AsInt(arridx,0)))
	case reflect.Int32:
		params[paramidx] = reflect.ValueOf(arr.AsInt32(arridx,0))
	case reflect.Uint32:
		params[paramidx] = reflect.ValueOf(uint32(arr.AsInt32(arridx,0)))
	case reflect.Uint64:
		params[paramidx] = reflect.ValueOf(uint64(arr.AsInt64(arridx,0)))
	case reflect.Int64:
		params[paramidx] = reflect.ValueOf(int64(arr.AsInt64(arridx,0)))
	case reflect.Float32:
		params[paramidx] = reflect.ValueOf(arr.AsDouble(arridx,0))
	case reflect.Float64:
		params[paramidx] = reflect.ValueOf(arr.AsFloat(arridx,0))
	case reflect.String:
		params[paramidx] = reflect.ValueOf(arr.AsString(arridx,""))
	case reflect.Interface:
		switch arr.VaueTypeByIndex(arridx) {
		case DxValue.DVT_Bool:
			params[paramidx] = reflect.ValueOf(arr.AsBool(arridx,false))
		}
	case reflect.Bool:
		params[paramidx] = reflect.ValueOf(arr.AsBool(arridx,false))
	case reflect.Map:
	case reflect.Struct:
		
	case reflect.Ptr:
		rtype := tp.Elem()
		setarrV(params,arr,paramidx,arridx,rtype)
	}
}

func (mnd *rpcMethod)call(pkg *RpcPkg)  {
	params := pkg.pkgData.AsBaseValue("Params")
	if params != nil{
		switch params.ValueType() {
		case DxValue.DVT_Record:
			//那么应该是只有2个参数，一个输入，一个输出
		case DxValue.DVT_Array:
			arr,_ := params.AsArray()
			if arr.Length() != len(mnd.paramType){
				//错误
				return
			}
			paramcount := arr.Length()
			var params []reflect.Value
			idx := 0
			if mnd.InstanceV != nil{
				params = make([]reflect.Value,paramcount+1)
				params[0] = *mnd.InstanceV
				idx = 1
			}else if paramcount > 0{
				params = make([]reflect.Value,paramcount)
			}
			for i := 0;i<paramcount;i++{
				setarrV(params,arr,idx,i,mnd.paramType[i])
				idx++
			}
			values := mnd.funcValue.Call(params)

			if values != nil{

			}
		}
	}
}



type _RpcServer struct {
	ServerBase.DxTcpServer
	fmethods					sync.Map			 //客户请求的方法处理
	fRpcWaitReturnMethods		sync.Map			 //等待返回的RPC方法
	fMethodResultHandlers		sync.Map			 //自己发送的方法的还行结果返回
	fHandler					[]*reflect.Value
	fFuncTypes					map[reflect.Type][]reflect.Type
}

func (server *_RpcServer)findReg(serverHandle interface{})  bool{
	for _,v := range server.fHandler{
		if v.Interface() == serverHandle{
			return true
		}
	}
	return false
}

//注册RPC处理函数
func (server *_RpcServer)RegisterHandle(serverHandle interface{})  {
	rtype := reflect.TypeOf(serverHandle)
	oldtype := rtype
	for rtype.Kind() == reflect.Ptr{
		oldtype = rtype
		rtype = rtype.Elem()
	}
	switch rtype.Kind() {
	case reflect.Func:
		//是函数，那么存放，函数值
		//rtype.Name()

	case reflect.Struct:
		if server.findReg(serverHandle){
			return
		}
		mndcount := oldtype.NumMethod()
		if mndcount == 0{
			return
		}
		v := reflect.ValueOf(serverHandle)
		if server.fHandler == nil{
			server.fHandler = make([]*reflect.Value,0,100)
			server.fHandler = append(server.fHandler,&v)
		}
		//注册内容
		for i := 0;i < mndcount;i++{
			mnd := oldtype.Method(i)
			var rpMethod *rpcMethod
			vmnd,ok := server.fmethods.Load(mnd.Name)
			if ok {
				rpMethod = vmnd.(*rpcMethod)
			}else{
				rpMethod = new(rpcMethod)
				rpMethod.Name = mnd.Name
			}
			rpMethod.InstanceV = &v
			functype := mnd.Func.Type()
			if paramtype,ok := server.fFuncTypes[functype];ok{
				rpMethod.paramType = paramtype
			}else{
				paramtype = make([]reflect.Type,functype.NumIn()-1)
				for j := 1;j<functype.NumIn();j++{
					paramtype[j-1] = functype.In(j)
				}
				rpMethod.paramType = paramtype
			}
			rpMethod.funcValue = mnd.Func
		}
	}
}

func NewRpcServer()*_RpcServer  {
	srv := new(_RpcServer)
	srv.LimitSendPkgCount = 0
	srv.MaxDataBufCount = 500
	srv.OnClientConnect = func(con *ServerBase.DxNetConnection)interface{} {
		return nil
	}
	srv.OnRecvData = func(con *ServerBase.DxNetConnection,recvData interface{}){
		methodpkg := recvData.(*RpcPkg)
		defer func(){
			if err := recover();err!=nil{
				/*methodpkg.PkgData().Delete("Params")
				methodpkg.PkgData().Delete("Result")
				methodpkg.fReturnResult = true//立即返回
				methodpkg.PkgData().SetString("Err",fmt.Sprintf("%v",err))*/
			}
		}()
		methodName := methodpkg.MethodName()
		pktType := RpcPkgType(methodpkg.PkgData().AsInt("Type",int(RPT_UnKnown)))
		if  pktType == RPT_Result { //是返回结果，做结果处理
			//获取结果，返回
			methodid := methodpkg.MethodID()
			var (
				runMethod *RpcPkg
				resulthandler *rpcMethod
			)
			if vhandle,ok := srv.fRpcWaitReturnMethods.Load(methodid);ok{
				runMethod = vhandle.(*RpcPkg)
			}
			if vresulthandler,ok := srv.fMethodResultHandlers.Load(methodName);ok{
				resulthandler = vresulthandler.(*rpcMethod)
			}
			if resulthandler != nil { //执行结果处理函数
				//resulthandler.funcValue.Call()
			}
			if runMethod != nil {
				runMethod.CloseWaitChan() //关闭等待
				if runMethod.CloseWaitChan(){
					pkgdata := runMethod.PkgData()
					runMethod.ReSetPkgData(methodpkg.PkgData())
					methodpkg.ReSetPkgData(pkgdata)
				}
				srv.fRpcWaitReturnMethods.Delete(methodid)
			}
			FreeMethod(methodpkg)
			return
		}
		if vresulthandler,ok := srv.fmethods.Load(methodName);ok{
			handler := vresulthandler.(*rpcMethod)
			if pktType == RPT_Notify{ //通知，不用返回的
				handler.call(methodpkg)
				FreeMethod(methodpkg)
				return
			}
			handler.call(methodpkg)
			if methodpkg.ReturnResult(){
				methodpkg.PkgData().SetInt("Type",int(RPT_Result)) //作为结果返回
				con.WriteObject(methodpkg) //发送结果回去
			}
		}else{
			//logger.Debugln("未处理的函数：",methodpkg.MethodName)
			FreeMethod(methodpkg)
		}
	}

	return srv
}