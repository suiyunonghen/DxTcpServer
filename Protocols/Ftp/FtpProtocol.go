package Ftp

import (
	"bytes"
	"strconv"
	"io"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"github.com/suiyunonghen/DxCommonLib"
	"fmt"
)

type  FtpProtocol	struct{
	isClient		bool
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
		return false,nil,err
	}
	e = err
	if e != nil{
		return false,nil,e
	}
	parserOk = true
	var bt [1]byte
	bt[0]=' '
	linebyte = bytes.Trim(linebyte,"\r\n")
	params := bytes.SplitN(linebyte,bt[:],2)
	if coder.isClient{
		ftpclient := con.GetUseData().(*FtpClient)
		if ftpclient.connectOk != nil{
			if string(params[0]) == "220"{
				close(ftpclient.connectOk)
				ftpclient.connectOk = nil
				return
			}
		}
		if ftpclient.curMulResponsMsg != nil{
			if len(params) == 2 && string(bytes.ToUpper(params[0])) == "" &&
				DxCommonLib.FastByte2String(bytes.ToUpper(params[1])) == "END"{
				datapkg = ftpclient.curMulResponsMsg
				ftpclient.curMulResponsMsg = nil
			}else{
				ftpclient.curMulResponsMsg.responseMsg = fmt.Sprintf("%s\n%s",ftpclient.curMulResponsMsg,DxCommonLib.FastByte2String(params[1]))
			}
		}else if len(params[0])>3 && params[0][3] == '-'{
			bt[0] = '-'
			params = bytes.SplitN(linebyte,bt[:],2)
			code,err := strconv.Atoi(DxCommonLib.FastByte2String(params[0]))
			e = err
			if err != nil{
				return
			}
			ftpclient.curMulResponsMsg = &ftpResponsePkg{uint16(code),string(params[1]),true}
		}else{
			code,err := strconv.Atoi(DxCommonLib.FastByte2String(params[0]))
			e = err
			if err != nil{
				return
			}
			datapkg = &ftpResponsePkg{uint16(code),string(params[1]),false}
		}
		return
	}
	cmdpkg := &ftpcmdpkg{bytes.ToUpper(params[0]),nil}
	if len(params) == 2{
		cmdpkg.params = params[1]
	}
	datapkg = cmdpkg
	return
}

func (coder *FtpProtocol)PacketObject(objpkg interface{},buffer *bytes.Buffer)([]byte,error) { //将发送的内容打包
	if coder.isClient{
		return nil,nil
	}
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