/*
实现了FTP协议的协议包
Autor: 不得闲
QQ:75492895
 */
package Ftp
import (
	"io"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"github.com/suiyunonghen/DxCommonLib"
	"bytes"
	"strconv"
	"strings"
	"fmt"
	"runtime"
	"sync"
)

type(
	FtpProtocol  struct{}

	ftpcmdpkg struct{
		cmd			[]byte
		params		[]byte
	}

	//回复包
	ftpResponsePkg  struct{
		responseCode			uint16
		responseMsg				string
		hasEndMsg				bool
	}

	ftpUser		struct{
		UserID			string
		PassWord		string
		Permission 		int
		IsAnonymous		bool
	}

	ftpDirs		struct{
		ftpdirName			string
		localDirName		string
	}

	ftpClientBinds		struct{
		user			*ftpUser
		isLogin			bool
		curPath			string			//当前的路径位置
		typeAnsi		bool			//ANSIMode
	}

	FTPServer struct{
		ServerBase.DxTcpServer
		users				sync.Map
		ftpDirectorys		sync.Map
		WelcomeMessage		string
		anonymousUser		ftpUser
	}

	ftpcmd		interface{
		Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)
		IsFeatCmd()bool
		MustLogin()bool
	}

	cmdBase		struct{}
	cmdUser		struct{cmdBase}
	cmdSYST		struct{cmdBase}
	cmdPASS		struct{cmdBase}
	cmdFEAT		struct{cmdBase}
	cmdQUIT		struct{cmdBase}
	cmdPWD		struct{cmdBase}
	cmdTYPE		struct{cmdBase}
	cmdPASV		struct{cmdBase}
)

var (
	ftpCmds	= map[string]ftpcmd{
		"USER":			cmdUser{},
		"SYST":			cmdSYST{},
		"PASS":			cmdPASS{},
		"FEAT":			cmdFEAT{},
		"QUIT":			cmdQUIT{},
		"PWD":			cmdPWD{},
		"TYPE":			cmdTYPE{},
		"PASV":			cmdPASV{},
		}
	featCmds			string
)

func init() {
	for k, v := range ftpCmds {
		if v.IsFeatCmd() {
			featCmds = featCmds + " " + k + "\n"
		}
	}
}

func (cmd cmdBase)IsFeatCmd() bool{
	return false
}

func (cmd cmdBase)MustLogin() bool{
	return true
}

func (cmd cmdPASV)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//被动传输模式
	con.WriteObject(&ftpResponsePkg{500,"Commands not supported temporarily",false})
}


func (cmd cmdTYPE)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//查看或者设置当前的传输方式
	client := con.GetUseData().(*ftpClientBinds)
	paramstr = strings.ToUpper(paramstr)
	if paramstr == "A"{
		client.typeAnsi = true
		con.WriteObject(&ftpResponsePkg{200,"Type set to ASCII",false})
	}else if paramstr == "I"{
		client.typeAnsi = false
		con.WriteObject(&ftpResponsePkg{200,"Type set to binary",false})
	}else if paramstr == ""{
		if client.typeAnsi{
			con.WriteObject(&ftpResponsePkg{200,"Type is ASCII",false})
		}else{
			con.WriteObject(&ftpResponsePkg{200,"Type is binary",false})
		}
	}else{
		con.WriteObject(&ftpResponsePkg{500,"Type Can Only A or I",false})
	}
}

func (cmd cmdPWD)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	client := con.GetUseData().(*ftpClientBinds)
	con.WriteObject(&ftpResponsePkg{257,"\""+client.curPath+"\" is the current directory",false})
}

func (cmd cmdQUIT)MustLogin() bool{
	return false
}

func (cmd cmdQUIT)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	con.WriteObject(&ftpResponsePkg{221,"Byebye",false})
}

func (cmd cmdFEAT)MustLogin() bool{
	return false
}

func (cmd cmdFEAT)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//获取系统支持的CMD扩展命令
	con.WriteObject(&ftpResponsePkg{211,fmt.Sprintf("Extensions supported:\n%s", featCmds),true})
}

func (cmd cmdPASS)MustLogin() bool{
	return false
}

func (cmd cmdPASS)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//用户密码
	client := con.GetUseData().(*ftpClientBinds)
	client.isLogin = false
	if client.user.PassWord == paramstr{
		client.isLogin = true
		client.curPath = "/"
		con.WriteObject(&ftpResponsePkg{230,"user login success!",false})
	}else{
		if client.user.IsAnonymous && client.user.PassWord == ""{
			client.isLogin = true
			client.curPath = "/"
			con.WriteObject(&ftpResponsePkg{230,"user login success!",false})
		}else{
			con.WriteObject(&ftpResponsePkg{530,"Password error, user logon failed",false})
		}
	}
}

func (cmd cmdSYST)MustLogin() bool{
	return false
}

func (cmd cmdSYST)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//服务器远程系统的操作系统类型
	if strings.Compare(runtime.GOOS,"windows") == 0{
		con.WriteObject(&ftpResponsePkg{215,"Windows Type: L8",false})
	}else{
		con.WriteObject(&ftpResponsePkg{215,"UNIX Type: L8",false})
	}
}


