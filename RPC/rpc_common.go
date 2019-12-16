package RPC

import (
	"github.com/suiyunonghen/DxValue"
	"unsafe"
	"bytes"
	"github.com/suiyunonghen/DxValue/Coders/DxMsgPack"
	"io"
	"sync"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"github.com/bwmarrin/snowflake"
)
const (
	RPT_UnKnown		RpcPkgType=iota
	RPT_Method
	RPT_Notify
	RPT_Result
)
var(
	pkgpool 		sync.Pool
	node 			*snowflake.Node
)

type RpcPkgType	byte
type MethodHandler func(*ServerBase.DxNetConnection, *RpcPkg)
type RpcCoder struct {
	maxBufSize		uint16
}

func init()  {
	node,_ = snowflake.NewNode(1)
}


func SnowFlakeID()int64  {
	id := node.Generate()
	return id.Int64()
}

func GetMethod(methodName string,IsNotify bool,MethodId int64)*RpcPkg  {
	result := pkgpool.Get()
	var method *RpcPkg
	if result == nil {
		method = new(RpcPkg)
	}else{
		method = result.(*RpcPkg)
	}
	if method.pkgData == nil{
		method.pkgData = DxValue.NewRecord()
	}
	method.pkgData.SetInt64("ID",MethodId)
	method.fReturnResult = true
	method.pkgData.SetString("Name",methodName)
	if IsNotify{
		method.pkgData.SetInt("Type",int(RPT_Notify))
	}else{
		method.pkgData.SetInt("Type",int(RPT_Method))
	}
	return method
}


func FreeMethod(method *RpcPkg)  {
	method.ReSet()
	pkgpool.Put(method)
}
//命令编码规则
func (coder *RpcCoder)Encode(obj interface{},buf io.Writer) error {
	encoder := DxValue.NewEncoder(buf)
	err := encoder.EncodeRecord(obj.(*RpcPkg).pkgData)
	DxValue.FreeEncoder(encoder)
	return err
}

func (coder *RpcCoder)Decode(pkgbytes []byte)(result interface{},ok bool)  {
	methodpkg := GetMethod("",false,-1)
	buf := bytes.NewReader(pkgbytes[:])
	decoder := DxValue.NewDecoder(buf)
	if err := decoder.DecodeStrMap(DxMsgPack.CodeUnkonw,methodpkg.pkgData);err!=nil{
		result = nil
		FreeMethod(methodpkg)
		ok = false
	}
	DxValue.FreeDecoder(decoder)
	result = methodpkg
	ok = true
	return
}

func (coder *RpcCoder)HeadBufferLen()uint16  {
	return 2
}

func (coder *RpcCoder)MaxBufferLen()uint16  {
	if coder.maxBufSize > 0{
		return coder.maxBufSize
	}
	return 1024
}

func (coder *RpcCoder)UseLitterEndian()bool{
	return false
}

type RpcPkg struct {
	pkgData			*DxValue.DxRecord
	fId				*DxValue.DxInt64Value
	fName			*DxValue.DxStringValue
	fHasWait		bool
	fWaitchan		chan *DxValue.DxRecord		//等待的通道
	fReturnResult	bool			//是否返回执行结果
	fresultHandler	MethodHandler	//执行等待结果返回的函数
	TargetData		interface{}
}

func (pkg *RpcPkg)SetParams(params map[string]interface{})  {
	if len(params)!=0{
		paramRecord := pkg.pkgData.NewRecord("Params",true)
		for k,v := range params{
			paramRecord.SetValue(k,v)
		}
	}else{
		pkg.pkgData.Delete("Params")
	}
}

func (pkg *RpcPkg)Params()*DxValue.DxBaseValue  {
	return pkg.pkgData.AsBaseValue("Params")
}

func (pkg *RpcPkg)Result()*DxValue.DxBaseValue  {
	return pkg.pkgData.AsBaseValue("Result")
}

