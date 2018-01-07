package Filesrv
import (
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"encoding/json"
	"bytes"
	"fmt"
	"io"
	"os"
	"encoding/base64"
	"github.com/ugorji/go/codec"
	"encoding/binary"
)

type IEncoder interface {
	Encode(v interface{}) error
}
type IDecoder interface {
	Decode(v interface{}) error
}

type FileSrvCoder struct {
	fuseBinary	bool
}

type MethodPkg struct {
	MethodName	string
	MethodId	string
	Param		map[string]interface{}
}

type ResultPkg struct {
	Result		interface{}
	MethodName	string
	MethodId	string
	TotalLen	int64
	Err		string
	refData		*bytes.Buffer
}



func (coder *FileSrvCoder)Encode(obj interface{},w io.Writer) error {
	var  encoder  IEncoder
	if coder.fuseBinary{
		var msgh codec.MsgpackHandle
		msgh.WriteExt = true
		encoder = codec.NewEncoder(w,&msgh)
	}else{
		encoder = json.NewEncoder(w)
	}
	return encoder.Encode(obj)
}

func (coder *FileSrvCoder)Decode(pkgbytes []byte)(result interface{},ok bool)  {
	buf := bytes.NewReader(pkgbytes[:])
	var decoder IDecoder
	if coder.fuseBinary{
		var msgh codec.MsgpackHandle
		msgh.WriteExt = true
		decoder = codec.NewDecoder(buf,&msgh)
	}else{
		decoder = json.NewDecoder(buf)
	}
	methodpkg := new(MethodPkg)
	ok = decoder.Decode(methodpkg) == nil
	if ok{
		result = methodpkg
	}else{
		result = nil
	}
	return
}

func (coder *FileSrvCoder)HeadBufferLen()uint16  {
	return 2
}

func (coder *FileSrvCoder)MaxBufferLen()uint16  {
	return 44*1460
}

func (coder *FileSrvCoder)UseLitterEndian()bool  {
	return false
}


type DxFileServer struct {
	ServerBase.DxTcpServer
	fReadingFile	map[string]map[uint]*ServerBase.DxNetConnection //正在读的文件列表
	fWritingFile	map[string]*ServerBase.DxNetConnection //正在写的文件列表
	fLockingQueue	map[*ServerBase.DxNetConnection]string //锁定了内容的锁定队列
	filedir		string
	fresultQueue	chan *ResultPkg
}

func FileExists(filename string)bool  {
	_,err := os.Stat(filename)
	return err==nil
}

func (srv *DxFileServer)lockFile(filename string,ReadLock bool,con *ServerBase.DxNetConnection,result *ResultPkg)  {
	//锁定一个文件，锁定期间，不允许其他的访问,如果锁定成功，返回文件名，否则返回错误
	srv.Lock()
	if ReadLock{ //读文件锁定，写入到文件正在读的列表中去,那么这个文件不可写锁定
		//先判断文件是否写锁定，如果写锁定，就返回
		if _,ok :=srv.fWritingFile[filename];ok{
			result.Err = "文件正在写入占用，无法执行读锁定"
			result.Result = 1
		}else if v,ok := srv.fReadingFile[filename];ok{
			if _,ok = v[con.ConHandle];!ok{ //不存在，则写入
				if FileExists(filename){
					v[con.ConHandle] = con
					srv.fLockingQueue[con]=filename
				}else{
					result.Result = 3
					result.Err ="文件不存在"
				}
			}
		}else if FileExists(filename){
			conmap := make(map[uint]*ServerBase.DxNetConnection)
			conmap[con.ConHandle] =con
			srv.fReadingFile[filename]=conmap
			srv.fLockingQueue[con]=filename
		}else{
			result.Result = 3
			result.Err ="文件不存在"
		}
	}else{//写锁定，那么就无法读取文件，也无法写入文件
		if _,ok := srv.fWritingFile[filename];ok{
			result.Err = "文件正在写入占用，无法执行写锁定"
		}else if v,ok:=srv.fReadingFile[filename];ok{
			//文件正在读列表中，判断读列表是否为空
			if len(v)==0{
				delete(srv.fReadingFile,filename)
				srv.fWritingFile[filename]=con
				srv.fLockingQueue[con]=filename
			}else{
				result.Err = "文件正在读占用锁定，无法执行写锁定"
				result.Result = 2
			}
		}else{
			srv.fWritingFile[filename]=con
			srv.fLockingQueue[con]=filename
		}
	}
	srv.Unlock()
}


