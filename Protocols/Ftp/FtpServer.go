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
	"time"
	"os"
	"encoding/binary"
	"errors"
	"path/filepath"
	mrand "math/rand"
	"sync/atomic"
	"runtime"
	"sync"
)

type(
	FtpProtocol  struct{}

	dataEncoder struct{
		datasrv   *ftpDataServer
	}

	ftpcmdpkg struct{
		cmd			[]byte
		params		[]byte
	}

	ftpfileinfo 	struct{
		f				*os.File
		totalSize		uint32
		curPosition		uint32
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
		keyName				string
		ftpdirName			string
		localPathName		string
	}

	ftpClientBinds		struct{
		user			*ftpUser
		isLogin			bool
		curPath			string			//当前的路径位置
		typeAnsi		bool			//ANSIMode
		lastFilePos		uint32
		wg				sync.WaitGroup
		datacon			*ServerBase.DxNetConnection //数据流的链接
		cmdcon			*ServerBase.DxNetConnection //命令的链接
	}

	FTPServer struct{
		ServerBase.DxTcpServer
		users				sync.Map
		ftpDirectorys		sync.Map
		ftplocalPaths		sync.Map
		maindir				ftpDirs
		WelcomeMessage		string
		anonymousUser		ftpUser
		PublicIP			string   		//对外开放的IP服务地址
		dataServer			*ftpDataServer  //对外的被动二进制服务
		MinPasvPort			uint16			//被动数据请求的开放端口范围
		MaxPasvPort			uint16
		clientPool			sync.Pool
	}


	//被动模式下建立的数据服务器
	ftpDataServer  struct{
		ServerBase.DxTcpServer
		ownerFtp			*FTPServer
		port				uint16
		curCon				atomic.Value
		lastDataTime		atomic.Value
		clientConChan		chan *ServerBase.DxNetConnection
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
	cmdCWD		struct{cmdBase}
	cmdLIST		struct{cmdBase}
	cmdCDUP		struct{cmdBase}
	cmdRETR		struct{cmdBase}
	cmdSIZE		struct{cmdBase}
	cmdMDTM		struct{cmdBase}
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
		"CWD":			cmdCWD{},
		"PASV":			cmdPASV{},
		"LIST":			cmdLIST{},
		"CDUP":			cmdCDUP{},
		"XCUP":			cmdCDUP{},
		"RETR":			cmdRETR{},
		"SIZE":			cmdSIZE{},
		"MDTM":			cmdMDTM{},
		}
	featCmds			string
	dirNoExistError = errors.New("Directors not Exists")
	noDirError = errors.New("Not a directory")
)

func init() {
	for k, v := range ftpCmds {
		if v.IsFeatCmd() {
			featCmds = featCmds + " " + k + "\n"
		}
	}
}

func (ecoder dataEncoder)Encode(obj interface{},w io.Writer) error { //编码对象
	return nil
}


func (ecoder dataEncoder)Decode(bytes []byte)(result interface{},ok bool){ //解码数据到对应的对象
	return nil,false
}

func (ecoder dataEncoder)HeadBufferLen()uint16{  //编码器的包头大小
	return 0
}
func (ecoder dataEncoder)MaxBufferLen()uint16{ //允许的最大缓存
	return 44*1460
}

func (ecoder dataEncoder)UseLitterEndian()bool{ //是否采用小结尾端
	return false
}

func (ecoder dataEncoder)ProtoName()string{
	return "FTPDataTrans"
}

func (ecoder dataEncoder)ParserProtocol(r *ServerBase.DxReader)(parserOk bool,datapkg interface{},e error){		//解析协议，如果解析成功，返回true，根据情况可以设定返回协议数据包
	//一般是客户端发送过来的文件字节流信息
	count := r.Buffered()
	if count > 0{
		buffer := ecoder.datasrv.GetBuffer()
		r.WriteTo(buffer,count)
		datapkg = buffer
		parserOk = true
	}else{
		datapkg = nil
		parserOk = false
	}
	return parserOk,datapkg,nil
}

