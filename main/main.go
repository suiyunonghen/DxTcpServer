package main

import (
	"DxFileServer/src/EchoDemo"
	"suiyunonghen/GVCL/Components/Controls"
	"suiyunonghen/GVCL/Components/NVisbleControls"
	"suiyunonghen/GVCL/WinApi"
	"suiyunonghen/DxTcpServer"
	"unsafe"
	"syscall"
)
var (
	srv	*dxserver.DxTcpServer
)
func main() {
	app := controls.NewApplication()
	srv = EchoDemo.NewEchoServer()

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
	trayicon := NVisbleControls.NewTrayIcon(mainForm)
	trayicon.PopupMenu = PopMenu
	trayicon.SetVisible(true)
	//在GUI运行之前，开启文件服务功能
	srv.Open("127.0.0.1:8340")
	app.Run()
}
