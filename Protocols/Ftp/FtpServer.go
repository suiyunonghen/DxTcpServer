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
	"errors"
	"path/filepath"
	"math/rand"
	"sync/atomic"
	"runtime"
	"sync"
)

type(

	FTPPermission		uint32
	FtpProtocol			struct{}

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

	FtpUser		struct{
		UserID			string
		PassWord		string
		Permission 		FTPPermission
		IsAnonymous		bool
	}

	ftpDirs		struct{
		keyName				string
		ftpdirName			string
		localPathName		string
	}

	ftpClientBinds		struct{
		user			*FtpUser
		isLogin			bool
		curPath			string			//当前的路径位置
		typeAnsi		bool			//ANSIMode
		datacon			*ServerBase.DxNetConnection //数据流的链接
		cmdcon			*ServerBase.DxNetConnection //命令的链接
		lastPos			int64
		reNameFrom		string				//要重命名的文件或者目录
		clientUtf8		bool
	}

	dataClientBinds		struct{
		ftpClient		*ftpClientBinds
		f				*os.File
		curPosition		uint32
		lastFilePos		uint32
		waitReadChan	chan struct{}
	}

	FTPServer struct{
		ServerBase.DxTcpServer
		users				sync.Map
		ftpDirectorys		sync.Map
		ftplocalPaths		sync.Map
		maindir				ftpDirs
		WelcomeMessage		string
		anonymousUser		FtpUser
		PublicIP			string   		//对外开放的IP服务地址
		dataServer			*ftpDataServer  //对外的被动二进制服务
		MinPasvPort			uint16			//被动数据请求的开放端口范围
		MaxPasvPort			uint16
		clientPool			sync.Pool
		OnGetFtpUser		func(userId string)*FtpUser  //当在用户列表中没找到的时候，触发本函数查找用户
	}


	//被动模式下建立的数据服务器
	ftpDataServer  struct{
		ServerBase.DxTcpServer
		ownerFtp			*FTPServer
		port				uint16
		curCon				atomic.Value
		lastDataTime		atomic.Value
		waitBindChan		chan struct{}
		notifyBindOk		chan struct{}
		clientPool			sync.Pool
	}

	ftpcmd		interface{
		Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)
		IsFeatCmd()bool
		MustLogin()bool
		MustUtf8()bool
		HasParams()bool
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
	cmdSTOR		struct{cmdBase}
	cmdDELE		struct{cmdBase}
	cmdALLO		struct{cmdBase}
	cmdAPPE		struct{cmdBase}
	cmdNLST		struct{cmdBase}
	cmdNOOP		struct{cmdBase}
	cmdMKD		struct{cmdBase}
	cmdMODE		struct{cmdBase}
	cmdOPTS		struct{cmdBase}
	cmdREST		struct{cmdBase}
	cmdRNFR		struct{cmdBase}
	cmdRNTO		struct{cmdBase}
	cmdRMD		struct{cmdBase}
	cmdSTRU		struct{cmdBase}
)

const(
	Permission_File_Read = 1
	Permission_File_Write = 2
	Permission_File_Delete = 4
	Permission_File_Append = 8

	Permission_Dir_Create = 0x10
	Permission_Dir_Delete = 0x20
	Permission_Dir_List = 0x40
	Permission_Dir_SubDirs = 0x80
)

