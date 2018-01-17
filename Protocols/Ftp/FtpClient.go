/*
实现了FTP客户端
Autor: 不得闲
QQ:75492895
 */
package Ftp

import (
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"github.com/suiyunonghen/DxCommonLib"
	"bytes"
	"time"
	"github.com/landjur/golibrary/errors"
	"strings"
	"strconv"
	"fmt"
	"sync"
	"os"
)

var(
	ErrNoFtpServer = errors.New("Wait No Connect Response Info,maybe not a FtpServer")
	ErrLastCmdUnOk = errors.New("Previous Command not complete")
	ErrConnectionLost = errors.New("Connection Lost")
)

type TransDataMode uint8

type FTPFile		struct{
	Mode			os.FileMode
	ModifyTime		time.Time
	Name			string
	Attribute		string
	User			string
	Group			string
	Size			int
}

type FtpClient struct {
	ServerBase.DxTcpClient
	tmpBuffer			*bytes.Buffer
	curMulResponsMsg	*ftpResponsePkg
	cmdResultChan		chan*ftpResponsePkg
	cmdResultCount		uint8
	connectOk			chan struct{}
	transOk				chan struct{}
	OnResultResponse	func(responseCode uint16, msg string)
	fDataMode			TransDataMode
	portClientPool		sync.Pool
	ftpClientBinds
}


const(
	TDM_AUTO		TransDataMode = iota
	TDM_PASV
	TDM_PORT
)

var shortMonthNames = []string{
	"---",
	"JAN",
	"FEB",
	"MAR",
	"APR",
	"MAY",
	"JUN",
	"JUL",
	"AUG",
	"SEP",
	"OCT",
	"NOV",
	"DEC",
}


func (ftpclient *FtpClient)SubInit()  {
	ftpclient.DxTcpClient.SubInit()
	ftpclient.GDxBaseObject.SubInit(ftpclient)
}

//执行命令
func (ftpclient *FtpClient)ExecuteFtpCmd(cmd,param string,cmdResultCount uint8)(responspkg *ftpResponsePkg, e error)  {
	ftpclient.tmpBuffer.WriteString(cmd)
	if len(param)>0{
		ftpclient.tmpBuffer.WriteByte(' ')
		ftpclient.tmpBuffer.WriteString(param)
	}
	ftpclient.tmpBuffer.WriteString("\r\n")
	ftpclient.cmdResultCount = cmdResultCount
	if ftpclient.SendBytes(ftpclient.tmpBuffer.Bytes()){
		done := ftpclient.Done()
		select{
		case <-done:
			e = ErrConnectionLost
			ftpclient.ftpClientBinds.isLogin = false
		case responspkg = <-ftpclient.cmdResultChan:
			if ftpclient.OnResultResponse != nil{
				ftpclient.OnResultResponse(responspkg.responseCode,responspkg.responseMsg)
			}
		}
	}else{
		e = ErrConnectionLost
	}
	ftpclient.tmpBuffer.Reset()
	return
}

func (ftpclient *FtpClient)Login(uid,pwd string)error  {
	respkg,err := ftpclient.ExecuteFtpCmd("USER",uid,1)
	if err != nil{
		return err
	}
	if respkg.responseCode == 331{ //要求用户输入密码
		respkg,err = ftpclient.ExecuteFtpCmd("PASS",pwd,1)
		if err != nil{
			return err
		}
		if respkg.responseCode == 230{
			ftpclient.ftpClientBinds.isLogin = true
			return nil
		}
	}
	return errors.New(respkg.responseMsg)
}


//下载文件
func (ftpclient *FtpClient)DownLoad(fileName string)error{
	if ftpclient.fDataMode == TDM_AUTO || ftpclient.fDataMode == TDM_PASV{
		//优先启用被动模式
		respkg,err := ftpclient.ExecuteFtpCmd("PASV","",1)
		if err != nil{
			return err
		}
		if respkg.responseCode != 227{
			return errors.New(respkg.responseMsg)
		}
	}else{
		//主动模式，先开启一个数据接收服务端
	}
	return nil
}

