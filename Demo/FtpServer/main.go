package main
import (
	"github.com/suiyunonghen/DxTcpServer/Protocols/Ftp"
	"github.com/suiyunonghen/GVCL/Components/Controls"
	"github.com/suiyunonghen/GVCL/Components/NVisbleControls"
	"github.com/suiyunonghen/GVCL/WinApi"
	"unsafe"
	"syscall"
	"fmt"
)

func main()  {
	a := uint8(Ftp.Permission_Dir_List)
	fmt.Println(^a)
	app := controls.NewApplication()
	srv := Ftp.NewFtpServer()

	srv.OnGetFtpUser = func(userId string) *Ftp.FtpUser {
		if userId == "DxSoft"{
			result := new(Ftp.FtpUser)
			result.UserID = userId
			srv.CopyAnonymousUserPermissions(result)
			//赋值权限
			return result
		}
		return nil
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
	srv.MapDir("FtpDir1","H:\\FtpDir\\Dir1",true)
	srv.MapDir("FtpDir2","H:\\FtpDir2",false)
	srv.Open("127.0.0.1:8340")
	app.Run()
}