var (
	ftpCmds	= map[string]ftpcmd{
		"ALLO":			cmdALLO{},
		"APPE":			cmdAPPE{},
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
		"RETR":			cmdRETR{},
		"SIZE":			cmdSIZE{},
		"MDTM":			cmdMDTM{},
		"MKD":			cmdMKD{},
		"NOOP":			cmdNOOP{},
		"NLST":			cmdNLST{},
		"STOR":			cmdSTOR{},
		"DELE":			cmdDELE{},
		"MODE":			cmdMODE{},
		"OPTS":			cmdOPTS{},
		"REST":			cmdREST{},
		"RNFR":			cmdRNFR{},
		"RNTO":			cmdRNTO{},
		"RMD":			cmdRMD{},
		"STRU":			cmdSTRU{},
		"XCUP":			cmdCDUP{},
		"XRMD":			cmdRMD{},
		"XCWD":			cmdCWD{},
		"XPWD":			cmdPWD{},
		}
	dirNoExistError = errors.New("Directors not Exists")
	noDirError = errors.New("Not a directory")
	unlogPkg = ftpResponsePkg{530,"you must login first",false}
	unknowCmdPkg = ftpResponsePkg{500,"Commands not supported temporarily",false}
	noParamPkg = ftpResponsePkg{501,"action aborted, required param missing",false}
	obsoletePkg = ftpResponsePkg{202,"Obsolete",false}
	okpkg = ftpResponsePkg{200,"OK",false}
	dataTransferStartPkg =ftpResponsePkg{150,"Data transfer starting",false}
	openAsCiiModePkg = ftpResponsePkg{150,"Opening ASCII mode data connection for file list",false}
	noDirPkg = ftpResponsePkg{550,"Not a directory",false}
	dataConnectionFailedPkg = ftpResponsePkg{425,"Data connection failed",false}
	tpSetApkg = ftpResponsePkg{200,"Type set to ASCII",false}
	tbSetBpkg = ftpResponsePkg{200,"Type set to binary",false}
	featcmdPkg = ftpResponsePkg{211,"",true}
	logokpkg = ftpResponsePkg{230,"user login success!",false}
	logfailedpkg = ftpResponsePkg{530,"Password error, user logon failed",false}
	winTypePkg = ftpResponsePkg{215,"Windows Type: L8",false}
	unixTypePkg = ftpResponsePkg{215,"UNIX Type: L8",false}
	passWordPkg = ftpResponsePkg{331,"Please enter a password",false}
	canNotLogPkg = ftpResponsePkg{530,"User does not exist and cannot log on",false}
	byePkg = ftpResponsePkg{221,"Byebye",false}
	pathNoFile = ftpResponsePkg{550,"Path Is not a File",false}
	fileDelPkg = ftpResponsePkg{250,"File deleted",false}
	storeFileFpkg = ftpResponsePkg{450,"can not Store File: has a Same Name Directory",false}
	dirCreateedPkg = ftpResponsePkg{257,"Directory created OK",false}
	modeobsoletePkg= ftpResponsePkg{504,"MODE is an obsolete command",false}
	utf8OnOffPkg = ftpResponsePkg{550,"Params Must 'UTF8 ON(OFF)'",false}
	utf8OnPkg = ftpResponsePkg{200,"UTF8 mode enabled",false}
	utf8ffPkg = ftpResponsePkg{550,"UTF8 mode disabled",false}
	invalidateFilePosPkg = ftpResponsePkg{550,"invalidate File Position",false}
	rnfrCmdPkg = ftpResponsePkg{350,"Please Send RNTO Command To Set New Name.",false}
	frenameOkpkg = ftpResponsePkg{250,"File renamed",false}
	dirDeleteOkPkg = ftpResponsePkg{250,"Directory deleted OK",false}

	noDelDirPermission = ftpResponsePkg{551,"Directory delete failed: No Del Dir Permission",false}
	noCreateDirPermission = ftpResponsePkg{551,"Create Directory failed: No MkDir Permission",false}
	noWriteFilePermission = ftpResponsePkg{551,"Upload File failed: No Write Permission",false}
	noDelFilePermission = ftpResponsePkg{551,"Del File failed: No Write Permission",false}
	noAppendFilePermission = ftpResponsePkg{551,"Append File failed: No Append Permission",false}
	noReadFilePermission = ftpResponsePkg{551,"DownLoad File failed: No Read Permission",false}
	noListPermission = ftpResponsePkg{551,"No List Permission",false}
)


func init() {
	featCmds := ""
	for k, v := range ftpCmds {
		if v.IsFeatCmd() {
			featCmds = featCmds + " " + k + "\n"
		}
	}
	featcmdPkg.responseMsg = featCmds
}

func (permission FTPPermission)CanReadFile()bool  {
	return uint32(permission) & Permission_File_Read == Permission_File_Read
}

func (permission FTPPermission)CanWriteFile()bool  {
	return uint32(permission) & Permission_File_Write == Permission_File_Write
}

func (permission FTPPermission)CanDelFile()bool  {
	return uint32(permission) & Permission_File_Delete == Permission_File_Delete
}


func (permission FTPPermission)CanAppendFile()bool  {
	return uint32(permission) & Permission_File_Append == Permission_File_Append
}


func (permission FTPPermission)CanCreateDir()bool  {
	return uint32(permission) & Permission_Dir_Create == Permission_Dir_Create
}

func (permission FTPPermission)CanDelDir()bool  {
	return uint32(permission) & Permission_Dir_Delete == Permission_Dir_Delete
}

func (permission FTPPermission)CanListDir()bool  {
	return uint32(permission) & Permission_Dir_List == Permission_Dir_List
}


func (permission FTPPermission)CanSubDirs()bool  {
	return uint32(permission) & Permission_Dir_SubDirs == Permission_Dir_SubDirs
}

func (permission *FTPPermission)SetFileReadPermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_File_Read)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_File_Read)))
	}
}

func (permission *FTPPermission)SetFileWritePermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_File_Write)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_File_Write)))
	}
}

func (permission *FTPPermission)SetFileDelPermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_File_Delete)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_File_Delete)))
	}
}


func (permission *FTPPermission)SetFileAppendPermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_File_Append)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_File_Append)))
	}
}


func (permission *FTPPermission)SetDirCreatePermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_Dir_Create)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_Dir_Create)))
	}
}

func (permission *FTPPermission)SetDirListPermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_Dir_List)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_Dir_List)))
	}
}

func (permission *FTPPermission)SetDirDelPermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_Dir_Delete)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_Dir_Delete)))
	}
}


