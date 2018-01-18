package main

import (
	"github.com/suiyunonghen/DxTcpServer/Demo/EchoDemo"
	"github.com/suiyunonghen/GVCL/Components/Controls"
	"github.com/suiyunonghen/GVCL/Components/NVisbleControls"
	"github.com/suiyunonghen/GVCL/WinApi"
	"github.com/suiyunonghen/DxTcpServer/ServerBase"
	"unsafe"
	"syscall"
	"fmt"
	"time"
)
var (
	srv	*ServerBase.DxTcpServer
	client *ServerBase.DxTcpClient
)
func main() {
	app := controls.NewApplication()
	srv = EchoDemo.NewEchoServer()
	client = EchoDemo.NewEchoClient()
	app.ShowMainForm = false
	mainForm := app.CreateForm()
	PopMenu := NVisbleControls.NewPopupMenu(mainForm)
	mItem := PopMenu.Items().AddItem("服务信息")
	mItem.OnClick = func(sender interface{}) {
		//通过网页返回服务端消息
		WinApi.ShellExecute(mainForm.GetWindowHandle(),uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("OPEN"))),
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("https://github.com/suiyunonghen"))),0,0,WinApi.SW_SHOWNORMAL)
	}

	mItem = PopMenu.Items().AddItem("-")
	mItem = PopMenu.Items().AddItem("退出")
	mItem.OnClick = func(sender interface{}) {
		srv.Close()
		mainForm.Close()
	}

	mItem = PopMenu.Items().AddItem("发送Echo消息")
	mItem.OnClick = func(sender interface{}) {
		client.SendData(&client.Clientcon,"测试不得闲")
	}

	trayicon := NVisbleControls.NewTrayIcon(mainForm)
	trayicon.PopupMenu = PopMenu
	trayicon.SetVisible(true)
	//在GUI运行之前，开启文件服务功能
	srv.Open("127.0.0.1:8340")
	client.Connect("127.0.0.1:8340")
	app.Run()
}
