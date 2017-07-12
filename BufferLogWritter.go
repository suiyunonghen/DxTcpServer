package DxTcpServer

import(
	"sync"
	"time"
	"os"
	"os/exec"
	"fmt"
	"strings"
	"io/ioutil"
	"io"
	"path/filepath"
)

type(
	BufferLoggerWriter struct {
		logfiletag	string			//文件标记名称
		logDataSize	int			//记录的文件大小
		bufferchan	chan []byte		//缓存
		writefilelock	sync.Mutex
		quitchan	chan struct{}
		datachan	chan []byte
	}
)


func (loggerWriter *BufferLoggerWriter)Write(p []byte) (n int, err error)  {
	//通知有数据到来
	if loggerWriter.datachan!=nil{
		mp := loggerWriter.getBuffer(len(p))
		copy(mp,p)
		loggerWriter.datachan <- mp //通知数据到来
		return len(p),nil
	}
	return 0,io.EOF
}

func (loggerWriter *BufferLoggerWriter)QuitWriter(){
	close(loggerWriter.datachan)
	loggerWriter.datachan = nil
	if loggerWriter.bufferchan!=nil{
		close(loggerWriter.bufferchan)
	}
	loggerWriter.quitchan<- struct{}{}
}

func (loggerWriter *BufferLoggerWriter)getBuffer(buflen int)(retbuf []byte)  {
	var ok bool
	caplen := buflen
	if caplen < 512{
		caplen = 512
	}
	if loggerWriter.bufferchan != nil{
		select{
		case retbuf,ok = <-loggerWriter.bufferchan:
			if !ok || cap(retbuf)<buflen{
				retbuf = make([]byte,buflen,caplen)
			}
		default:
			retbuf = make([]byte,buflen,caplen)
		}
	}else if loggerWriter.bufferchan == nil {
		loggerWriter.bufferchan = make(chan []byte,100)
		retbuf = make([]byte,buflen,caplen)
	}else{
		retbuf = make([]byte,buflen,caplen)
	}
	retbuf = retbuf[:buflen]
	return
}

func (loggerWriter *BufferLoggerWriter)reciveBuffer(buf []byte)bool  {
	if loggerWriter.bufferchan != nil{
		select{
		case loggerWriter.bufferchan <- buf:
			return true
		case <-After(time.Second * 5):
			//回收失败
			return false
		}
	}
	return false
}

func (loggerWriter *BufferLoggerWriter)WriteData2File()  {
	curExeDir,_ := exec.LookPath(os.Args[0])
	logdir := filepath.Dir(curExeDir)
	logdir = fmt.Sprintf("%s%s",logdir,"\\log\\")
	//判断目录是否存在
	curExeDir = ""
	if _,err := os.Stat(logdir);err != nil{
		os.MkdirAll(logdir,0777)
	}else{
		//先找到一个最新的文件
		if dir, err := ioutil.ReadDir(logdir);err == nil{
			tmpName := ""
			for _, fi := range dir {
				if !fi.IsDir() && strings.HasSuffix(strings.ToLower(fi.Name()), ".log") {
					//找到文件
					tmpName = fmt.Sprintf("%s%s",logdir,fi.Name())
					if finfo,err := os.Stat(tmpName);err == nil && finfo.Size() < 10*1024*1024{
						curExeDir = tmpName
						break
					}
				}
			}
		}
	}
	if curExeDir == ""{
		curExeDir = fmt.Sprintf("%s%s%s.log",logdir,loggerWriter.logfiletag,time.Now().Format("2006-01-02_15_04_05"))
	}
	if file,err := os.OpenFile(curExeDir, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666);err==nil{
		file.Seek(0,os.SEEK_END) //移动到末尾
		for{
			select {
			case bt, ok := <-loggerWriter.datachan:
				//有数据过来，写入文件
				if ok {
					loggerWriter.writefilelock.Lock()
					if wlen, err := file.Write(bt); err == nil {
						loggerWriter.logDataSize += wlen
						if loggerWriter.logDataSize >= 10*1024*1024 { //10M
							file.Close() //超过60M，创建新文件
							loggerWriter.logDataSize = 0
							curExeDir = fmt.Sprintf("%s%s%s%s.log", logdir, "\\log\\",
								loggerWriter.logfiletag, time.Now().Add(time.Second).Format("2006-01-02_15_04_05"))
							if file, err = os.OpenFile(curExeDir, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666);
								err != nil {
								loggerWriter.writefilelock.Unlock()
								loggerWriter.reciveBuffer(bt)
								return
							}
						}
					}
					loggerWriter.writefilelock.Unlock()
					loggerWriter.reciveBuffer(bt)
				}
			case <-loggerWriter.quitchan:
				file.Close()
				return
			}
		}
	}
}

func NewLoggerBufferWriter() *BufferLoggerWriter {
	bufferWriter := new(BufferLoggerWriter)
	bufferWriter.quitchan = make(chan struct{})
	bufferWriter.datachan = make(chan []byte,20)
	return bufferWriter
}