func (permission *FTPPermission)SetDirSubDirsPermission(v bool)  {
	if v {
		*permission = FTPPermission(uint32(*permission) | Permission_Dir_SubDirs)
	}else{
		*permission = FTPPermission(uint32(*permission) & uint32(^uint8(Permission_Dir_SubDirs)))
	}
}

func (cmd cmdBase)IsFeatCmd() bool{
	return false
}

func (cmd cmdBase)MustLogin() bool{
	return true
}

func (cmd cmdBase)MustUtf8()bool  {
	return false
}

func (cmd cmdBase)HasParams()bool  {
	return true
}

func (cmd cmdSTRU)HasParams()bool  {
	return false
}

func (cmd cmdSTRU)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	if strings.ToUpper(paramstr) == "F" {
		con.WriteObject(&okpkg)
	}else{
		con.WriteObject(&ftpResponsePkg{504, "STRU is an obsolete command",false})
	}
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

func (cmd cmdALLO)MustLogin() bool{
	return false
}

func (cmd cmdALLO)HasParams()bool  {
	return false
}

func (cmd cmdALLO)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	con.WriteObject(&obsoletePkg)
}

func (cmd cmdNOOP)MustLogin() bool{
	return false
}

func (cmd cmdNOOP)HasParams()bool  {
	return false
}

func (cmd cmdNOOP) Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//保证心跳的
	con.WriteObject(&okpkg)
}

func (cmd cmdRMD)MustUtf8()bool  {
	return true
}

func (cmd cmdRMD) Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string) {
	client := con.GetUseData().(*ftpClientBinds)
	if !client.user.Permission.CanDelDir(){
		con.WriteObject(&noDelDirPermission)
		return
	}
	rpath := srv.localPath(srv.buildPath(client.curPath,paramstr))
	f,err := os.Lstat(rpath)
	if err!=nil{
		con.WriteObject(&ftpResponsePkg{550,fmt.Sprintln("Directory delete failed:", err),false})
	}else if !f.IsDir(){
		con.WriteObject(&noDirPkg)
	}else{
		err = os.Remove(rpath)
		if err != nil{
			con.WriteObject(&ftpResponsePkg{550,fmt.Sprintln("Directory delete failed:", err),false})
		}else{
			con.WriteObject(&dirDeleteOkPkg)
		}
	}
}

func (cmd cmdRNTO)MustUtf8()bool  {
	return true
}


func (cmd cmdRNTO) Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string) {
	client := con.GetUseData().(*ftpClientBinds)
	if !client.user.Permission.CanWriteFile(){
		con.WriteObject(&noWriteFilePermission)
		return
	}
	toPath := srv.localPath(srv.buildPath(client.curPath,paramstr))
	frompath := srv.localPath(client.reNameFrom)
	if err := os.Rename(frompath,toPath);err!=nil{
		con.WriteObject(&ftpResponsePkg{550,fmt.Sprintf("ReName File or Dir Failed: ",err.Error()),false})
	}else{
		con.WriteObject(&frenameOkpkg)
	}
	client.reNameFrom = ""
}

func (cmd cmdRNFR)MustUtf8()bool  {
	return true
}

func (cmd cmdRNFR) Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//重命名，RNFR <old path>,该命令表示重新命名文件，该命令的下一条命令用RNTO指定新的文件名。
	//RNTO <new path>,该命令和RNFR命令共同完成对文件的重命名。
	client := con.GetUseData().(*ftpClientBinds)
	if !client.user.Permission.CanWriteFile(){
		con.WriteObject(&noWriteFilePermission)
		return
	}
	client.reNameFrom = srv.buildPath(client.curPath,paramstr)
	con.WriteObject(&rnfrCmdPkg)
}

func (cmd cmdMODE)HasParams()bool  {
	return false
}

func (cmd cmdMODE) Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string) {
	if strings.ToUpper(paramstr) == "S" {
		con.WriteObject(&okpkg)
	} else {
		con.WriteObject(&modeobsoletePkg)
	}
}

func (cmd cmdREST) Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string) {
	lastFilePos,err := strconv.ParseInt(paramstr, 10, 64)
	if err != nil {
		con.WriteObject(&invalidateFilePosPkg)
		return
	}
	client := con.GetUseData().(*ftpClientBinds)
	client.lastPos = lastFilePos
	con.WriteObject(&ftpResponsePkg{350,fmt.Sprintln("Start transfer from: ", lastFilePos),false})
}

func (cmd cmdMKD)MustUtf8()bool  {
	return true
}

func (cmd cmdMKD)  Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//新建目录
	client := con.GetUseData().(*ftpClientBinds)
	if !client.user.Permission.CanCreateDir(){
		con.WriteObject(&noCreateDirPermission)
		return
	}
	touploadpath:= srv.localPath(srv.buildPath(client.curPath, paramstr)) //要上传到的实际位置
	if err := os.Mkdir(touploadpath,os.ModePerm);err!=nil{
		con.WriteObject(&ftpResponsePkg{550,fmt.Sprintln("make Directory Failed: ", err),false})
	}else{
		con.WriteObject(&dirCreateedPkg)
	}
}

