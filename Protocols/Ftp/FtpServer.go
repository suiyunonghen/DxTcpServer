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
	}

	FTPServer struct{
		ServerBase.DxTcpServer
		users				sync.Map
		ftpDirectorys		sync.Map
		WelcomeMessage		string
		anonymousUser		ftpUser
	}
)


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
	result.anonymousUser.PassWord = "anonymous"//账户密码
	result.anonymousUser.IsAnonymous = true
	result.WelcomeMessage = "欢迎使用DxGoFTP"
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
		switch v {
		case "ADAT":
		case "ALLO":
		case "APPE":
		case "AUTH":
		case "CDUP":
		case "CWD":
		case "CCC":
		case "CONF":
		case "DELE":
		case "ENC":
		case "EPRT":
		case "EPSV":
		case "FEAT":
		case "LIST":
		case "NLST":
		case "MDTM":
		case "MIC":
		case "MKD":
		case "MODE":
		case "NOOP":
		case "OPTS":
		case "PASS":
			//用户密码
			//conn.writeMessage(331, "User name ok, password required")
			client := con.GetUseData().(*ftpClientBinds)
			if client.user.PassWord == paramstr{
				client.isLogin = true
				con.WriteObject(&ftpResponsePkg{230,"用户登录成功！",false})
			}else{
				if client.user.IsAnonymous && client.user.PassWord == ""{
					con.WriteObject(&ftpResponsePkg{230,"用户登录成功！",false})
				}else{
					con.WriteObject(&ftpResponsePkg{530,"密码错误，用户登录失败",false})
				}
			}
		case "PASV":
		case "PBSZ":
		case "PORT":
		case "PROT":
		case "PWD":
		case "QUIT":
		case "RETR":
		case "REST":
		case "RNFR":
		case "RNTO":
		case "RMD":
		case "SIZE":
		case "STOR":
		case "STRU":
		case "SYST":
			//服务器远程系统的操作系统类型
			if strings.Compare(runtime.GOOS,"windows") == 0{
				con.WriteObject(&ftpResponsePkg{215,"Windows Type: L8",false})
			}else{
				con.WriteObject(&ftpResponsePkg{215,"UNIX Type: L8",false})
			}
		case "TYPE":
		case "USER":
			//用户登录，将记录用户的信息,绑定到用户信息上
			if strings.Compare(paramstr,result.anonymousUser.UserID)==0{
				ftpbinds := new(ftpClientBinds)
				ftpbinds.isLogin = false
				ftpbinds.user = &result.anonymousUser
				con.SetUseData(ftpbinds)
				con.WriteObject(&ftpResponsePkg{331,"用户名确认，请输入用户密码",false})
				return
			}
			if v,ok := result.users.Load(paramstr);ok{
				ftpbinds := new(ftpClientBinds)
				ftpbinds.isLogin = false
				ftpbinds.user = v.(*ftpUser)
				con.SetUseData(ftpbinds)
				con.WriteObject(&ftpResponsePkg{331,"用户名确认，请输入用户密码",false})
				return
			}
			con.WriteObject(&ftpResponsePkg{530,"用户不存在，无法登录",false})
		case "XCUP":
		case "XCWD":
		case "XPWD":
		case "XRMD":
		default:
			con.WriteObject(&ftpResponsePkg{500,"暂时不支持的命令",false})
		}
	}
	return result
}