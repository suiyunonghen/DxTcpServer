package main

import (
	"DxFileClient/FileClient"
	"fmt"
	"suiyunonghen/GVCL/Components/Controls"
	"suiyunonghen/GVCL/Components/NVisbleControls"
)

var (
	fclient *FileClient.FileClient
)

func main() {
	app := controls.NewApplication()
	app.ShowMainForm = false
	mainForm := app.CreateForm()
	PopMenu := NVisbleControls.NewPopupMenu(mainForm)

	fclient = FileClient.NewFileClient()
	fclient.OnDownLoad = func(client *FileClient.FileClient, FileName string, TotalSize, Position int64) {
		fmt.Println(FileName, "正在文件下载", Position*100/TotalSize, "%")
	}
	mItem := PopMenu.Items().AddItem("下载文件")
	mItem.OnClick = func(sender interface{}) {
		fclient.DownLoadFile("第2版394860.pdf", "d:\\2.pdf")
		fclient.DownLoadFile("游戏编程大师技巧(第二版.pdf", "d:\\3.pdf")
	}
	mItem = PopMenu.Items().AddItem("-")
	mItem = PopMenu.Items().AddItem("退出")
	mItem.OnClick = func(sender interface{}) {
		fclient.Close()
		mainForm.Close()
	}
	trayicon := NVisbleControls.NewTrayIcon(mainForm)
	trayicon.PopupMenu = PopMenu
	trayicon.SetVisible(true)
	fclient.Connect("127.0.0.1:8340")
	app.Run()
}