func (cmd cmdOPTS)  Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string){
	//命令的传输模式
	parts := strings.Fields(paramstr)
	if len(parts) != 2 {
		con.WriteObject(&utf8OnOffPkg)
		return
	}
	if strings.ToUpper(parts[0]) != "UTF8" {
		con.WriteObject(&utf8OnOffPkg)
		return
	}
	if strings.ToUpper(parts[1]) == "ON" {
		con.GetUseData().(*ftpClientBinds).clientUtf8 = true
		con.WriteObject(&utf8OnPkg)
	} else {
		con.GetUseData().(*ftpClientBinds).clientUtf8 = false
		con.WriteObject(&utf8ffPkg)
	}
}

func (cmd cmdDELE)MustUtf8()bool  {
	return true
}

func (cmd cmdDELE)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	client := con.GetUseData().(*ftpClientBinds)
	if !client.user.Permission.CanDelFile(){
		con.WriteObject(&noDelFilePermission)
		return
	}
	touploadpath:= srv.localPath(srv.buildPath(client.curPath, paramstr)) //要上传到的实际位置
	//删除文件
	finfo, err := os.Lstat(touploadpath)
	if err == nil{
		if finfo.IsDir(){
			con.WriteObject(&pathNoFile)
			return
		}
		if err = os.Remove(touploadpath);err != nil{
			con.WriteObject(&ftpResponsePkg{550,fmt.Sprintf("File delete failed:",err),false})
		}else{
			con.WriteObject(&fileDelPkg)
		}
	}else{
		con.WriteObject(&ftpResponsePkg{550,fmt.Sprintf("File delete failed:",err),false})
	}
}

func (cmd cmdAPPE)MustUtf8()bool  {
	return true
}

func (cmd cmdAPPE)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//增加文件
	client := con.GetUseData().(*ftpClientBinds)
	if !client.user.Permission.CanAppendFile(){
		con.WriteObject(&noAppendFilePermission)
		return
	}
	if !client.user.Permission.CanWriteFile(){
		con.WriteObject(&noWriteFilePermission)
		return
	}
	touploadpath:= srv.localPath(srv.buildPath(client.curPath, paramstr)) //要上传到的实际位置
	//上传文件开始
	con.WriteObject(&dataTransferStartPkg)
	finfo, err := os.Lstat(touploadpath)
	var f *os.File
	if err == nil{
		if finfo.IsDir(){
			con.WriteObject(&storeFileFpkg)
			return
		}
		//打开文件
		f, err = os.OpenFile(touploadpath, os.O_APPEND|os.O_RDWR, 0660)
		if finfo.Size() >= client.lastPos{
			f.Seek(int64(client.lastPos), os.SEEK_SET)
		}else{
			f.Truncate(int64(client.lastPos))
		}
	}else if !os.IsNotExist(err) {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln("Put File error:", err),false})
		return
	}else{
		f, err = os.Create(touploadpath)
		f.Truncate(int64(client.lastPos))
	}

	if err != nil {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln("Create File error:", err),false})
		return
	}
	//等待用户上传文件直到完成
	dataclient := client.datacon.GetUseData().(*dataClientBinds)
	dataclient.lastFilePos = uint32(client.lastPos)
	dataclient.f = f
	dataclient.curPosition = 0
	close(dataclient.waitReadChan) //通知可以读了
}

func (cmd cmdSTOR)MustUtf8()bool  {
	return true
}

func (cmd cmdSTOR)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//创建或者覆盖文件
	client := con.GetUseData().(*ftpClientBinds)
	dataclient := client.datacon.GetUseData().(*dataClientBinds)
	if dataclient.waitReadChan != nil{
		defer close(dataclient.waitReadChan) //通知可以读了
	}
	if !client.user.Permission.CanWriteFile(){
		con.WriteObject(&noWriteFilePermission)
		return
	}
	touploadpath:= srv.localPath(srv.buildPath(client.curPath, paramstr)) //要上传到的实际位置
	//上传文件开始
	con.WriteObject(&dataTransferStartPkg)
	finfo, err := os.Lstat(touploadpath)
	if err == nil{
		if finfo.IsDir(){
			con.WriteObject(&storeFileFpkg)
			return
		}
	}else if !os.IsNotExist(err) {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln("Put File error:", err),false})
		return
	}
	//创建文件
	f, err := os.Create(touploadpath)
	if err != nil {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln("Create File error:", err),false})
		return
	}
	//等待用户上传文件直到完成
	//client.uploadFileInfo.transEvent =  make(chan struct{})
	dataclient.f = f
	dataclient.curPosition = 0
}

func (cmd cmdMDTM)MustUtf8()bool  {
	return true
}

func (cmd cmdMDTM)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//检查文件最后修改时间
	client := con.GetUseData().(*ftpClientBinds)
	path := srv.buildPath(client.curPath, paramstr)
	path = srv.localPath(path)
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

func (cmd cmdSIZE)MustUtf8()bool  {
	return true
}

