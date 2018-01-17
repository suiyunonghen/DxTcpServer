package Ftp

import (
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"bytes"
	"io"
	"time"
	"fmt"
	"os"
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
)


func (dataClient *ftpDataClient) SubInit() {
	dataClient.DxTcpClient.SubInit()
	dataClient.GDxBaseObject.SubInit(dataClient)
}

func (dataclient *ftpDataClient)CustomRead(con *ServerBase.DxNetConnection,targetdata interface{})bool  {
	if dataclient.clientbind.waitReadChan != nil{
		select{
		case <-dataclient.clientbind.waitReadChan:
		}
	}
	dataclient.clientbind.waitReadChan = nil
	var clientFtp *FtpClient
	if ftp,ok := dataclient.clientPools.(*FtpClient);ok{
		clientFtp = ftp
	}
	if dataclient.clientbind.f == nil{
		return  true
	}
	lreader := io.LimitedReader{con,44*1460}
	for{
		rl,err := io.Copy(dataclient.dataBuffer,&lreader)
		lreader.N = 44*1460
		if clientFtp != nil{
			clientFtp.Clientcon.LastValidTime.Store(time.Now())
		}else{
			dataclient.clientbind.ftpClient.cmdcon.LastValidTime.Store(time.Now()) //更新一下命令处理连接的操作数据时间
		}
		if rl != 0{
			dataclient.clientbind.curPosition += uint32(rl)
			dataclient.dataBuffer.WriteTo(dataclient.clientbind.f)
			if err != nil{
				break
			}
		}else{
			break
		}
	}
	//读取完了，执行关闭，然后释放
	if f,ok := dataclient.clientbind.f.(*os.File);ok{
		f.Close()
	}
	dataclient.clientbind.f = nil
	if clientFtp != nil{
		close(clientFtp.transOk)
		fmt.Println("ftpClient Done")
	}else{
		dataclient.clientbind.ftpClient.cmdcon.WriteObject(&ftpResponsePkg{226,fmt.Sprintln("OK, received %d bytes",dataclient.clientbind.curPosition),false})
	}
	dataclient.clientbind.curPosition = 0
	dataclient.clientbind.ftpClient.lastPos = 0
	return true
}

func (client *ftpDataClient)onDataClientClose(con *ServerBase.DxNetConnection)  {
	client.clientPools.freeDataClient(client)
}
