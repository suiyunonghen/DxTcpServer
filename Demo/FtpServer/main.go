package main
import (
	"github.com/suiyunonghen/DxTcpServer/Protocols/Ftp"
	"github.com/suiyunonghen/GVCL/Components/Controls"
	"github.com/suiyunonghen/GVCL/Components/NVisbleControls"
	"github.com/suiyunonghen/GVCL/WinApi"
	"unsafe"
	"syscall"
)

func main()  {
	app := controls.NewApplication()
	srv := Ftp.NewFtpServer()

	//设置anonymouse可以下载
	srv.SetAnonymouseFilePermission(true,true,true,false)

	//FTP登录的时候，新增用户,赋予密码以及权限等
	srv.OnGetFtpUser = func(userId string) *Ftp.FtpUser {
		if userId == "DxSoft"{
			result := new(Ftp.FtpUser)
			result.UserID = userId
			result.PassWord = "DxSoft"
			//赋值权限为匿名用户权限
			srv.CopyAnonymousUserPermissions(result)
			result.Permission.SetFileWritePermission(true)
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
	srv.MapDir("FtpDir1","F:\\FTp1",true)
	//srv.MapDir("FtpDir2","H:\\FtpDir2",false)
	srv.Open("127.0.0.1:8340")
	app.Run()
}