func (cmd cmdSIZE)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//获得文件大小
	client := con.GetUseData().(*ftpClientBinds)
	path := srv.buildPath(client.curPath, paramstr)
	path = srv.localPath(path)
	rPath, err := filepath.Abs(path)
	if err != nil {
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "Size Error"),false})
		return
	}
	if f, err := os.Lstat(rPath);err!=nil{
		con.WriteObject(&ftpResponsePkg{450,fmt.Sprintln(paramstr, "Size Error"),false})
	}else{
		con.WriteObject(&ftpResponsePkg{213,strconv.Itoa(int(f.Size())),false})
	}

}

func (cmd cmdRETR)MustUtf8()bool  {
	return true
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
	var bufsize int
	if totalsize < 44 * 1460{
		bufsize = int(totalsize)
	}else{
		bufsize = 44 * 1460
	}
	buffer := srv.dataServer.GetBuffer(bufsize)
	maxsize := int64(bufsize)
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
	client.lastPos = 0
	srv.dataServer.ReciveBuffer(buffer)
}

func (cmd cmdRETR)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//下载文件或者目录
	client := con.GetUseData().(*ftpClientBinds)
	dataclient := client.datacon.GetUseData().(*dataClientBinds)
	if !client.user.Permission.CanReadFile(){
		close(dataclient.waitReadChan) //通知可以读了
		con.WriteObject(&noReadFilePermission)
		return
	}
	path := srv.buildPath(client.curPath, paramstr)
	path = srv.localPath(path)
	rPath, err := filepath.Abs(path)
	dataclient.lastFilePos = uint32(client.lastPos)
	close(dataclient.waitReadChan) //通知可以读了
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
	f.Seek(int64(dataclient.lastFilePos), os.SEEK_SET)
	totalsize := info.Size() - int64(dataclient.lastFilePos)
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

func (cmd cmdNLST)MustUtf8()bool  {
	return true
}


func (cmd cmdNLST)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//返回指定目录的文件名列表
	con.WriteObject(&openAsCiiModePkg)
	client := con.GetUseData().(*ftpClientBinds)
	dataclient := client.datacon.GetUseData().(*dataClientBinds)
	if !client.user.Permission.CanListDir(){
		close(dataclient.waitReadChan) //通知可以读了
		con.WriteObject(&noListPermission)
		return
	}
	var fpath string
	close(dataclient.waitReadChan) //通知可以读了
	fields := strings.Fields(paramstr)
	for _, field := range fields {
		if strings.HasPrefix(field, "-") {
			//TODO: currently ignore all the flag
			//fpath = conn.namePrefix
		} else {
			fpath = field
		}
	}
	path := srv.buildPath(client.curPath, fpath)
	path = srv.localPath(path)
	var err error
	if path == ""{
		//根目录，直接返回根目录结构
		buffer := srv.GetBuffer(4096)
		var finfo os.FileInfo
		srv.ftpDirectorys.Range(func(key, value interface{}) bool {
			dirs := value.(*ftpDirs)
			finfo,err = os.Stat(dirs.localPathName)
			buffer.WriteString(finfo.Mode().String())
			buffer.WriteString(" 1 System System ")
			buffer.WriteString(lpad(strconv.Itoa(int(finfo.Size())), 12))
			buffer.WriteString(finfo.ModTime().Format(" Jan _2 15:04 "))
			if client.clientUtf8{
				buffer.WriteString(finfo.Name())
			}else{
				gbkbyte,_:= DxCommonLib.GBKString(finfo.Name())
				buffer.Write(gbkbyte)
			}
			buffer.WriteString("\r\n")
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
				if client.clientUtf8{
					buffer.WriteString(info.Name())
				}else{
					gbkbyte,_:= DxCommonLib.GBKString(info.Name())
					buffer.Write(gbkbyte)
				}
				buffer.WriteString("\r\n")
			}
			return nil
		})

		buffer.WriteString("\r\n")
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
	buffer := srv.GetBuffer(4096)
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
			if client.clientUtf8{
				buffer.WriteString(info.Name())
			}else{
				gbkbyte,_:= DxCommonLib.GBKString(info.Name())
				buffer.Write(gbkbyte)
			}
			buffer.WriteString("\r\n")
		}
		return nil
	})
	buffer.WriteString("\r\n")
	con.WriteObject(&ftpResponsePkg{226,"Closing data connection, sent " + strconv.Itoa(buffer.Len()) + " bytes",false})
	buffer.WriteTo(client.datacon)
	client.datacon.Close()
	client.datacon = nil
	srv.ReciveBuffer(buffer)
}


func (cmd cmdLIST)MustUtf8()bool  {
	return true
}

func (cmd cmdLIST)HasParams()bool  {
	return false
}