func (srv *DxFileServer)unlockFile(filename string,ReadunLock bool,con *ServerBase.DxNetConnection)  {
	srv.Lock()
	if ReadunLock{ //读文件锁定解锁
		if v,ok := srv.fReadingFile[filename];ok{
			if _,ok := v[con.ConHandle];ok{
				delete(v,con.ConHandle)
				delete(srv.fLockingQueue,con)
				if len(v)==0{
					delete(srv.fReadingFile,filename)
				}
			}
		}
	}else if _,ok := srv.fWritingFile[filename];ok{
		delete(srv.fWritingFile,filename)
		delete(srv.fLockingQueue,con)
	}
	srv.Unlock()
}

func (srv *DxFileServer)download(filename string,fsize int64,position int64,result *ResultPkg,con *ServerBase.DxNetConnection)  {
	coder := srv.GetCoder().(*FileSrvCoder)
	if position>=fsize{
		result.Err = "已经读取到尾部"
	}else if file,err := os.Open(filename);err==nil{
		result.TotalLen = fsize
		datalen := fsize - position
		var blocklen int64
		if coder.fuseBinary{
			blocklen = 43 * 1460
		}else{
			blocklen = 21 * 1460
		}
		if datalen > blocklen{
			datalen = blocklen
		}
		result.refData = srv.GetBuffer(0)
		bt := result.refData.Bytes()
		file.Seek(position,os.SEEK_SET)
		file.Read(bt[:datalen])
		if coder.fuseBinary{
			result.Result = []uint8(bt[:datalen])
		}else{
			result.Result = base64.StdEncoding.EncodeToString(bt[:datalen]) //返回内容
		}
		file.Close()
	}else{
		result.Err = err.Error()
	}
}

func (srv *DxFileServer)getUpdateVersion(versionFile string,result *ResultPkg,con *ServerBase.DxNetConnection) {
	if file,err := os.Open(versionFile);err==nil{
		result.refData = srv.GetBuffer(0)
		bt := result.refData.Bytes()
		_,err := file.Read(bt[:2]) //读取头部信息
		if err == nil && bt[0]=='U' && bt[1]=='P'{
			if _,err = file.Read(bt[2:4]);err == nil{//读取更新的文件数据类型的长度
				verdatalen := binary.BigEndian.Uint16(bt[2:4])
				if _,err = file.Read(bt[:verdatalen]);err == nil{//读取实际的版本数据
					result.Result = []uint8(bt[:verdatalen])
				}else{
					result.Err =  err.Error()
					result.Result = 3
				}
			}else{
				result.Err = err.Error()
				result.Result = 3
			}
		}else{
			result.Err = "无效的更新文件类型"
			result.Result = 4
		}
		file.Close()
	}else{
		result.Err = err.Error()
		result.Result = 3
	}
}

func (srv *DxFileServer)upload(filename string,position int64,con *ServerBase.DxNetConnection,filedata interface{},result *ResultPkg)  {
	var  fileobj *os.File
	var err error
	if position == 0{
		fileobj,err = os.Create(filename)
	}else{
		fileobj,err = os.OpenFile(filename,os.O_CREATE|os.O_WRONLY,0)
	}
	if err!=nil{
		result.Result = 4
		result.Err = err.Error()
		return
	}
	if _,err := fileobj.Seek(position,os.SEEK_SET);err==nil{
		var objbyte []byte
		switch filedata.(type) {
		case []byte:
			objbyte = filedata.([]byte)
		case string://Base64解码
			objbyte,err = base64.StdEncoding.DecodeString(filedata.(string))
		}
		if err != nil{
			result.Result = 4
			result.Err = err.Error()
		}else if wl,err := fileobj.Write(objbyte);err != nil{
			result.Result = 4
			result.Err = err.Error()
		}else{
			result.TotalLen = int64(wl) + position
		}
	}else{
		result.Result = 4
		result.Err = err.Error()
	}
	fileobj.Close()
}