func (ecoder dataEncoder)PacketObject(objpkg interface{},buffer *bytes.Buffer)([]byte,error){  //将发送的内容打包到w中
	//发送给客户端的文件流
	switch v := objpkg.(type){
	case string:
		return ([]byte)(v),nil
	case []byte:
		return v,nil
	case *bytes.Buffer:
		return v.Bytes(),nil
	default:
		binary.Write(buffer,binary.LittleEndian,objpkg)
		return buffer.Bytes(),nil
	}
}


func (cmd cmdBase)IsFeatCmd() bool{
	return false
}

func (cmd cmdBase)MustLogin() bool{
	return true
}

func lpad(input string, length int) (result string) {
	if len(input) < length {
		result = strings.Repeat(" ", length-len(input)) + input
	} else if len(input) == length {
		result = input
	} else {
		result = input[0:length]
	}
	return
}

func (cmd cmdMDTM)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//检查文件最后修改时间
	client := con.GetUseData().(*ftpClientBinds)
	path := srv.buildPath(client.curPath, paramstr)
	path,err := srv.localPath(path)
	rPath, err := filepath.Abs(path)
	if err != nil {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "Get ModifyTime Error: ",err.Error()),false})
		return
	}
	if f, err := os.Lstat(rPath);err!=nil{
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "Get ModifyTime : ",err.Error()),false})
	}else{
		con.WriteObject(&ftpResponsePkg{213,f.ModTime().Format("20060102150405"),false})
	}
}

func (cmd cmdSIZE)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//获得文件大小
	client := con.GetUseData().(*ftpClientBinds)
	path := srv.buildPath(client.curPath, paramstr)
	path,err := srv.localPath(path)
	rPath, err := filepath.Abs(path)
	if err != nil {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "Size Error: ",err.Error()),false})
		return
	}
	if f, err := os.Lstat(rPath);err!=nil{
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "Size Error: ",err.Error()),false})
	}else{
		con.WriteObject(&ftpResponsePkg{213,strconv.Itoa(int(f.Size())),false})
	}

}

func (cmd cmdRETR)petrFile(params ...interface{})  {
	totalsize := params[3].(int64)
	f := params[1].(*os.File)
	client := params[2].(*ftpClientBinds)
	srv := params[0].(*FTPServer)
	/*rsize,err := io.Copy(client.datacon,f)
	if err == nil{
		client.cmdcon.WriteObject(&ftpResponsePkg{226,fmt.Sprintf("Closing data connection,TotalSize %d sent %d bytes",totalsize,rsize),false})
	}*/
	rsize := 0
	buffer := srv.dataServer.GetBuffer()
	maxsize := int64(srv.dataServer.GetCoder().MaxBufferLen())
	lreader := io.LimitedReader{f,maxsize}
	for{
		rl,err := io.Copy(buffer,&lreader)
		lreader.N = maxsize
		client.cmdcon.LastValidTime.Store(time.Now()) //更新一下命令处理连接的操作数据时间
		if rl != 0{
			rsize += int(rl)
			buffer.WriteTo(client.datacon)
			if err != nil{
				break
			}
		}else{
			break
		}
	}
	f.Close()
	client.cmdcon.WriteObject(&ftpResponsePkg{226,fmt.Sprintf("Closing data connection,TotalSize %d sent %d bytes",totalsize,rsize),false})
	client.datacon.Close() //数据连接关闭
	client.datacon = nil
	srv.dataServer.ReciveBuffer(buffer)
}