func (cmd cmdLIST)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	con.WriteObject(&openAsCiiModePkg)
	var fpath string
	client := con.GetUseData().(*ftpClientBinds)
	dataclient := client.datacon.GetUseData().(*dataClientBinds)
	if !client.user.Permission.CanListDir(){
		close(dataclient.waitReadChan) //通知可以读了
		con.WriteObject(&noListPermission)
		return
	}
	close(dataclient.waitReadChan) //通知可以读了
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
	var err error
	path := srv.localPath(srv.buildPath(client.curPath, fpath))
	if path == ""{
		//根目录，直接返回根目录结构
		buffer := srv.GetBuffer(4096)
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
			if client.clientUtf8{
				buffer.WriteString(finfo.Name())
			}else{
				gbkbyte,_:= DxCommonLib.GBKString(finfo.Name())
				buffer.Write(gbkbyte)
			}
			buffer.WriteString("\r\n")
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
				if client.clientUtf8{
					buffer.WriteString(info.Name())
				}else{
					gbkbyte,_:= DxCommonLib.GBKString(info.Name())
					buffer.Write(gbkbyte)
				}
				buffer.WriteString("\r\n")
			}
			return nil
		})

		buffer.WriteString("\r\n")
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
		con.WriteObject(&noDirPkg)
		return
	}
	//找到了真实的目录位置
	buffer := srv.GetBuffer(4096)
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
			if client.clientUtf8{
				buffer.WriteString(info.Name())
			}else{
				gbkbyte,_:= DxCommonLib.GBKString(info.Name())
				buffer.Write(gbkbyte)
			}
			buffer.WriteString("\r\n")
		}
		return nil
	})
	buffer.WriteString("\r\n")
	con.WriteObject(&ftpResponsePkg{226,"Closing data connection, sent " + strconv.Itoa(buffer.Len()) + " bytes",false})
	buffer.WriteTo(client.datacon)
	client.datacon.Close()
	client.datacon = nil
	srv.ReciveBuffer(buffer)
}

func (cmd cmdPASV)HasParams()bool  {
	return false
}


func (cmd cmdPASV)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//被动传输模式
	port := uint16(0)
	if  srv.dataServer != nil{
		if !srv.dataServer.Active(){
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			port = srv.MinPasvPort + uint16(r.Intn(int(srv.MaxPasvPort-srv.MinPasvPort)))
			if !srv.createDataServer(port,con){
				con.WriteObject(&dataConnectionFailedPkg)
				return
			}
		}else{
			port = srv.dataServer.port
		}
	}else{
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		port = srv.MinPasvPort + uint16(r.Intn(int(srv.MaxPasvPort-srv.MinPasvPort)))
		if !srv.createDataServer(port,con){
			con.WriteObject(&dataConnectionFailedPkg)
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
	if srv.dataServer.waitBindChan != nil{
		select{
		case <-srv.dataServer.waitBindChan:
		}
	}
	srv.dataServer.waitBindChan = make(chan struct{})
	srv.dataServer.notifyBindOk = make(chan struct{})
	srv.dataServer.curCon.Store(con)
	con.WriteObject(&ftpResponsePkg{227,fmt.Sprintf("Entering Passive Mode (%s,%s,%s,%s,%d,%d)", quads[0], quads[1], quads[2], quads[3], p1, p2),false})
	//然后执行等待链接并且绑定
	select{
	case  <-srv.dataServer.notifyBindOk:

	case <-DxCommonLib.After(time.Second*60):
		var m *ServerBase.DxNetConnection=nil
		srv.dataServer.curCon.Store(m)
	}
	close(srv.dataServer.waitBindChan)
}

func (cmd cmdCWD)MustUtf8()bool  {
	return true
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

func (cmd cmdTYPE)HasParams()bool  {
	return false
}


func (cmd cmdTYPE)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//查看或者设置当前的传输方式
	client := con.GetUseData().(*ftpClientBinds)
	paramstr = strings.ToUpper(paramstr)
	if paramstr == "A"{
		client.typeAnsi = true
		con.WriteObject(&tpSetApkg)
	}else if paramstr == "I"{
		client.typeAnsi = false
		con.WriteObject(&tbSetBpkg)
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

func (cmd cmdPWD)HasParams()bool  {
	return false
}

func (cmd cmdPWD)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	client := con.GetUseData().(*ftpClientBinds)
	con.WriteObject(&ftpResponsePkg{257,"\""+client.curPath+"\" is the current directory",false})
}

func (cmd cmdQUIT)HasParams()bool  {
	return false
}

func (cmd cmdQUIT)MustLogin() bool{
	return false
}

func (cmd cmdQUIT)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	con.WriteObject(&byePkg)
}

func (cmd cmdFEAT)MustLogin() bool{
	return false
}

func (cmd cmdFEAT)HasParams()bool  {
	return false
}

func (cmd cmdFEAT)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//获取系统支持的CMD扩展命令
	con.WriteObject(&featcmdPkg)
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
		con.WriteObject(&logokpkg)
	}else{
		if client.user.IsAnonymous && client.user.PassWord == ""{
			client.isLogin = true
			con.WriteObject(&logokpkg)
		}else{
			con.WriteObject(&logfailedpkg)
		}
	}
}

func (cmd cmdSYST)MustLogin() bool{
	return false
}

func (cmd cmdSYST)HasParams()bool  {
	return false
}