func (cmd cmdUser)MustLogin() bool{
	return false
}

func (cmd cmdUser)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//用户登录，将记录用户的信息,绑定到用户信息上
	if strings.Compare(paramstr,srv.anonymousUser.UserID)==0{
		ftpbinds := new(ftpClientBinds)
		ftpbinds.isLogin = false
		ftpbinds.typeAnsi = true
		ftpbinds.user = &srv.anonymousUser
		con.SetUseData(ftpbinds)
		con.WriteObject(&ftpResponsePkg{331,"Please enter a password",false})
		return
	}
	if v,ok := srv.users.Load(paramstr);ok{
		ftpbinds := new(ftpClientBinds)
		ftpbinds.typeAnsi = true
		ftpbinds.isLogin = false
		ftpbinds.user = v.(*ftpUser)
		con.SetUseData(ftpbinds)
		con.WriteObject(&ftpResponsePkg{331,"Please enter a password",false})
		return
	}
	con.WriteObject(&ftpResponsePkg{530,"User does not exist and cannot log on",false})
}

func (coder *FtpProtocol)Encode(obj interface{},w io.Writer)error  {
	return nil
}

func (coder *FtpProtocol)Decode(bytes []byte)(result interface{},ok bool)  {
	return bytes,true
}

func (coder *FtpProtocol)HeadBufferLen()uint16  {
	return 0
}

func (coder *FtpProtocol)UseLitterEndian()bool  {
	return false
}

func (coder *FtpProtocol)MaxBufferLen()uint16  {
	return 0
}

//实现FTP协议接口
func (coder *FtpProtocol)ProtoName()string  {
	return "FTP"
}

func (coder *FtpProtocol)ParserProtocol(r *ServerBase.DxReader)(parserOk bool,datapkg interface{},e error) { //解析协议，如果解析成功，返回true，根据情况可以设定返回协议数据包
	linebyte,err := r.ReadBytes('\n')
	if linebyte == nil{
		return false,nil,nil
	}
	e = err
	if e != nil{
		return false,nil,e
	}
	parserOk = true
	bt :=  make([]byte,1)
	bt[0]=' '
	linebyte = bytes.Trim(linebyte,"\r\n")
	params := bytes.SplitN(linebyte,bt,2)
	cmdpkg := &ftpcmdpkg{bytes.ToUpper(params[0]),nil}
	if len(params) == 2{
		cmdpkg.params = params[1]
	}
	datapkg = cmdpkg
	return
}
func (coder *FtpProtocol)PacketObject(objpkg interface{},buffer *bytes.Buffer)([]byte,error) { //将发送的内容打包
	resp := objpkg.(*ftpResponsePkg)
	codemsg := strconv.Itoa(int(resp.responseCode))
	buffer.WriteString(codemsg)
	if !resp.hasEndMsg{
		buffer.WriteByte(' ')
		buffer.WriteString(resp.responseMsg)
	}else{
		buffer.WriteByte('-')
		buffer.WriteString(resp.responseMsg)
		buffer.WriteString("\r\n")
		buffer.WriteString(codemsg)
		buffer.WriteString(" END")
	}
	buffer.WriteString("\r\n")
	return buffer.Bytes(),nil
}

func NewFtpServer()*FTPServer  {
	result := new(FTPServer)
	result.SetCoder(new(FtpProtocol))
	result.anonymousUser.UserID = "anonymous"
	result.anonymousUser.IsAnonymous = true
	result.WelcomeMessage = "Welcom to DxGoFTP"
	result.OnClientConnect = func(con *ServerBase.DxNetConnection){
		//客户链接，发送回消息
		con.WriteObject(&ftpResponsePkg{220,result.WelcomeMessage,false})
	}
	result.OnRecvData = func(con *ServerBase.DxNetConnection,recvData interface{}) {
		cmdpkg := recvData.(*ftpcmdpkg)
		v := DxCommonLib.FastByte2String(cmdpkg.cmd)
		paramstr := DxCommonLib.FastByte2String(cmdpkg.params)
		fmt.Println(v)
		fmt.Println(paramstr)
		if cmd,ok := ftpCmds[v];!ok{
			con.WriteObject(&ftpResponsePkg{500,"Commands not supported temporarily",false})
		}else {
			if cmd.MustLogin(){
				client := con.GetUseData().(*ftpClientBinds)
				if !client.isLogin{
					con.WriteObject(&ftpResponsePkg{530,"you must login first",false})
					return
				}
			}
			cmd.Execute(result,con,paramstr)
		}
	}
	result.OnSendData = func(con *ServerBase.DxNetConnection,Data interface{},sendlen int,sendok bool){
		if respPkg,ok := Data.(*ftpResponsePkg);ok{
			if respPkg.responseCode==221 && respPkg.responseMsg=="Byebye"{
				con.Close()
			}
		}
	}
	return result
}