func (cmd cmdRETR)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//下载文件或者目录
	client := con.GetUseData().(*ftpClientBinds)
	path := srv.buildPath(client.curPath, paramstr)
	path,err := srv.localPath(path)
	rPath, err := filepath.Abs(path)
	if err != nil {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "RETR Error: ",err.Error()),false})
		return
	}
	f, err := os.Open(rPath)
	if err != nil{
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "RETR Error: ",err.Error()),false})
		return
	}
	info, err := f.Stat()
	if err != nil{
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "RETR Error: ",err.Error()),false})
		return
	}
	f.Seek(int64(client.lastFilePos), os.SEEK_SET)
	totalsize := info.Size() - int64(client.lastFilePos)
	con.WriteObject(&ftpResponsePkg{150,fmt.Sprintf("Data transfer starting %v bytes", totalsize),false})
	DxCommonLib.PostFunc(cmd.petrFile,srv,f,client,totalsize)
}

func (cmd cmdCDUP)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//回到上级目录
	/*var ocmd cmdCWD
	ocmd.Execute(srv,con,"..")*/
	client := con.GetUseData().(*ftpClientBinds)
	if len(client.curPath) > 0 && client.curPath != "/"{
		bt := ([]byte)(client.curPath)
		idx := bytes.LastIndexByte(bt,'/')
		if idx != -1{
			client.curPath = string(bt[:idx])
			srv.ChangeDir(client.curPath)
		}
	}
	con.WriteObject(&ftpResponsePkg{250,"Directory changed to "+client.curPath,false})
}

func (cmd cmdLIST)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	con.WriteObject(&ftpResponsePkg{150,"Opening ASCII mode data connection for file list",false})
	var fpath string
	client := con.GetUseData().(*ftpClientBinds)
	if len(paramstr) == 0 {
		fpath = paramstr
	} else {
		fields := strings.Fields(paramstr)
		for _, field := range fields {
			if strings.HasPrefix(field, "-") {
				//TODO: currently ignore all the flag
				//fpath = conn.namePrefix
			} else {
				fpath = field
			}
		}
	}
	path := srv.buildPath(client.curPath, fpath)
	path,err := srv.localPath(path)
	if err != nil{
		con.WriteObject(&ftpResponsePkg{550,err.Error(),false})
		return
	}
	if path == ""{
		//根目录，直接返回根目录结构
		buffer := srv.GetBuffer()
		var finfo os.FileInfo
		srv.ftpDirectorys.Range(func(key, value interface{}) bool {
			dirs := value.(*ftpDirs)
			finfo,err = os.Stat(dirs.localPathName)
			if err != nil{
				return false
			}
			buffer.WriteString(finfo.Mode().String())
			buffer.WriteString(" 1 System System ")
			buffer.WriteString(lpad(strconv.Itoa(int(finfo.Size())), 12))
			buffer.WriteString(finfo.ModTime().Format(" Jan _2 15:04 "))
			fmt.Fprintf(buffer, "%s\r\n", finfo.Name())
			return true
		})
		//主目录
		//获取主目录下的所有目录结构和文档信息
		filepath.Walk(srv.maindir.localPathName, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rPath, _ := filepath.Rel(srv.maindir.localPathName, path)
			if rPath == info.Name(){
				buffer.WriteString(info.Mode().String())
				buffer.WriteString(" 1 System System ")
				buffer.WriteString(lpad(strconv.Itoa(int(info.Size())), 12))
				buffer.WriteString(info.ModTime().Format(" Jan _2 15:04 "))
				fmt.Fprintf(buffer, "%s\r\n", info.Name())
			}
			return nil
		})

		buffer.WriteString("\r\n")
		client.wg.Wait()
		con.WriteObject(&ftpResponsePkg{226,"Closing data connection, sent " + strconv.Itoa(buffer.Len()) + " bytes",false})
		buffer.WriteTo(client.datacon)
		client.datacon.Close()
		client.datacon = nil
		srv.ReciveBuffer(buffer)
		return
	}
	path, err = filepath.Abs(path)
	if err != nil {
		con.WriteObject(&ftpResponsePkg{550,err.Error(),false})
		return
	}
	f, err := os.Lstat(path)
	if err != nil {
		con.WriteObject(&ftpResponsePkg{550,err.Error(),false})
		return
	}
	if !f.IsDir(){
		con.WriteObject(&ftpResponsePkg{550,"Not a directory",false})
		return
	}
	//找到了真实的目录位置
	buffer := srv.GetBuffer()
	filepath.Walk(path, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rPath, _ := filepath.Rel(path, fpath)
		if rPath == info.Name(){
			buffer.WriteString(info.Mode().String())
			buffer.WriteString(" 1 System System ")
			buffer.WriteString(lpad(strconv.Itoa(int(info.Size())), 12))
			buffer.WriteString(info.ModTime().Format(" Jan _2 15:04 "))
			fmt.Fprintf(buffer, "%s\r\n", info.Name())
		}
		return nil
	})
	buffer.WriteString("\r\n")
	client.wg.Wait()
	con.WriteObject(&ftpResponsePkg{226,"Closing data connection, sent " + strconv.Itoa(buffer.Len()) + " bytes",false})
	buffer.WriteTo(client.datacon)
	client.datacon.Close()
	client.datacon = nil
	srv.ReciveBuffer(buffer)
}

