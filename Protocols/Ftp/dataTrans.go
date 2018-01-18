package Ftp

import (
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"bytes"
	"io"
	"time"
	"fmt"
	"os"
	"sync/atomic"
	"sync"
	"strings"
	"github.com/suiyunonghen/DxCommonLib"
)

type(
	//主动模式的数据连接端
	iClientPools	interface{
		getDataClient(client *ftpClientBinds)(dataclient *ftpDataClient)
		freeDataClient(client *ftpDataClient)
	}

	ftpDataClient    struct{
		clientPools			iClientPools
		ServerBase.DxTcpClient
		clientbind			dataClientBinds
		dataBuffer			*bytes.Buffer
	}


	//数据服务器
	ftpDataServer  struct{
		ServerBase.DxTcpServer
		port				uint16
		curCon				atomic.Value
		lastDataTime		atomic.Value
		waitBindChan		chan struct{}
		notifyBindOk		chan struct{}
		clientPool			sync.Pool
	}
)


func (dataClient *ftpDataClient) SubInit() {
	dataClient.DxTcpClient.SubInit()
	dataClient.GDxBaseObject.SubInit(dataClient)
}

func (dataclient *ftpDataClient)CustomRead(con *ServerBase.DxNetConnection,targetdata interface{})bool  {
	var clientFtp *FtpClient
	startTime := time.Now()
	if ftp,ok := dataclient.clientPools.(*FtpClient);ok{
		clientFtp = ftp
	}

	if clientFtp == nil && dataclient.clientbind.waitReadChan != nil{
		select{
		case <-dataclient.clientbind.waitReadChan:
		}
	}
	dataclient.clientbind.waitReadChan = nil

	if dataclient.clientbind.f == nil{
		return  true
	}

	f,isFile := dataclient.clientbind.f.(*os.File)
	if isFile && clientFtp != nil{
		if clientFtp.OnDataProgress != nil{
			clientFtp.OnDataProgress(int(clientFtp.curTotalSize),0,-1,false)
		}
	}
	lreader := io.LimitedReader{con,44*1460}
	for{
		rl,err := io.Copy(dataclient.dataBuffer,&lreader)
		lreader.N = 44*1460
		if rl != 0{
			rl,err = dataclient.dataBuffer.WriteTo(dataclient.clientbind.f)
			dataclient.clientbind.curPosition += uint32(rl)
			if err != nil{
				break
			}
		}else{
			break
		}
		curtime := time.Now()
		if clientFtp != nil{
			clientFtp.Clientcon.LastValidTime.Store(curtime)
			if isFile && clientFtp.OnDataProgress != nil{
				clientFtp.OnDataProgress(int(clientFtp.curTotalSize),int(dataclient.clientbind.curPosition),curtime.Sub(startTime),false)
			}
		}else{
			dataclient.clientbind.ftpClient.cmdcon.LastValidTime.Store(curtime) //更新一下命令处理连接的操作数据时间
		}
	}
	//读取完了，执行关闭，然后释放
	if isFile{
		f.Close()
	}
	dataclient.clientbind.f = nil
	if clientFtp != nil{
		close(clientFtp.transOk)
		clientFtp.transOk = nil
		if isFile && clientFtp.OnDataProgress != nil{
			clientFtp.OnDataProgress(int(clientFtp.curTotalSize),int(dataclient.clientbind.curPosition),time.Now().Sub(startTime),true)
		}
	}else{
		dataclient.clientbind.ftpClient.cmdcon.WriteObject(&ftpResponsePkg{226,fmt.Sprintf("OK, received %d bytes",dataclient.clientbind.curPosition),false})
	}
	dataclient.clientbind.curPosition = 0
	dataclient.clientbind.ftpClient.lastPos = 0
	return true
}

func (client *ftpDataClient)onDataClientClose(con *ServerBase.DxNetConnection)  {
	client.clientPools.freeDataClient(client)
}


//做一步继承
func (srv *ftpDataServer) SubInit() {
	srv.DxTcpServer.SubInit()
	srv.GDxBaseObject.SubInit(srv)
}

func (srv *ftpDataServer)ClientConnect(con *ServerBase.DxNetConnection) interface{}{
	v := srv.curCon.Load()
	if v!=nil{
		m,ok := v.(*ServerBase.DxNetConnection)
		if ok && m != nil{
			st1 := strings.SplitN(m.RemoteAddr(),":",2)
			st2 := strings.SplitN(con.RemoteAddr(),":",2)
			if st1[0] == st2[0]{
				v := m.GetUseData()
				var client *ftpClientBinds
				if mclient,ok := v.(*ftpClientBinds);ok{
					client = mclient
				}else{
					client = &v.(*FtpClient).ftpClientBinds
				}
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
	startTime := time.Now()
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
	var ftpclient *FtpClient
	v := client.ftpClient.cmdcon.GetUseData()
	if v != nil{
		if c,ok := v.(*FtpClient);ok{
			ftpclient = c
		}
	}

	f,isFile := client.f.(*os.File)
	if isFile && ftpclient != nil{
		if ftpclient.OnDataProgress != nil{
			ftpclient.OnDataProgress(int(ftpclient.curTotalSize),0,-1,false)
		}
	}

	maxsize := 44*1460
	buffer := srv.GetBuffer(maxsize)
	lreader := io.LimitedReader{con,int64(maxsize)}
	for{
		rl,err := io.Copy(buffer,&lreader)
		lreader.N = int64(maxsize)
		if rl != 0{
			client.curPosition += uint32(rl)
			buffer.WriteTo(client.f)
			if err != nil{
				break
			}
		}else{
			break
		}
		curtime := time.Now()
		client.ftpClient.cmdcon.LastValidTime.Store(curtime) //更新一下命令处理连接的操作数据时间
		if isFile && ftpclient != nil && ftpclient.OnDataProgress != nil{
			ftpclient.OnDataProgress(int(ftpclient.curTotalSize),int(client.curPosition),curtime.Sub(startTime),false)
		}
	}
	srv.ReciveBuffer(buffer)
	//读取完了，执行关闭，然后释放
	if isFile{
		f.Close()
	}
	client.f = nil

	if ftpclient != nil{
		close(ftpclient.transOk)
		ftpclient.transOk = nil
		if isFile && ftpclient.OnDataProgress != nil{
			ftpclient.OnDataProgress(int(ftpclient.curTotalSize),int(client.curPosition),time.Now().Sub(startTime),true)
		}
	}else{
		client.ftpClient.cmdcon.WriteObject(&ftpResponsePkg{226,fmt.Sprintf("OK, received %d bytes",client.curPosition),false})
	}
	client.curPosition = 0
	client.ftpClient.lastPos = 0
	return true
}