func (pkg *RpcPkg)SetArrParams(params []interface{})  {
	if len(params)!=0{
		paramArr := pkg.pkgData.NewArray("Params",true)
		for idx,v := range params{
			paramArr.SetValue(idx,v)
		}
	}else{
		pkg.pkgData.Delete("Params")
	}
}

func (pkg *RpcPkg)SetResult(result interface{})  {
	if result != nil{
		pkg.pkgData.SetValue("Result",result)
	}else{
		pkg.pkgData.Delete("Result")
	}
}

func (pkg *RpcPkg)WaitChan()chan *DxValue.DxRecord {
	if pkg.fWaitchan == nil{
		pkg.fWaitchan = make(chan *DxValue.DxRecord)
	}
	pkg.fHasWait = true
	return pkg.fWaitchan
}

func (pkg *RpcPkg)CloseWaitChan(waitresult *DxValue.DxRecord)bool  {
	if pkg.fWaitchan != nil{
		pkg.fWaitchan <- waitresult
		return true
	}
	return false
}

func (pkg *RpcPkg)HasWait()bool  {
	return pkg.fHasWait
}

func (pkg *RpcPkg)ReturnResult()bool  {
	return pkg.fReturnResult
}

func (pkg *RpcPkg)SetReturnResult(v bool)  {
	pkg.fReturnResult = v
}

func (pkg *RpcPkg)SetCanRecive(v bool)  {
	pkg.fReturnResult = v
}

func (pkg *RpcPkg)ReSet()  {
	pkg.fName = nil
	pkg.fId = nil
	pkg.fHasWait = false
	pkg.fresultHandler = nil
	if pkg.pkgData != nil{
		pkg.pkgData.SetInt("Type",int(RPT_UnKnown))
		bvalue := pkg.pkgData.AsBaseValue("Params")
		if bvalue != nil{
			bvalue.ClearValue(false)
		}
		pkg.pkgData.Delete("Result")
		pkg.pkgData.Delete("Err")
	}
	pkg.fReturnResult = true //默认都返回执行结果
	if pkg.fWaitchan != nil{
		close(pkg.fWaitchan)
		pkg.fWaitchan = nil
	}
}

func (pkg *RpcPkg)SetError(errmsg string)  {
	pkg.pkgData.SetString("Err",errmsg)
}

func (pkg *RpcPkg)ClearParams()  {
	bvalue := pkg.pkgData.AsBaseValue("Params")
	if bvalue != nil{
		bvalue.ClearValue(false)
	}
}

func (pkg *RpcPkg)PkgData()*DxValue.DxRecord  {
	return pkg.pkgData
}

func (pkg *RpcPkg)ReSetPkgData(newpkg *DxValue.DxRecord)  {
	pkg.pkgData = newpkg
}

func (pkg *RpcPkg)MethodID()int64  {
	if pkg.fId == nil{
		v  := pkg.pkgData.AsBaseValue("ID")
		if v != nil{
			pkg.fId = (*DxValue.DxInt64Value)(unsafe.Pointer(v))
		}else{
			p := uintptr(1)
			pkg.fId = (*DxValue.DxInt64Value)(unsafe.Pointer(p))
		}
	}
	if pkg.fId != nil && uintptr(unsafe.Pointer(pkg.fId)) != 1{
		return pkg.fId.Int64()
	}
	return -1
}

func (pkg *RpcPkg)MethodName()string  {
	if pkg.fName == nil{
		v  := pkg.pkgData.AsBaseValue("Name")
		if v != nil{
			pkg.fName = (*DxValue.DxStringValue)(unsafe.Pointer(v))
		}else{
			p := uintptr(1)
			pkg.fName = (*DxValue.DxStringValue)(unsafe.Pointer(p))
		}
	}
	if pkg.fName != nil && uintptr(unsafe.Pointer(pkg.fName)) != 1{
		return pkg.fName.ToString()
	}
	return ""
}