func (cmd cmdSYST)Execute(srv *FTPServer,con *ServerBase.DxNetConnection,paramstr string)  {
	//服务器远程系统的操作系统类型
	if strings.Compare(runtime.GOOS,"windows") == 0{
		con.WriteObject(&winTypePkg)
	}else{
		con.WriteObject(&unixTypePkg)
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
		ftpbinds.curPath = "/"
		ftpbinds.user = &srv.anonymousUser
		con.SetUseData(ftpbinds)
		con.WriteObject(&passWordPkg)
		return
	}
	if v,ok := srv.users.Load(paramstr);ok{
		ftpbinds := srv.getFtpClient()
		ftpbinds.typeAnsi = true
		ftpbinds.isLogin = false
		ftpbinds.cmdcon = con
		ftpbinds.curPath = "/"
		ftpbinds.user = v.(*FtpUser)
		con.SetUseData(ftpbinds)
		con.WriteObject(&passWordPkg)
		return
	}
	//检验账户
	var user *FtpUser = nil
	if srv.OnGetFtpUser != nil{
		user = srv.OnGetFtpUser(paramstr)
	}
	if user == nil {
		con.WriteObject(&canNotLogPkg)
	}else{
		user.UserID = paramstr
		ftpbinds := srv.getFtpClient()
		ftpbinds.isLogin = false
		ftpbinds.cmdcon = con
		ftpbinds.typeAnsi = true
		ftpbinds.curPath = "/"
		ftpbinds.user = user
		con.SetUseData(ftpbinds)
		con.WriteObject(&passWordPkg)
	}
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

func (coder *FtpProtocol)ParserProtocol(r *ServerBase.DxReader,con *ServerBase.DxNetConnection)(parserOk bool,datapkg interface{},e error) { //解析协议，如果解析成功，返回true，根据情况可以设定返回协议数据包
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
	result.SubInit()
	result.SetCoder(new(FtpProtocol))
	result.anonymousUser.UserID = "anonymous"
	result.anonymousUser.IsAnonymous = true

	result.anonymousUser.Permission.SetDirSubDirsPermission(true)
	result.anonymousUser.Permission.SetDirListPermission(true)
	result.anonymousUser.Permission.SetFileReadPermission(true)

	result.WelcomeMessage = "Welcom to DxGoFTP"
	result.MinPasvPort = 50000
	result.MaxPasvPort = 60000
	result.OnClientConnect = func(con *ServerBase.DxNetConnection)interface{}{
		//客户链接，发送回消息
		con.WriteObject(&ftpResponsePkg{220,result.WelcomeMessage,false})
		return nil
	}
	result.OnRecvData = func(con *ServerBase.DxNetConnection,recvData interface{}) {
		cmdpkg := recvData.(*ftpcmdpkg)
		v := DxCommonLib.FastByte2String(cmdpkg.cmd)
		fmt.Println(v)
		if cmd,ok := ftpCmds[v];!ok{
			con.WriteObject(&unknowCmdPkg)
		}else {
			var client *ftpClientBinds=nil
			if cmd.MustLogin(){
				client = con.GetUseData().(*ftpClientBinds)
				if !client.isLogin{
					con.WriteObject(&unlogPkg)
					return
				}
			}
			bt := cmdpkg.params
			if cmd.HasParams() && (bt == nil || len(bt) == 0){
				con.WriteObject(&noParamPkg)
				return
			}
			if cmd.MustUtf8(){
				if client == nil{
					client = con.GetUseData().(*ftpClientBinds)
				}
				if !client.clientUtf8 { //客户端不是用的utf8，需要转换
					if abt,err := DxCommonLib.GBK2Utf8(bt);err==nil{
						bt = abt
					}
				}
			}
			paramstr := DxCommonLib.FastByte2String(bt)
			fmt.Println(paramstr)
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
			if client,ok := v.(*ftpClientBinds);ok && client != nil{
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


//做一步继承
func (srv *ftpDataServer) SubInit() {
	srv.DxTcpServer.SubInit()
	srv.GDxBaseObject.SubInit(srv)
}

func (srv *ftpDataServer)ClientConnect(con *ServerBase.DxNetConnection) interface{}{
	v := srv.curCon.Load()
	if v!=nil{
		m := v.(*ServerBase.DxNetConnection)
		if m != nil{
			st1 := strings.SplitN(m.RemoteAddr(),":",2)
			st2 := strings.SplitN(con.RemoteAddr(),":",2)
			if st1[0] == st2[0]{
				client := m.GetUseData().(*ftpClientBinds)
				client.datacon = con
				dclient := srv.getClient()
				dclient.ftpClient = client
				dclient.waitReadChan = make(chan struct{}) //等待读取
				con.SetUseData(dclient)
				close(srv.notifyBindOk) //通知绑定成功
				//等待客户端链接上来，如果一直等不到，就关闭
				if srv.waitBindChan != nil{
					select{
					case <-srv.waitBindChan:
						srv.waitBindChan = nil
						return nil
					case <-DxCommonLib.After(time.Second):
						srv.waitBindChan = nil
						return nil
					}
				}
			}
		}

	}
	return nil
}

func (srv *ftpDataServer)getClient()*dataClientBinds  {
	v := srv.clientPool.Get()
	if v == nil{
		return new(dataClientBinds)
	}else{
		return v.(*dataClientBinds)
	}
}

func (srv *ftpDataServer)freeClient(client *dataClientBinds){
	client.ftpClient = nil
	client.f = nil
	client.curPosition = 0
	client.waitReadChan = nil
	client.lastFilePos = 0
	srv.clientPool.Put(client)
}

func (srv *ftpDataServer)dataClientDisconnect(con *ServerBase.DxNetConnection)  {
	if v,ok := con.GetUseData().(*dataClientBinds);ok && v!= nil{
		v.ftpClient.datacon = nil
		srv.freeClient(v)
	}
}


//自定义读取文件的接口
func (srv *ftpDataServer)CustomRead(con *ServerBase.DxNetConnection,targetdata interface{})bool{
	//这里需要等待con绑定到用户
	client := con.GetUseData().(*dataClientBinds)
	if client.waitReadChan != nil{
		select{
		case <-client.waitReadChan:
		}
	}
	client.waitReadChan = nil
	if client.f == nil{
		return  true
	}
	maxsize := 44*1460
	buffer := srv.GetBuffer(maxsize)
	lreader := io.LimitedReader{con,int64(maxsize)}
	for{
		rl,err := io.Copy(buffer,&lreader)
		lreader.N = int64(maxsize)
		client.ftpClient.cmdcon.LastValidTime.Store(time.Now()) //更新一下命令处理连接的操作数据时间
		if rl != 0{
			client.curPosition += uint32(rl)
			buffer.WriteTo(client.f)
			if err != nil{
				break
			}
		}else{
			break
		}
	}
	srv.ReciveBuffer(buffer)
	//读取完了，执行关闭，然后释放
	client.f.Close()
	client.f = nil
	client.ftpClient.cmdcon.WriteObject(&ftpResponsePkg{226,fmt.Sprintln("OK, received %d bytes",client.curPosition),false})
	client.curPosition = 0
	client.ftpClient.lastPos = 0
	return true
}

func (srv *FTPServer)checkDataServerIdle(params ...interface{})  {
	srvCloseChan := srv.Done()
	dataclosechan := srv.dataServer.Done()
	for{
		select{
		case <-srvCloseChan:
			return
		case <-DxCommonLib.After(time.Minute):
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

func (srv *FTPServer)CopyAnonymousUserPermissions(user *FtpUser)  {
	user.Permission = srv.anonymousUser.Permission
}

func (srv *FTPServer)createDataServer(port uint16,con *ServerBase.DxNetConnection)bool  {
	if srv.dataServer == nil{
		srv.dataServer = new(ftpDataServer)
		srv.dataServer.SubInit() //继承一步
		srv.dataServer.TimeOutSeconds = 5 //5秒钟没数据关闭
		srv.dataServer.ownerFtp = srv
		srv.dataServer.SrvLogger = srv.SrvLogger
		srv.dataServer.OnClientConnect = srv.dataServer.ClientConnect
		srv.dataServer.OnClientDisConnected = srv.dataServer.dataClientDisconnect
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

func (srv *FTPServer) SubInit() {
	srv.DxTcpServer.SubInit()
	srv.GDxBaseObject.SubInit(srv)
}

func (srv *FTPServer)SetAnonymouseDirPermission(canCreateDir,canDelDir,canListDir,canSubDirs bool)  {
	srv.anonymousUser.Permission.SetDirCreatePermission(canCreateDir)
	srv.anonymousUser.Permission.SetDirListPermission(canListDir)
	srv.anonymousUser.Permission.SetDirDelPermission(canDelDir)
	srv.anonymousUser.Permission.SetDirSubDirsPermission(canSubDirs)
}

func (srv *FTPServer)SetAnonymouseFilePermission(canRead,canWrite,canAppend,canDelete bool)  {
	srv.anonymousUser.Permission.SetFileReadPermission(canRead)
	srv.anonymousUser.Permission.SetFileAppendPermission(canAppend)
	srv.anonymousUser.Permission.SetFileDelPermission(canDelete)
	srv.anonymousUser.Permission.SetFileWritePermission(canWrite)
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

func (srv *FTPServer)localPath(ftpdir string)string  {
	paths := strings.Split(ftpdir, "/")
	rdir := strings.ToUpper(paths[0]) //第一个目录是用户指定的目录
	storedir := rdir
	if rdir==""  && len(paths)>1{
		rdir = paths[1]
		storedir = rdir
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
		return ""
	}
	if v,ok := srv.ftpDirectorys.Load(rdir);ok{
		dirs := v.(*ftpDirs)
		return filepath.Join(append([]string{dirs.localPathName}, paths...)...)
	}
	//不是根目录中的目录，那么认为是主目录中的
	return filepath.Join(append([]string{srv.maindir.localPathName,storedir}, paths...)...)
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
	client.isLogin = false
	client.reNameFrom = ""
	client.lastPos = 0
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