func (ftpclient *FtpClient)ListDir(dirName string,listfunc func(ftpFileinfo *FTPFile))error  {
	var (
		respkg *ftpResponsePkg
		err error
		buffer *bytes.Buffer
	)
	oldmode := ftpclient.fDataMode
	redo:
	if ftpclient.fDataMode == TDM_AUTO || ftpclient.fDataMode == TDM_PASV{
		//优先启用被动模式
		respkg,err = ftpclient.ExecuteFtpCmd("PASV","",1)
		if err != nil{
			return err
		}
		if respkg.responseCode != 227{
			return errors.New(respkg.responseMsg)
		}
		nums := strings.Split(respkg.responseMsg, ",")
		portOne, _ := strconv.Atoi(nums[4])
		tmpstr := nums[5]
		idx := strings.IndexByte(tmpstr,')')
		if idx > -1{
			tmpstr = string([]byte(tmpstr)[:idx])
		}
		portTwo, _ := strconv.Atoi(tmpstr)
		port := (portOne * 256) + portTwo

		tmpstr = nums[0]
		idx = strings.IndexByte(tmpstr,'(')
		if idx > -1{
			tmpstr = string([]byte(tmpstr)[idx+1:])
		}

		PortHost := fmt.Sprintf("%s.%s.%s.%s:%d",tmpstr,nums[1],nums[2],nums[3],port)
		//需要连接上去
		dataclient := ftpclient.getDataClient(&ftpclient.ftpClientBinds)
		dataclient.clientbind.waitReadChan = make(chan struct{}) //等待读取
		if err := dataclient.Connect(PortHost);err!=nil{
			close(dataclient.clientbind.waitReadChan)
			dataclient.clientbind.waitReadChan = nil
			ftpclient.freeDataClient(dataclient)
			//准备转到Port模式
			ftpclient.fDataMode = TDM_PORT
			goto redo
		}
		//发送List指令
		buffer = bytes.NewBuffer(make([]byte,0,4096))
		dataclient.clientbind.f = buffer
		close(dataclient.clientbind.waitReadChan)
	}else{
		//主动模式，先开启一个数据接收服务端

	}
	//等待返回数据
	transOk := make(chan struct{})
	ftpclient.transOk = transOk
	done := ftpclient.Done()
	respkg,err = ftpclient.ExecuteFtpCmd("LIST",dirName,2) //第二个返回才是结果
	if err != nil{
		ftpclient.fDataMode = oldmode
		return err
	}
	if respkg.responseCode == 226{
		select{
		case <-done:
			return ErrConnectionLost
		case <-transOk:
			now := time.Now()
			year := 0
			fileinfo := &FTPFile{}
			for{
				linebyte,err := buffer.ReadBytes('\n')
				linebyte = bytes.Trim(linebyte,"\r\n")
				fileInfos := strings.Fields(string(linebyte))
				if len(fileInfos)>0{
					fileinfo.Attribute = fileInfos[0]
					fileinfo.User = fileInfos[2]
				 	fileinfo.Group = fileInfos[3]
					fileinfo.Mode = DxCommonLib.ModePermStr2FileMode(fileInfos[0])
					if fileinfo.Mode.IsDir(){
						fileinfo.Size = 0
					}else{
						fileinfo.Size,_ = strconv.Atoi(fileInfos[4])
					}

					if shortMonthNames[now.Month()] != strings.ToUpper(fileInfos[5]){
						year = now.Year() - 1
					}else{
						year = now.Year()
					}
					day,_ := strconv.Atoi(fileInfos[6])
					index := strings.IndexByte(fileInfos[7],':')
					hour,_ := strconv.Atoi(string([]byte(fileInfos[7])[:index]))
					mint,_ := strconv.Atoi(string([]byte(fileInfos[7])[index+1:]))
					fileinfo.ModifyTime = time.Date(year,now.Month(),day,hour,mint,0,0,time.Local)
					fileinfo.Name = fileInfos[len(fileInfos)-1]
					if listfunc != nil{
						listfunc(fileinfo)
					}
				}
				if err != nil{
					break
				}
			}
			return nil
		}
	}
	return errors.New(respkg.responseMsg)
}


func (ftpclient *FtpClient) getDataClient(client *ftpClientBinds)(dataclient *ftpDataClient)  {
	v := ftpclient.portClientPool.Get()
	if v == nil{
		dataclient = new(ftpDataClient)
		dataclient.clientPools = ftpclient
		dataclient.SubInit()
		dataclient.clientbind.portDataClient = dataclient
		dataclient.dataBuffer = bytes.NewBuffer(make([]byte,0,44*1460))
		dataclient.AfterClientDisConnected = dataclient.onDataClientClose
	}else{
		dataclient = v.(*ftpDataClient)
		dataclient.dataBuffer.Reset()
	}
	dataclient.clientbind.ftpClient = client
	dataclient.Clientcon.SetUseData(&dataclient.clientbind)
	dataclient.clientbind.waitReadChan = make(chan struct{}) //等待可读
	return
}

func (ftpclient *FtpClient) freeDataClient(client *ftpDataClient)  {
	client.clientbind.waitReadChan = nil
	client.clientbind.ftpClient = nil
	client.clientbind.f = nil
	client.clientbind.curPosition = 0
	client.clientbind.lastFilePos = 0
	ftpclient.portClientPool.Put(client)
}

func (ftpclient *FtpClient)Connect(addr string)error  {
	if ftpclient.Active(){
		ftpclient.Close()
	}
	conOk := make(chan struct{})
	ftpclient.connectOk = conOk
	if ftpclient.cmdResultChan == nil{
		ftpclient.cmdResultChan = make(chan*ftpResponsePkg)
	}
	ftpclient.ftpClientBinds.cmdcon = &ftpclient.Clientcon
	ftpclient.ftpClientBinds.typeAnsi = false
	ftpclient.Clientcon.SetUseData(ftpclient)
	ftpclient.ftpClientBinds.isLogin = false
	err := ftpclient.DxTcpClient.Connect(addr)
	if err != nil{
		close(ftpclient.connectOk)
		ftpclient.connectOk = nil
		return err
	}
	select{
	case <-conOk:
		return nil
	case <-DxCommonLib.After(5*time.Second):
		ftpclient.connectOk = nil
		close(conOk)
		ftpclient.Close()
		return ErrNoFtpServer
	}
}

func NewFtpClient(dataMode TransDataMode)*FtpClient  {
	result := new(FtpClient)
	result.fDataMode = dataMode
	result.SetCoder(&FtpProtocol{true})
	result.tmpBuffer = bytes.NewBuffer(make([]byte,0,1024))
	result.OnSendHeart = func(con *ServerBase.DxNetConnection) {
		//发送心跳保活
		result.ExecuteFtpCmd("NOOP","",1)
	}
	result.OnRecvData = func(con *ServerBase.DxNetConnection, recvData interface{}) {
		//执行命令返回的
		respkg := recvData.(*ftpResponsePkg)
		if result.cmdResultCount--;result.cmdResultCount <= 0{
			result.cmdResultCount = 0
			result.cmdResultChan <- respkg
		}else if result.OnResultResponse != nil{
			result.OnResultResponse(respkg.responseCode,respkg.responseMsg)
		}
	}
	return result
}