func (cmd cmdPASV)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//被动传输模式
	port := uint16(0)
	if  srv.dataServer != nil{
		if !srv.dataServer.Active(){
			port = srv.MinPasvPort + uint16(mrand.Intn(int(srv.MaxPasvPort-srv.MinPasvPort)))
			if !srv.createDataServer(port){
				con.WriteObject(&ftpResponsePkg{425,"Data connection failed",false})
				return
			}
		}else{
			port = srv.dataServer.port
		}
	}else{
		port = srv.MinPasvPort + uint16(mrand.Intn(int(srv.MaxPasvPort-srv.MinPasvPort)))
		if !srv.createDataServer(port){
			con.WriteObject(&ftpResponsePkg{425,"Data connection failed",false})
			return
		}
	}
	listenIP := srv.PublicIP
	if len(listenIP)==0{
		listenIP = con.Address()
	}
	p1 := port / 256
	p2 := port - (p1 * 256)
	quads := strings.Split(listenIP, ".")
	//准备等待，客户端建立链接上来
	srv.dataServer.curCon.Store(con)
	con.WriteObject(&ftpResponsePkg{227,fmt.Sprintf("Entering Passive Mode (%s,%s,%s,%s,%d,%d)", quads[0], quads[1], quads[2], quads[3], p1, p2),false})
	//然后执行等待链接
	client := con.GetUseData().(*ftpClientBinds)
	client.wg.Add(1)
	DxCommonLib.PostFunc(func(data ...interface{}) {
		datasrv := data[0].(*ftpDataServer)
		ftpclient := data[1].(*ftpClientBinds)
		select{
		case clientcon := <-srv.dataServer.clientConChan:
			clientcon.SetUseData(ftpclient) //绑定到用户上
			ftpclient.datacon = clientcon
		case <-DxCommonLib.After(time.Second*60):
			var m *ServerBase.DxNetConnection=nil
			datasrv.curCon.Store(m)
		}
		ftpclient.wg.Done()
	},srv.dataServer,client)
}


