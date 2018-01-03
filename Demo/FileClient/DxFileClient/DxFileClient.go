package FileClient

import (
	"bytes"
	"github.com/ugorji/go/codec"
	"encoding/json"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"crypto/md5"
	"encoding/hex"
	"crypto/rand"
	"io"
	"encoding/base64"
	"time"
	"sync/atomic"
	"os"
	"fmt"
	"sync"
	"strings"
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
	refData		[]byte
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
	methodpkg := new(ResultPkg)
	ok = decoder.Decode(methodpkg)==nil
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

//生成32位md5字串
func GetMd5String(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func GetGuid() string {
	b := make([]byte, 48)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	return GetMd5String(base64.URLEncoding.EncodeToString(b))
}

type GOnUpDownFile func(client *FileClient,FileName string,TotalSize,Position int64)
type FileClient struct {
	ServerBase.DxTcpClient
	fWaitchan		chan bool
	fWaitMethodId		string
	curRemoteFile		string
	curLocalFileName	string
	curfilePosition		int64
	fbusystate		uint32
	curOpFile		*os.File
	curResultPkg		*ResultPkg
	fUpdownFiles		map[string]bool
	fDelayUpDownFiles	map[string]bool
	OnDownLoad		GOnUpDownFile
	sync.Mutex
}


func (client *FileClient)ExecuteMethod(MethodName string,Params map[string]interface{})  {
	pkgMap := new(MethodPkg)
	pkgMap.MethodName = MethodName
	pkgMap.MethodId = GetGuid()
	pkgMap.Param = Params
	client.SendData(&client.Clientcon,pkgMap)
}

func (client *FileClient)DownLoadFile(FileName,LocalFileName string)  {
	if atomic.LoadUint32(&client.fbusystate)==1{//正忙
		if client.fUpdownFiles == nil{
			client.fUpdownFiles = make(map[string]bool)
		}
		client.fUpdownFiles[fmt.Sprintf("%s=%s",FileName,LocalFileName)]=false //表示下载
	}else{
		params := make(map[string]interface{})
		params["ReadLock"]=true
		params["FileName"]=FileName
		if client.ExecuteWait("LockFile",params,30000) {//锁定文件
			if client.curResultPkg.Err!=""{
				if client.curResultPkg.Result != 3{//文件存在才加入到后期下载列表
					if client.fDelayUpDownFiles == nil{
						client.fDelayUpDownFiles = make(map[string]bool)
					}
					client.fDelayUpDownFiles[fmt.Sprintf("%s=%s",FileName,LocalFileName)]=false
				}
				client.curResultPkg = nil
				return
			}
			atomic.SwapUint32(&client.fbusystate,1)
			client.curLocalFileName = LocalFileName
			client.curRemoteFile = FileName
			delete(params,"ReadLock")
			params["Position"] = 0
			client.ExecuteMethod("DownLoad",params)
		}
	}
}

func (client *FileClient)ExecuteWait(MethodName string,Params map[string]interface{},WaitTime int32)bool  {
	//先发送数据，然后等待返回
	if WaitTime<=0{
		WaitTime = 5000
	}
	client.fWaitMethodId = GetGuid()
	pkgMap := new(MethodPkg)
	pkgMap.MethodId = client.fWaitMethodId
	pkgMap.MethodName = MethodName
	pkgMap.Param = Params
	client.SendData(&client.Clientcon,pkgMap)
	select {
	case <-client.fWaitchan:
		//返回了
		return true
	case <-time.After(time.Millisecond * time.Duration(WaitTime)):
		//超时了
		return false
	}
}

func (client *FileClient)doNextFile()  {
	//开始执行下一个
	client.Lock()
	defer client.Unlock()
	for k,isupload := range client.fUpdownFiles {
		delete(client.fUpdownFiles,k)
		if !isupload{ //下载
			rlocals := strings.Split(k,"=")
			client.curRemoteFile = rlocals[0]
			client.curLocalFileName = rlocals[1]
			params := make(map[string]interface{})
			params["ReadLock"]=true
			params["FileName"]=client.curRemoteFile
			if client.ExecuteWait("LockFile",params,30000) {
				if client.curResultPkg.Err!=""{
					if client.curResultPkg.Result != 3{//文件存在才加入到后期下载列表
						if client.fDelayUpDownFiles == nil{
							client.fDelayUpDownFiles = make(map[string]bool)
						}
						client.fDelayUpDownFiles[fmt.Sprintf("%s=%s",client.curRemoteFile,client.curLocalFileName)]=false
					}
					client.curResultPkg = nil
					return
				}
				delete(params,"ReadLock")
				params["Position"] = 0
				client.ExecuteMethod("DownLoad",params)
			}
		}
		return
	}
	atomic.SwapUint32(&client.fbusystate,0)
}

func (client *FileClient)doneCurFileOperate(readLock bool)  {
	if client.curOpFile != nil{
		client.curOpFile.Close()
		client.curOpFile = nil
	}
	params := make(map[string]interface{})
	params["ReadLock"]=readLock
	params["FileName"]=client.curRemoteFile
	client.ExecuteWait("UnLockFile",params,30000)//解锁
	client.curRemoteFile = ""
	client.curLocalFileName =""
	client.curfilePosition = 0
}

func (client *FileClient)parserRecvPkg(con *ServerBase.DxNetConnection, resultpkg *ResultPkg)  {
	switch resultpkg.MethodName {
	case "DownLoad":
		if resultpkg.Err==""{
			if client.curOpFile == nil{
				if client.curfilePosition == 0{
					if file,err := os.Create(client.curLocalFileName);err==nil{
						client.curOpFile = file
					}else{
						client.doNextFile()
					}
				}else if file,err := os.OpenFile(client.curLocalFileName,os.O_CREATE|os.O_WRONLY,0);err==nil{
					client.curOpFile = file
				}else{
					client.doNextFile()
				}
			}
			if client.curOpFile != nil{
				client.curOpFile.Seek(client.curfilePosition,os.SEEK_SET)
				wl,_ := client.curOpFile.Write([]byte(resultpkg.Result.([]uint8)))//写入内容
				client.curfilePosition += int64(wl)
				if client.OnDownLoad != nil{
					client.OnDownLoad(client,client.curRemoteFile,resultpkg.TotalLen,client.curfilePosition)
				}
				if client.curfilePosition == resultpkg.TotalLen{ //写入完成,解锁，然后执行下一个下载
					client.doneCurFileOperate(true)
					client.doNextFile()
				}else{//请求下一段数据内容
					params := make(map[string]interface{})
					params["Position"]=client.curfilePosition
					params["FileName"]=client.curRemoteFile
					client.ExecuteMethod("DownLoad",params)
				}
			}

		}else{
			client.doneCurFileOperate(true)
			client.doNextFile()
		}
	case "UpLoad":
	default:
		if resultpkg.MethodId == client.fWaitMethodId{
			client.curResultPkg = resultpkg
			client.fWaitMethodId = ""
			client.fWaitchan<-true //等到数据
		}
	}
}

func NewFileClient()*FileClient{
	client := new(FileClient)
	client.fWaitchan = make(chan bool)
	coder := new(FileSrvCoder)
	coder.fuseBinary = true
	client.SetCoder(coder)
	/*client.OnSendHeart =  func(con *dxserver.DxNetConnection){
		//发送心跳包
	}*/
	//接收数据返回
	client.OnRecvData = func(con *ServerBase.DxNetConnection,recvData interface{}){
		resultpkg := recvData.(*ResultPkg)
		go client.parserRecvPkg(con,resultpkg)
	}

	client.OnClientDisConnected = func(con *ServerBase.DxNetConnection){
		if client.curOpFile!=nil{
			client.curOpFile.Close()
		}
		client.curResultPkg = nil
		client.curRemoteFile = ""
		client.curfilePosition = 0
		client.curLocalFileName = ""
		client.fWaitMethodId = ""
		client.fDelayUpDownFiles = make(map[string]bool)
		client.fUpdownFiles = make(map[string]bool)
	}

	return client
}