func NewFileServer(filedir string)*DxFileServer{
	srv := new(DxFileServer)
	srv.filedir = filedir
	srv.LimitSendPkgCount = 10
	srv.MaxDataBufCount = 500
	srv.fresultQueue = make(chan *ResultPkg,200)
	coder := new(FileSrvCoder)
	coder.fuseBinary = true
	srv.fReadingFile = make(map[string]map[uint]*ServerBase.DxNetConnection)
	srv.fWritingFile = make(map[string]*ServerBase.DxNetConnection)
	srv.fLockingQueue =make(map[*ServerBase.DxNetConnection]string)
	srv.SetCoder(coder)
	srv.OnRecvData = func(con *ServerBase.DxNetConnection,recvData interface{}){
		methodpkg := recvData.(*MethodPkg)
		result := new(ResultPkg)
		result.MethodName = methodpkg.MethodName
		result.MethodId = methodpkg.MethodId
		result.Result = 0
		var filename string
		if coder.fuseBinary{
			filename = fmt.Sprintf("%s/%s",srv.filedir,string(methodpkg.Param["FileName"].([]byte)))
		}else{
			filename = fmt.Sprintf("%s/%s",srv.filedir,methodpkg.Param["FileName"].(string))
		}
		switch methodpkg.MethodName {
		case "LockFile":
			srv.lockFile(filename,methodpkg.Param["ReadLock"].(bool),con,result)
		case "UnLockFile":
			srv.unlockFile(filename,methodpkg.Param["ReadLock"].(bool),con)
		case "DownLoad":
			if fileinfo,err := os.Stat(filename);err==nil {
				fsize := fileinfo.Size()
				var position int64
				switch methodpkg.Param["Position"].(type) {
				case uint64:  position = int64(methodpkg.Param["Position"].(uint64))
				case int64:   position = methodpkg.Param["Position"].(int64)
				case float64: position = int64(methodpkg.Param["Position"].(float64))
				}
				srv.download(filename,fsize,position,result,con)
			}else{
				result.Err = "指定的文件不存在"
				result.Result = 3
			}
		case "UpLoad":
			var position int64
			switch methodpkg.Param["Position"].(type) {
			case uint64:  position = int64(methodpkg.Param["Position"].(uint64))
			case int64:   position = methodpkg.Param["Position"].(int64)
			case float64: position = int64(methodpkg.Param["Position"].(float64))
			}
			srv.upload(filename,position,con,methodpkg.Param["FileData"],result)//上传，写入文件
		case "GetUpdateVersion"://获得最新的版本信息，更新包是自己的打包格式
			srv.getUpdateVersion(filename,result,con)
		}
		con.WriteObject(result)
	}

	srv.OnSendData = func(con *ServerBase.DxNetConnection,Data interface{},sendlen int,sendok bool){
		//回收
		resultpkg := Data.(*ResultPkg)
		if resultpkg.refData != nil{
			srv.ReciveBuffer(resultpkg.refData)
			resultpkg.refData = nil
		}
		resultpkg.Result = nil
	}

	srv.OnClientDisConnected = func(con *ServerBase.DxNetConnection){
		//客户端断开，释放锁定的信息
		srv.Lock()
		if v,ok := srv.fLockingQueue[con];ok{
			if vread,ok := srv.fReadingFile[v];ok{
				if _,ok := vread[con.ConHandle];ok{
					delete(vread,con.ConHandle)
					if len(vread) ==0 {
						delete(srv.fReadingFile,v)
					}
				}
			}else if _,ok := srv.fWritingFile[v];ok{
				delete(srv.fWritingFile,v)
			}
			delete(srv.fLockingQueue,con)
		}
		srv.Unlock()
	}
	return srv
}