func (cmd cmdCWD)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
   //变更目录
	fullPath := ""
	client := con.GetUseData().(*ftpClientBinds)
	if len(paramstr) > 0 {
		if paramstr[0:1] == "/"{
			fullPath = filepath.Clean(paramstr)
		}else if paramstr != "-a"{
			fullPath = filepath.Clean(client.curPath + "/" + paramstr)
		}else{
			fullPath = filepath.Clean(client.curPath)
		}
		fullPath = strings.Replace(fullPath, "//", "/", -1)
		fullPath = strings.Replace(fullPath, string(filepath.Separator), "/", -1)
	}
	var err error = nil
	if fullPath != ""{
		//准备变更
		err = srv.ChangeDir(fullPath)
		if err == nil{
			client.curPath = fullPath
		}
	}
	if err == nil{
		con.WriteObject(&ftpResponsePkg{250,"Directory changed to "+fullPath,false})
	}else{
		con.WriteObject(&ftpResponsePkg{550,fmt.Sprintln("Directory change to", fullPath, "failed:", err),false})
	}
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
		con.WriteObject(&ftpResponsePkg{230,"user login success!",false})
	}else{
		if client.user.IsAnonymous && client.user.PassWord == ""{
			client.isLogin = true
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
		ftpbinds := srv.getFtpClient()
		ftpbinds.isLogin = false
		ftpbinds.cmdcon = con
		ftpbinds.typeAnsi = true
		ftpbinds.curPath = ""
		ftpbinds.user = &srv.anonymousUser
		con.SetUseData(ftpbinds)
		con.WriteObject(&ftpResponsePkg{331,"Please enter a password",false})
		return
	}
	if v,ok := srv.users.Load(paramstr);ok{
		ftpbinds := srv.getFtpClient()
		ftpbinds.typeAnsi = true
		ftpbinds.isLogin = false
		ftpbinds.cmdcon = con
		ftpbinds.curPath = ""
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
	return 512
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
	result.MinPasvPort = 50000
	result.MaxPasvPort = 60000
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
	result.OnClientDisConnected = func(con *ServerBase.DxNetConnection) {
		v := con.GetUseData()
		if v != nil{
			if client,ok := v.(*ftpClientBinds);ok{
				result.freeFtpClient(client)
			}
		}
	}
	result.OnSrvClose = func() {
		if result.dataServer != nil{
			result.dataServer.Close()
		}
	}
	return result
}

func (srv *ftpDataServer)dataRecvData(con *ServerBase.DxNetConnection,recvData interface{})  {
	buffer := recvData.(*bytes.Buffer)
	//写入文件
	srv.ReciveBuffer(buffer) //回收
}

func (srv *ftpDataServer)ClientConnect(con *ServerBase.DxNetConnection){
	v := srv.curCon.Load()
	if v!=nil{
		m := v.(*ServerBase.DxNetConnection)
		if m != nil{
			st1 := strings.SplitN(m.RemoteAddr(),":",2)
			st2 := strings.SplitN(con.RemoteAddr(),":",2)
			if st1[0] == st2[0]{
				srv.clientConChan <- con
			}
		}

	}
}

func (srv *ftpDataServer)sendFileData()  {

}

func (srv *FTPServer)checkDataServerIdle(params ...interface{})  {
	srvCloseChan := srv.Done()
	dataclosechan := srv.dataServer.Done()
	for{
		select{
		case <-srvCloseChan:
			return
		case <-time.After(time.Minute):
			vtime := srv.dataServer.lastDataTime.Load().(time.Time)
			if time.Now().Sub(vtime).Minutes() > 10 { //超过10分钟没有信息上来，关闭数据服务
				srv.dataServer.Close()
				return
			}
		case <-dataclosechan:
			return
		}
	}
}


func (srv *FTPServer)createDataServer(port uint16)bool  {
	if srv.dataServer == nil{
		srv.dataServer = new(ftpDataServer)
		srv.dataServer.TimeOutSeconds = 90
		srv.dataServer.ownerFtp = srv
		srv.dataServer.SrvLogger = srv.SrvLogger
		srv.dataServer.SetCoder(dataEncoder{})
		srv.dataServer.clientConChan = make(chan*ServerBase.DxNetConnection)
		srv.dataServer.OnRecvData = srv.dataServer.dataRecvData //上传文件的时候
		srv.dataServer.OnClientConnect = srv.dataServer.ClientConnect
	}
	srv.dataServer.lastDataTime.Store(time.Time{})
	srv.dataServer.port = port
	if !srv.dataServer.Active(){
		if err := srv.dataServer.Open(fmt.Sprintf(":%d",port));err!=nil{
			return false
		}
		//开启一个监听的，然后监控服务的空闲时间
		DxCommonLib.PostFunc(srv.checkDataServerIdle)
	}
	return true
}

func (srv *FTPServer)MapDir(remotedir,localPath string,isMainRoot bool)  {
	ldir := strings.ToUpper(localPath)
	rdir := strings.ToUpper(remotedir)
	var dir *ftpDirs
	if v,ok := srv.ftplocalPaths.Load(ldir);ok{
		dir := v.(*ftpDirs)
		if dir.keyName == rdir{
			return
		}
		srv.ftpDirectorys.Delete(dir.keyName)
	}else{
		dir = new(ftpDirs)
	}
	if isMainRoot{
		srv.maindir.ftpdirName = remotedir
		srv.maindir.localPathName = localPath
		return
	}
	dir.keyName = rdir
	dir.ftpdirName = remotedir
	dir.localPathName = localPath
	srv.ftpDirectorys.Store(rdir,dir)
}

func (srv *FTPServer)buildPath(curPath,paramstr string)string  {
	fullPath := curPath
	if len(paramstr) > 0 {
		if paramstr[0:1] == "/"{
			fullPath = filepath.Clean(paramstr)
		}else if paramstr != "-a"{
			fullPath = filepath.Clean(curPath + "/" + paramstr)
		}else{
			fullPath = filepath.Clean(curPath)
		}
		fullPath = strings.Replace(fullPath, "//", "/", -1)
		fullPath = strings.Replace(fullPath, string(filepath.Separator), "/", -1)
	}
	return fullPath
}

func (srv *FTPServer)localPath(ftpdir string)(string,error)  {
	paths := strings.Split(ftpdir, "/")
	rdir := strings.ToUpper(paths[0]) //第一个目录是用户指定的目录
	isinRoot := rdir=="" //包含了root
	if isinRoot  && len(paths)>1{
		rdir = paths[1]
		if len(rdir)>0{
			rdir = strings.ToUpper(rdir)
			if len(paths)>2{
				paths = paths[2:]
			}else{
				paths = paths[1:1]
			}
		}
	}
	if rdir == ""{
		//根目录,回到根目录
		return "",nil
	}
	if v,ok := srv.ftpDirectorys.Load(rdir);ok{
		dirs := v.(*ftpDirs)
		return filepath.Join(append([]string{dirs.localPathName}, paths...)...),nil
	}
	//不是根目录中的目录，那么认为是主目录中的
	return filepath.Join(append([]string{srv.maindir.localPathName,rdir}, paths...)...),nil
}

func (srv *FTPServer)getFtpClient()*ftpClientBinds  {
	v := srv.clientPool.Get()
	if v == nil{
		return  new(ftpClientBinds)
	}
	return v.(*ftpClientBinds)
}

func (srv *FTPServer)freeFtpClient(client *ftpClientBinds)  {
	client.cmdcon = nil
	client.datacon = nil
	client.curPath = ""
	client.user = nil
	client.lastFilePos = 0
	client.isLogin = false
	client.typeAnsi = true
	srv.clientPool.Put(client)
}

func (srv *FTPServer)ChangeDir(fullpath string)error  {
	paths := strings.Split(fullpath, "/")
	rdir := strings.ToUpper(paths[0]) //第一个目录是用户指定的目录
	if rdir == "" && len(paths)>1{
		rdir = strings.ToUpper(paths[1])
		if len(paths)>2{
			paths = paths[2:]
		}else{
			paths = paths[1:1]
		}
	}
	if rdir == ""{
		//根目录,回到根目录
		return  nil
	}
	if v,ok := srv.ftpDirectorys.Load(rdir);ok{
		dirs := v.(*ftpDirs)
		rdir = filepath.Join(append([]string{dirs.localPathName}, paths...)...)
		f, err := os.Lstat(rdir)
		if err != nil {
			return err
		}
		if f.IsDir() {
			return nil
		}else{
			return noDirError
		}
	}
	return dirNoExistError
}