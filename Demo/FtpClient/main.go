package main

import (
	"github.com/suiyunonghen/DxTcpServer/Protocols/Ftp"
	"fmt"
	"time"
	"github.com/suiyunonghen/GVCL/Components/Controls"
	"github.com/suiyunonghen/GVCL/Components/NVisbleControls"
	"github.com/suiyunonghen/GVCL/WinApi"
	"unsafe"
	"syscall"
)

func main()  {
	app := controls.NewApplication()
	client := Ftp.NewFtpClient(Ftp.TDM_PORT)
	client.OnResultResponse = func(responseCode uint16, msg string) {
		fmt.Println("ResponseInfo:  ",responseCode,"  ",msg)
	}

	client.OnDataProgress = func(TotalSize int, CurPos int, TransTimes time.Duration, transOk bool) {
		if TransTimes == -1{
			fmt.Println("开始传输")
		}else if transOk{
			fmt.Println("传输完成，共传输数据：",TotalSize," 传输时长：",TransTimes)
		}
	}

	app.ShowMainForm = false
	mainForm := app.CreateForm()
	PopMenu := NVisbleControls.NewPopupMenu(mainForm)
	mItem := PopMenu.Items().AddItem("服务信息")
	mItem.OnClick = func(sender interface{}) {
		//通过网页返回服务端消息
		WinApi.ShellExecute(mainForm.GetWindowHandle(),uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("OPEN"))),
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("https://github.com/suiyunonghen"))),0,0,WinApi.SW_SHOWNORMAL)
	}

	mItem = PopMenu.Items().AddItem("连接FTP")
	mItem.OnClick = func(sender interface{}) {
		if !client.Active(){
			client.Connect("127.0.0.1:8340")



			client.ListDir("html", func(ftpFileinfo *Ftp.FTPFile) {
				fmt.Println(ftpFileinfo)
			})

			if err := client.Login("DxSoft","DxSoft");err == nil{
				fmt.Println("login OK")
			}else{
				fmt.Println("login failed:",err)
			}
			if _,err := client.ExecuteFtpCmd("PWD","",nil);err != nil{
				return
			}
			client.ExecuteFtpCmd("OPTS","UTF8 ON",nil)
		}
		//获取目录信息
		client.ExecuteFtpCmd("CWD","html",nil)
		client.ExecuteFtpCmd("CWD","html",nil)

		fmt.Println("List: ")

		client.ListDir("html", func(ftpFileinfo *Ftp.FTPFile) {
			fmt.Println(ftpFileinfo)
		})

		client.ListDir("/html5Test/html5test/css", func(ftpFileinfo *Ftp.FTPFile) {
			fmt.Println(ftpFileinfo)
		})

		fmt.Println("ASdf")
		client.ListDir("/html5Test/html5test/css", func(ftpFileinfo *Ftp.FTPFile) {
			fmt.Println(ftpFileinfo)
		})

		//client.DownLoad("HoorayOS 2.0.0.zip","d:\\tt.zip",0)
		//client.UpLoad("D:\\病历服务器端（三层结构）.rar","病历服务器.rar",0)
	}

	mItem = PopMenu.Items().AddItem("-")
	mItem = PopMenu.Items().AddItem("退出")
	mItem.OnClick = func(sender interface{}) {
		mainForm.Close()
	}
	trayicon := NVisbleControls.NewTrayIcon(mainForm)
	trayicon.PopupMenu = PopMenu
	trayicon.SetVisible(true)
	app.Run()
}