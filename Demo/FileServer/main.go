package main

import (
	"github.com/suiyunonghen/DxTcpServer/Demo/FileServer/DxFileServer"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"github.com/suiyunonghen/GVCL/Components/Controls"
	"github.com/suiyunonghen/GVCL/Components/NVisbleControls"
	"github.com/suiyunonghen/GVCL/WinApi"
	"syscall"
	"time"
	"unsafe"
)

var (
	curExecDir      string
	srv             *Filesrv.DxFileServer
	ServerStartTime string
)

type userClient struct {
	Index        int
	ClientIp     string
	LogTime      string
	LastDataTime string
	RecvDatas    template.HTML
	SendDatas    template.HTML
}

//模板中的数据内容信息
type templateData struct {
	StartTime        string //模板开始时间
	RunTimes         string
	RequestCount     uint64
	SendRequestCount uint64
	RecvDatas        template.HTML
	SendDatas        template.HTML
	UserClients      []*userClient
	ClientCount      uint
}

func srvInfoHandler(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles(fmt.Sprintf("%s%s", curExecDir, "/template/html/srvInfo.html"))
	if err != nil {
		io.WriteString(w, err.Error())

	} else {
		templatedata := &templateData{}
		templatedata.StartTime = ServerStartTime
		templatedata.RequestCount = srv.RequestCount
		templatedata.SendRequestCount = srv.SendRequestCount
		srv.Lock()
		clients := srv.GetClients()
		templatedata.ClientCount = uint(len(clients))
		templatedata.UserClients = make([]*userClient, len(clients))
		idx := 0
		for _, v := range clients {
			client := new(userClient)
			client.Index = idx + 1
			client.ClientIp = v.RemoteAddr()
			client.LastDataTime = v.LastValidTime.Load().(time.Time).Format("2006-01-02 15:04:05")
			client.LogTime = v.LoginTime.Format("2006-01-02 15:04:05")
			client.SendDatas = template.HTML(v.SendDataLen.ToString(true))
			client.RecvDatas = template.HTML(v.ReciveDataLen.ToString(true))
			templatedata.UserClients[idx] = client
			idx += 1
		}
		templatedata.SendDatas = template.HTML(srv.SendDataSize.ToString(true))
		templatedata.RecvDatas = template.HTML(srv.RecvDataSize.ToString(true))
		srv.Unlock()
		t.Execute(w, templatedata)
	}
}

func main() {
	curExecDir, _ = exec.LookPath(os.Args[0])
	curExecDir = filepath.Dir(curExecDir)
	app := controls.NewApplication()

	srv = Filesrv.NewFileServer(curExecDir)

	app.ShowMainForm = false
	mainForm := app.CreateForm()
	PopMenu := NVisbleControls.NewPopupMenu(mainForm)
	mItem := PopMenu.Items().AddItem("服务信息")
	mItem.OnClick = func(sender interface{}) {
		//通过网页返回服务端消息
		WinApi.ShellExecute(mainForm.GetWindowHandle(), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("OPEN"))),
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("http://127.0.0.1:8344/srvInfo"))), 0, 0, WinApi.SW_SHOWNORMAL)
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
	ServerStartTime = time.Now().Format("2006-01-02 15:04:05")
	//增加模板信息
	http.Handle("/css/", http.FileServer(http.Dir(fmt.Sprintf("%s%s", curExecDir, "/template"))))
	http.Handle("/js/", http.FileServer(http.Dir(fmt.Sprintf("%s%s", curExecDir, "/template"))))
	http.Handle("/bootstrap/", http.FileServer(http.Dir(fmt.Sprintf("%s%s", curExecDir, "/template"))))
	http.Handle("/jquery/", http.FileServer(http.Dir(fmt.Sprintf("%s%s", curExecDir, "/template"))))
	http.HandleFunc("/srvInfo", srvInfoHandler)
	go http.ListenAndServe(":8344", nil)

	app.Run()
}
