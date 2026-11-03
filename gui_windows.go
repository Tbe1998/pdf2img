//go:build windows

package main

// 纯 Win32 界面（零第三方库）。编译产物是一个独立 exe + 随包的 poppler，
// 在 Windows 上双击即用，不需要安装 Python / .NET / 运行库。

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")

	pRegisterClassExW   = user32.NewProc("RegisterClassExW")
	pCreateWindowExW    = user32.NewProc("CreateWindowExW")
	pDefWindowProcW     = user32.NewProc("DefWindowProcW")
	pGetMessageW        = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessageW   = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pShowWindow         = user32.NewProc("ShowWindow")
	pUpdateWindow       = user32.NewProc("UpdateWindow")
	pSendMessageW       = user32.NewProc("SendMessageW")
	pGetWindowTextW     = user32.NewProc("GetWindowTextW")
	pSetWindowTextW     = user32.NewProc("SetWindowTextW")
	pEnableWindow       = user32.NewProc("EnableWindow")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pSetTimer           = user32.NewProc("SetTimer")
	pMessageBoxW        = user32.NewProc("MessageBoxW")
	pLoadCursorW        = user32.NewProc("LoadCursorW")
	pLoadIconW          = user32.NewProc("LoadIconW")
	pCreateFontW        = gdi32.NewProc("CreateFontW")
	pAdjustWindowRect   = user32.NewProc("AdjustWindowRect")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pGetDpiForSystem    = user32.NewProc("GetDpiForSystem")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW     = kernel32.NewProc("CreateMutexW")

	pSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	pShellExecuteW        = shell32.NewProc("ShellExecuteW")
	pGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	pShellNotifyIconW     = shell32.NewProc("Shell_NotifyIconW")
	pFindWindowW          = user32.NewProc("FindWindowW")
	pPostMessageW         = user32.NewProc("PostMessageW")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	pCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	pAppendMenuW          = user32.NewProc("AppendMenuW")
	pTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	pDestroyMenu          = user32.NewProc("DestroyMenu")
	pGetCursorPos         = user32.NewProc("GetCursorPos")
	pBeginPaint           = user32.NewProc("BeginPaint")
	pEndPaint             = user32.NewProc("EndPaint")
	pCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	pCoInitializeEx       = ole32.NewProc("CoInitializeEx")

	pDragAcceptFiles  = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW   = shell32.NewProc("DragQueryFileW")
	pDragFinish       = shell32.NewProc("DragFinish")
	pSetWindowPos     = user32.NewProc("SetWindowPos")
	pGetDlgItem       = user32.NewProc("GetDlgItem")
	pUpdateWindowProc = user32.NewProc("UpdateWindow")
)

const (
	wsChild       = 0x40000000
	wsVisible     = 0x10000000
	wsVScroll     = 0x00200000
	wsTabStop     = 0x00010000
	wsOverlapped  = 0x00CF0000
	wsThickFrame  = 0x00040000
	wsMaximizeBox = 0x00010000
	wsMinimizeBox = 0x00020000
	wsClipChildren = 0x02000000

	esAutoHScroll         = 0x0080
	esMultiline           = 0x00000004
	esAutoVScroll         = 0x00000040
	esReadOnly            = 0x00000800
	esNumber              = 0x2000
	bsAutoCheck           = 0x00000003
	bsOwnerDraw           = 0x0000000B
	emSetSel              = 0x00B1
	emLineScroll          = 0x00B6
	emGetLineCount        = 0x00BA
	emGetFirstVisibleLine = 0x00CE
	emReplaceSel          = 0x00C2
	emScrollCaret         = 0x00B7
	emSetMargins          = 0x00D3

	cbsDropDownList = 0x0003
	cbAddString     = 0x0143
	cbSetCurSel     = 0x014E
	cbGetCurSel     = 0x0147

	pbsSmooth     = 0x0001
	pbmSetPos     = 0x0402
	pbmSetRange32 = 0x0406

	wmCreate         = 0x0001
	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmCommand        = 0x0111
	wmTimer          = 0x0113
	wmSetFont        = 0x0030
	wmCtlColorStatic = 0x0138
	wmDropFiles      = 0x0233
	bmSetCheck       = 0x00F1
	bmGetCheck       = 0x00F0

	wmTray   = 0x8001
	wmShowMe = 0x8002

	swHide            = 0
	swMinimize        = 6
	swShowNormal      = 1
	swShowMinNoActive = 7
	cwUseDefault      = 0x80000000

	mbOK        = 0x00000000
	mbYesNo     = 0x00000004
	mbIconInfo  = 0x00000040
	mbIconWarn  = 0x00000030
	mbIconQuest = 0x00000020
	idYes       = 6
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type msgT struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type rect struct{ left, top, right, bottom int32 }

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type browseInfo struct {
	hwndOwner      syscall.Handle
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle     *uint16
	ulFlags       uint32
	lpfn          uintptr
	lParam        uintptr
	iImage        int32
}

const (
	bifReturnOnlyFSDirs = 0x0001
	bifEditBox          = 0x0010
	bifNewDialogStyle   = 0x0040
)

// 控件 ID
const (
	idBtnAdd = 101 + iota
	idBtnAddDir
	idBtnClear
	idEditOut
	idBtnBrowseOut
	idBtnRecent
	idChkSub
	idChkSkip
	idChkOpen
	idCmbFmt
	idEditDPI
	idEditPages
	idEditQuality
	idEditPass
	idBtnStart
	idBtnStop
	idBtnOpenOut
	idBtnUpd
	idBtnQuit
	idList
	idLog
	idStatus
)

type uiEvent struct {
	kind  int
	text  string
	man   *Manifest
	done  int
	total int
	ok    bool
}

const (
	evLog = iota + 1
	evFound
	evProgress
	evDone
)

type guiApp struct {
	hwnd   syscall.Handle
	cfg    Config
	events chan uiEvent
	scale  float64
	font   syscall.Handle

	hList, hBtnAdd, hBtnAddDir, hBtnClear syscall.Handle
	hOut, hBtnBrowseOut, hBtnRecent       syscall.Handle
	hChkSub, hChkSkip, hChkOpen           syscall.Handle
	hFmt, hDPI, hPages, hQuality, hPass   syscall.Handle
	hBtnStart, hBtnStop, hBtnOpenOut      syscall.Handle
	hBtnUpd, hBtnQuit                     syscall.Handle
	hLog, hStatus, hProgress, hLblInfo    syscall.Handle

	th            theme
	cards         []rect
	editFrames    []rect
	sectionLabels []syscall.Handle
	allButtons    []syscall.Handle

	files      []string
	busy       bool
	cancel     *cancelToken
	totalPages int
	donePages  int
	runStart   time.Time
	tick       int
	trayAdded  bool
	reallyQuit bool
	logLines   int
	logBuf     []string

	mu sync.Mutex
}

var (
	app       *guiApp
	hInstance syscall.Handle
	keepAlive syscall.Handle
	trayIcon  syscall.Handle
)

func runGUI() error {
	defer func() {
		if r := recover(); r != nil {
			reportFatal("启动异常", r)
		}
	}()
	trace("==== 启动 v%s，参数 %v ====", appVersion, os.Args)
	// Win32 消息循环必须固定在同一 OS 线程
	runtime.LockOSThread()

	// 单实例
	name, _ := syscall.UTF16PtrFromString("Local\\PDF2Image-single-instance")
	h, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	keepAlive = syscall.Handle(h)
	if e == syscall.ERROR_ALREADY_EXISTS {
		cls, _ := syscall.UTF16PtrFromString("PDF2ImageWnd")
		h2, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
		runtime.KeepAlive(cls)
		if h2 != 0 {
			pPostMessageW.Call(h2, wmShowMe, 0, 0)
		}
		return nil
	}

	cleanupOldUpdates()

	if missing := preflight(); len(missing) > 0 {
		msgBox(0, "系统缺少以下函数，程序无法启动：\r\n\r\n"+strings.Join(missing, "\r\n")+
			"\r\n\r\n请把这句话发给我。", "启动失败", mbOK|mbIconWarn)
		return nil
	}

	pCoInitializeEx.Call(0, 2) // 新版文件夹对话框需要 COM

	pSetProcessDPIAware.Call()
	scale := 1.0
	if err := pGetDpiForSystem.Find(); err == nil {
		if dpi, _, _ := pGetDpiForSystem.Call(); dpi >= 72 && dpi <= 480 {
			scale = float64(dpi) / 96.0
		}
	}

	cfg := loadConfig()
	app = &guiApp{
		cfg:    cfg,
		events: make(chan uiEvent, 4096),
	}
	app.scale = scale

	hInst, _, _ := pGetModuleHandleW.Call(0)
	hInstance = syscall.Handle(hInst)

	className, _ := syscall.UTF16PtrFromString("PDF2ImageWnd")
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInstance,
		hbrBackground: syscall.Handle(6),
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	cursor, _, _ := pLoadCursorW.Call(0, uintptr(32512))
	wc.hCursor = syscall.Handle(cursor)
	icon, _, _ := pLoadIconW.Call(uintptr(hInstance), 1)
	if icon == 0 {
		icon, _, _ = pLoadIconW.Call(0, uintptr(32512))
	}
	trayIcon = syscall.Handle(icon)
	wc.hIcon = trayIcon
	wc.hIconSm = trayIcon

	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		msgBox(0, "窗口注册失败："+err.Error(), "启动失败", mbOK|mbIconWarn)
		return nil
	}

	// 设计稿 830 x 800（96 DPI 客户区）
	style := uint32(wsOverlapped)&^uint32(wsThickFrame)&^uint32(wsMaximizeBox) | wsClipChildren
	rc := rect{0, 0, app.px(830), app.px(800)}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), uintptr(style), 0)

	title, _ := syscall.UTF16PtrFromString("PDF 转图片助手  v" + appVersion)
	hwnd, _, err := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(style),
		uintptr(cwUseDefault), uintptr(cwUseDefault),
		uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top),
		0, 0, uintptr(hInstance), 0,
	)
	runtime.KeepAlive(className)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		msgBox(0, "窗口创建失败："+err.Error(), "启动失败", mbOK|mbIconWarn)
		return nil
	}

	app.hwnd = syscall.Handle(hwnd)
	// 允许把文件/文件夹直接拖进窗口
	pDragAcceptFiles.Call(hwnd, 1)

	showMin := false
	for _, a := range os.Args[1:] {
		if a == "--minimized" || a == "/minimized" {
			showMin = true
		}
	}
	if showMin {
		pUpdateWindow.Call(hwnd)
		app.hideToTray()
	} else {
		pShowWindow.Call(hwnd, swShowNormal)
		pUpdateWindow.Call(hwnd)
	}

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func preflight() []string {
	procs := map[string]*syscall.LazyProc{
		"RegisterClassExW": pRegisterClassExW, "CreateWindowExW": pCreateWindowExW,
		"DefWindowProcW": pDefWindowProcW, "GetMessageW": pGetMessageW,
		"TranslateMessage": pTranslateMessage, "DispatchMessageW": pDispatchMessageW,
		"ShowWindow": pShowWindow, "SendMessageW": pSendMessageW,
		"GetWindowTextW": pGetWindowTextW, "SetWindowTextW": pSetWindowTextW,
		"EnableWindow": pEnableWindow, "DestroyWindow": pDestroyWindow,
		"SetTimer": pSetTimer, "MessageBoxW": pMessageBoxW,
		"LoadCursorW": pLoadCursorW, "LoadIconW": pLoadIconW,
		"CreateFontW": pCreateFontW, "AdjustWindowRect": pAdjustWindowRect,
		"SetProcessDPIAware": pSetProcessDPIAware, "GetModuleHandleW": pGetModuleHandleW,
		"CreateMutexW": pCreateMutexW, "SHBrowseForFolderW": pSHBrowseForFolderW,
		"SHGetPathFromIDListW": pSHGetPathFromIDListW, "ShellExecuteW": pShellExecuteW,
		"GetOpenFileNameW": pGetOpenFileNameW, "Shell_NotifyIconW": pShellNotifyIconW,
		"FindWindowW": pFindWindowW, "PostMessageW": pPostMessageW,
		"CreatePopupMenu": pCreatePopupMenu, "AppendMenuW": pAppendMenuW,
		"TrackPopupMenu": pTrackPopupMenu, "DestroyMenu": pDestroyMenu,
		"GetCursorPos": pGetCursorPos, "BeginPaint": pBeginPaint, "EndPaint": pEndPaint,
		"CoTaskMemFree": pCoTaskMemFree, "CoInitializeEx": pCoInitializeEx,
		"DragAcceptFiles": pDragAcceptFiles, "DragQueryFileW": pDragQueryFileW,
		"DragFinish": pDragFinish, "FillRect": pFillRect, "DrawTextW": pDrawTextW,
		"DrawIconEx": pDrawIconEx, "InvalidateRect": pInvalidateRct,
		"GetClientRect": pGetClientRect, "WindowFromPoint": pWindowFromPt,
		"CreateSolidBrush": pCreateSolidBrush, "CreatePen": pCreatePen,
		"SelectObject": pSelectObject, "RoundRect": pRoundRect,
		"SetBkMode": pSetBkMode, "SetTextColor": pSetTextColor, "SetBkColor": pSetBkColor,
	}
	var missing []string
	for name, p := range procs {
		if err := p.Find(); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func (a *guiApp) px(v int32) int32 { return int32(float64(v)*a.scale + 0.5) }

func (a *guiApp) goSafe(what string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				trace("%s 协程异常：%v", what, r)
				a.logLine(fmt.Sprintf("%s 出错：%v", what, r))
			}
		}()
		fn()
	}()
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			reportFatal(fmt.Sprintf("界面消息处理异常 (msg=0x%x)", msg), r)
			ret = 0
		}
	}()
	if app == nil {
		r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r
	}
	switch msg {
	case wmCreate:
		app.hwnd = hwnd
		app.buildUI()
		return 0
	case wmCommand:
		app.onCommand(int32(wParam & 0xFFFF))
		return 0
	case wmTimer:
		app.drain()
		app.tick++
		app.trackHover()
		return 0
	case wmCtlColorStatic:
		return app.ctlColorStatic(wParam, syscall.Handle(lParam))
	case wmClose:
		app.onClose()
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		app.paintWindow(hdc)
		pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmEraseBg:
		var rc rect
		pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rc)))
		pFillRect.Call(wParam, uintptr(unsafe.Pointer(&rc)), app.th.brBg)
		return 1
	case wmDrawItem:
		if lParam != 0 {
			app.drawButton((*drawItem)(unsafe.Pointer(lParam)))
		}
		return 0
	case wmTray:
		app.onTray(lParam)
		return 0
	case wmShowMe:
		app.showFromTray()
		return 0
	case wmDropFiles:
		app.onDropFiles(wParam)
		return 0
	case wmDestroy:
		app.removeTray()
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func (a *guiApp) mk(class, text string, style uint32, x, y, w, h int32, id int32) syscall.Handle {
	cls, _ := syscall.UTF16PtrFromString(class)
	txt, _ := syscall.UTF16PtrFromString(text)
	hw, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(txt)),
		uintptr(style|wsChild|wsVisible),
		uintptr(a.px(x)), uintptr(a.px(y)), uintptr(a.px(w)), uintptr(a.px(h)),
		uintptr(a.hwnd), uintptr(id), uintptr(hInstance), 0,
	)
	runtime.KeepAlive(cls)
	runtime.KeepAlive(txt)
	hh := syscall.Handle(hw)
	if a.font != 0 && hh != 0 {
		pSendMessageW.Call(uintptr(hh), wmSetFont, uintptr(a.font), 1)
	}
	return hh
}

func (a *guiApp) buildUI() {
	a.initTheme()
	a.font = a.th.fBody

	setFont := func(h syscall.Handle, f syscall.Handle) { pSendMessageW.Call(uintptr(h), wmSetFont, uintptr(f), 1) }
	lbl := func(text string, x, y, w, h int32) syscall.Handle { return a.mk("STATIC", text, 0, x, y, w, h, 0) }
	section := func(text string, x, y, w, h int32) syscall.Handle {
		h2 := lbl(text, x, y, w, h)
		setFont(h2, a.th.fLabel)
		a.sectionLabels = append(a.sectionLabels, h2)
		return h2
	}
	edit := func(x, y, w, h int32, id int32, extra uint32) syscall.Handle {
		h2 := a.mk("EDIT", "", esAutoHScroll|wsTabStop|extra, x, y, w, h, id)
		a.editFrames = append(a.editFrames, rect{a.px(x - 2), a.px(y - 2), a.px(x + w + 2), a.px(y + h + 2)})
		pSendMessageW.Call(uintptr(h2), emSetMargins, 0x0003, uintptr(8|(8<<16)))
		return h2
	}
	btn := func(text string, x, y, w, h int32, id int32) syscall.Handle {
		b := a.mk("BUTTON", text, bsOwnerDraw|wsTabStop, x, y, w, h, id)
		a.allButtons = append(a.allButtons, b)
		return b
	}
	card := func(x, y, w, h int32) { a.cards = append(a.cards, rect{a.px(x), a.px(y), a.px(x + w), a.px(y + h)}) }

	// ① 输入
	card(16, 84, 798, 156)
	section("① 选择 PDF  ·  可多选，也可以把文件 / 文件夹直接拖进窗口", 32, 96, 620, 20)
	a.hList = a.mk("EDIT", "", wsVScroll|esMultiline|esReadOnly|esAutoVScroll|wsTabStop, 32, 116, 616, 84, idList)
	a.editFrames = append(a.editFrames, rect{a.px(30), a.px(114), a.px(650), a.px(202)})
	pSendMessageW.Call(uintptr(a.hList), emSetMargins, 0x0003, uintptr(8|(8<<16)))
	setFont(a.hList, a.th.fSmall)
	a.hBtnAdd = btn("添加 PDF…", 660, 116, 130, 30, idBtnAdd)
	a.hBtnAddDir = btn("添加文件夹…", 660, 152, 130, 30, idBtnAddDir)
	a.hBtnClear = btn("清空列表", 660, 188, 130, 26, idBtnClear)

	// ② 输出
	card(16, 250, 798, 124)
	section("② 输出文件夹  ·  不预填，每次自己选；可从“最近”里快选", 32, 262, 620, 20)
	a.hOut = edit(32, 282, 592, 30, idEditOut, 0)
	a.hBtnBrowseOut = btn("浏览…", 636, 282, 80, 30, idBtnBrowseOut)
	a.hBtnRecent = btn("最近", 724, 282, 80, 30, idBtnRecent)
	a.hChkSub = a.mk("BUTTON", "每份 PDF 单独建子文件夹", bsAutoCheck|wsTabStop, 32, 326, 250, 24, idChkSub)
	a.hChkSkip = a.mk("BUTTON", "跳过已存在的图片", bsAutoCheck|wsTabStop, 300, 326, 200, 24, idChkSkip)
	a.hChkOpen = a.mk("BUTTON", "完成后打开输出文件夹", bsAutoCheck|wsTabStop, 510, 326, 250, 24, idChkOpen)

	// ③ 参数
	card(16, 384, 798, 124)
	section("③ 转换参数", 32, 396, 300, 20)
	lbl("图片格式", 32, 420, 70, 22)
	a.hFmt = a.mk("COMBOBOX", "", wsTabStop|wsVScroll|cbsDropDownList, 32, 418, 110, 150, idCmbFmt)
	for _, it := range []string{"PNG（无损）", "JPG（体积小）"} {
		p, _ := syscall.UTF16PtrFromString(it)
		pSendMessageW.Call(uintptr(a.hFmt), cbAddString, 0, uintptr(unsafe.Pointer(p)))
		runtime.KeepAlive(p)
	}
	lbl("DPI", 160, 420, 40, 22)
	a.hDPI = edit(196, 416, 62, 28, idEditDPI, esNumber)
	lbl("页码范围", 274, 420, 70, 22)
	a.hPages = edit(344, 416, 150, 28, idEditPages, 0)
	a.hLblInfo = lbl("留空=全部，如 1-3,5", 502, 420, 280, 22)
	setFont(a.hLblInfo, a.th.fSmall)

	lbl("JPG 质量", 32, 464, 70, 22)
	a.hQuality = edit(96, 460, 52, 28, idEditQuality, esNumber)
	lbl("PDF 密码", 168, 464, 70, 22)
	a.hPass = edit(240, 460, 150, 28, idEditPass, 0x0080 /*ES_PASSWORD*/)
	lbl("有打开密码才填，没有就留空", 402, 464, 300, 22)

	// 动作
	a.hBtnStart = btn("开始转换", 32, 522, 150, 40, idBtnStart)
	a.hBtnStop = btn("停止", 190, 522, 100, 40, idBtnStop)
	a.hBtnOpenOut = btn("打开输出文件夹", 298, 522, 150, 40, idBtnOpenOut)
	a.hBtnUpd = btn("检查更新", 456, 522, 120, 40, idBtnUpd)
	a.hBtnQuit = btn("退出程序", 584, 522, 110, 40, idBtnQuit)

	// 进度条
	a.hProgress = a.mk("msctls_progress32", "", pbsSmooth|wsTabStop, 32, 574, 750, 12, 0)
	pSendMessageW.Call(uintptr(a.hProgress), pbmSetRange32, 0, 100)

	// 日志
	card(16, 600, 798, 170)
	section("运行日志", 32, 612, 200, 20)
	a.hLog = a.mk("EDIT", "", wsVScroll|esMultiline|esReadOnly|esAutoVScroll|wsTabStop, 32, 634, 750, 122, idLog)
	a.editFrames = append(a.editFrames, rect{a.px(30), a.px(632), a.px(784), a.px(758)})
	pSendMessageW.Call(uintptr(a.hLog), emSetMargins, 0x0003, uintptr(10|(8<<16)))
	setFont(a.hLog, a.th.fLog)

	a.hStatus = lbl("状态：就绪", 16, 776, 780, 22)
	setFont(a.hStatus, a.th.fSmall)

	// 填设置（输出目录故意不预填）
	idx := 0
	if strings.EqualFold(a.cfg.Format, "jpg") {
		idx = 1
	}
	pSendMessageW.Call(uintptr(a.hFmt), cbSetCurSel, uintptr(idx), 0)
	a.setText(a.hDPI, fmt.Sprintf("%d", a.cfg.DPI))
	a.setText(a.hPages, a.cfg.Pages)
	a.setText(a.hQuality, fmt.Sprintf("%d", a.cfg.Quality))
	a.setCheck(a.hChkSub, a.cfg.SubFolder)
	a.setCheck(a.hChkSkip, a.cfg.SkipExists)
	a.setCheck(a.hChkOpen, a.cfg.OpenWhenDone)

	a.enable(a.hBtnStop, false)
	pSetTimer.Call(uintptr(a.hwnd), 1, 300, 0)

	a.logLine("程序启动，版本 v" + appVersion)
	if tool, err := findTool("pdftoppm"); err != nil {
		a.logLine("⚠ 找不到转换组件：" + err.Error())
		a.logLine("请确认程序目录下带有 poppler 文件夹（重新解压完整安装包）")
	} else {
		a.logLine("转换组件：" + tool)
	}
	if a.cfg.AutoCheckUpd {
		a.goSafe("自动检查更新", func() {
			time.Sleep(1500 * time.Millisecond)
			a.doCheckUpdate(defaultUpdateURL, false)
		})
	}
}

func (a *guiApp) getText(h syscall.Handle) string {
	buf := make([]uint16, 8192)
	n, _, _ := pGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.TrimSpace(syscall.UTF16ToString(buf[:int(n)]))
}

func (a *guiApp) setText(h syscall.Handle, s string) {
	p, _ := syscall.UTF16PtrFromString(s)
	pSetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}

func (a *guiApp) setCheck(h syscall.Handle, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	pSendMessageW.Call(uintptr(h), bmSetCheck, v, 0)
}

func (a *guiApp) getCheck(h syscall.Handle) bool {
	r, _, _ := pSendMessageW.Call(uintptr(h), bmGetCheck, 0, 0)
	return r == 1
}

func (a *guiApp) enable(h syscall.Handle, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	pEnableWindow.Call(uintptr(h), v)
}

func (a *guiApp) logLine(s string) {
	line := time.Now().Format("15:04:05") + "  " + s
	select {
	case a.events <- uiEvent{kind: evLog, text: line}:
	default:
	}
}

func (a *guiApp) setStatus(s string) { a.setText(a.hStatus, "状态："+s) }

func (a *guiApp) drain() {
	defer func() {
		if r := recover(); r != nil {
			trace("定时器处理异常：%v", r)
		}
	}()
	for {
		select {
		case e := <-a.events:
			switch e.kind {
			case evLog:
				a.appendLog(e.text)
				appendLogFile(e.text)
			case evFound:
				a.promptUpdate(e.man)
			case evProgress:
				a.donePages = e.done
				if e.total > 0 {
					pos := e.done * 100 / e.total
					if pos > 100 {
						pos = 100
					}
					pSendMessageW.Call(uintptr(a.hProgress), pbmSetPos, uintptr(pos), 0)
					a.setStatus(fmt.Sprintf("转换中 … %d/%d 页", e.done, e.total))
				} else {
					a.setStatus(fmt.Sprintf("转换中 … 已完成 %d 页", e.done))
				}
			case evDone:
				a.busy = false
				a.enable(a.hBtnStart, true)
				a.enable(a.hBtnStop, false)
				a.enable(a.hBtnAdd, true)
				a.enable(a.hBtnAddDir, true)
				a.enable(a.hBtnClear, true)
				if e.ok {
					a.setStatus(e.text)
					pSendMessageW.Call(uintptr(a.hProgress), pbmSetPos, 100, 0)
				} else {
					a.setStatus(e.text)
				}
			}
		default:
			return
		}
	}
}

func (a *guiApp) appendLog(line string) {
	a.logBuf = append(a.logBuf, line)
	trimmed := false
	if len(a.logBuf) > 400 {
		a.logBuf = append([]string(nil), a.logBuf[len(a.logBuf)-400:]...)
		trimmed = true
	}
	if a.hLog == 0 {
		return
	}
	atBottom := a.logAtBottom()
	firstVisible := a.logFirstVisible()
	if trimmed || a.logLines == 0 {
		a.setText(a.hLog, strings.Join(a.logBuf, "\r\n"))
		if !trimmed && firstVisible > 0 {
			pSendMessageW.Call(uintptr(a.hLog), emLineScroll, 0, uintptr(firstVisible))
		}
	} else {
		txt := line
		if len(a.logBuf) > 1 {
			txt = "\r\n" + line
		}
		p, _ := syscall.UTF16PtrFromString(txt)
		pSendMessageW.Call(uintptr(a.hLog), emSetSel, 0x7FFFFFFF, 0x7FFFFFFF)
		pSendMessageW.Call(uintptr(a.hLog), emReplaceSel, uintptr(unsafe.Pointer(p)), 0)
		runtime.KeepAlive(p)
	}
	a.logLines = len(a.logBuf)
	if atBottom {
		pSendMessageW.Call(uintptr(a.hLog), emSetSel, 0x7FFFFFFF, 0x7FFFFFFF)
		pSendMessageW.Call(uintptr(a.hLog), emScrollCaret, 0, 0)
	}
}

func (a *guiApp) logFirstVisible() int32 {
	r, _, _ := pSendMessageW.Call(uintptr(a.hLog), emGetFirstVisibleLine, 0, 0)
	return int32(r)
}

func (a *guiApp) logAtBottom() bool {
	total, _, _ := pSendMessageW.Call(uintptr(a.hLog), emGetLineCount, 0, 0)
	if int32(total) <= 0 {
		return true
	}
	first := a.logFirstVisible()
	visible := int32(a.px(122) / a.px(20))
	if visible < 3 {
		visible = 3
	}
	return first+visible >= int32(total)-1
}

func (a *guiApp) onDropFiles(hDrop uintptr) {
	defer pDragFinish.Call(hDrop)
	n, _, _ := pDragQueryFileW.Call(hDrop, ^uintptr(0), 0, 0)
	var paths []string
	for i := uintptr(0); i < n; i++ {
		need, _, _ := pDragQueryFileW.Call(hDrop, i, 0, 0)
		if need == 0 {
			continue
		}
		buf := make([]uint16, need+2)
		pDragQueryFileW.Call(hDrop, i, uintptr(unsafe.Pointer(&buf[0])), need+1)
		paths = append(paths, syscall.UTF16ToString(buf))
	}
	a.addPaths(paths, true)
}

// addPaths 展开文件与文件夹（文件夹递归），去重后加入列表。
func (a *guiApp) addPaths(paths []string, verbose bool) {
	if a.busy {
		return
	}
	found, err := collectPDFs(paths, true)
	if err != nil {
		a.logLine("读取失败：" + err.Error())
		return
	}
	known := map[string]bool{}
	for _, f := range a.files {
		known[strings.ToLower(f)] = true
	}
	added := 0
	for _, f := range found {
		k := strings.ToLower(f)
		if known[k] {
			continue
		}
		known[k] = true
		a.files = append(a.files, f)
		added++
	}
	a.refreshList()
	if verbose {
		if added == 0 {
			a.logLine("没有新增 PDF（可能已在列表里，或拖进来的不是 PDF）")
		} else {
			a.logLine(fmt.Sprintf("已加入 %d 个 PDF，共 %d 个", added, len(a.files)))
		}
	}
}

func (a *guiApp) refreshList() {
	if a.hList == 0 {
		return
	}
	if len(a.files) == 0 {
		a.setText(a.hList, "")
		return
	}
	lines := make([]string, 0, len(a.files))
	for i, f := range a.files {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, f))
	}
	a.setText(a.hList, strings.Join(lines, "\r\n"))
}

func (a *guiApp) onCommand(id int32) {
	switch id {
	case idBtnAdd:
		files := browseForFiles(a.hwnd, "选择要转换的 PDF（可多选）",
			"PDF 文件 (*.pdf)\x00*.pdf\x00所有文件 (*.*)\x00*.*\x00")
		a.addPaths(files, true)
	case idBtnAddDir:
		if d := browseForFolder(a.hwnd, "选择包含 PDF 的文件夹（会包含子文件夹）"); d != "" {
			a.addPaths([]string{d}, true)
		}
	case idBtnClear:
		if a.busy {
			return
		}
		a.files = nil
		a.refreshList()
		a.logLine("已清空文件列表")
	case idBtnBrowseOut:
		if d := browseForFolder(a.hwnd, "选择图片输出文件夹"); d != "" {
			a.setText(a.hOut, d)
			a.rememberOut(d)
		}
	case idBtnRecent:
		a.showRecentMenu()
	case idChkSub, idChkSkip, idChkOpen, idCmbFmt:
		a.saveCfg()
	case idBtnStart:
		a.startConvert()
	case idBtnStop:
		if a.busy && a.cancel != nil {
			a.cancel.stop()
			a.logLine("正在停止…")
			a.setStatus("正在停止…")
		}
	case idBtnOpenOut:
		dir := a.getText(a.hOut)
		if dir == "" {
			msgBox(a.hwnd, "还没有选择输出文件夹。", "打不开", mbOK|mbIconInfo)
			return
		}
		a.shellOpen(dir)
	case idBtnUpd:
		a.enable(a.hBtnUpd, false)
		a.goSafe("检查更新", func() {
			a.doCheckUpdate(defaultUpdateURL, true)
			select {
			case a.events <- uiEvent{kind: evLog, text: ""}:
			default:
			}
			a.enable(a.hBtnUpd, true)
		})
	case idBtnQuit:
		a.quitApp()
	}
}

func (a *guiApp) rememberOut(dir string) {
	a.cfg.rememberOut(dir)
	_ = a.cfg.save()
}

func (a *guiApp) showRecentMenu() {
	if len(a.cfg.RecentOut) == 0 {
		msgBox(a.hwnd, "还没有历史输出文件夹。\r\n先点“浏览…”选一次，以后就能在这里快选。", "最近", mbOK|mbIconInfo)
		return
	}
	hMenu, _, _ := pCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	for i, d := range a.cfg.RecentOut {
		if i >= 10 {
			break
		}
		t, _ := syscall.UTF16PtrFromString(fmt.Sprintf("%d. %s", i+1, d))
		pAppendMenuW.Call(hMenu, 0, uintptr(i+1), uintptr(unsafe.Pointer(t)))
		runtime.KeepAlive(t)
	}
	var pt struct{ x, y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(uintptr(a.hwnd))
	cmd, _, _ := pTrackPopupMenu.Call(hMenu,
		uintptr(0x0002|0x0100|0x0080), uintptr(pt.x), uintptr(pt.y), 0, uintptr(a.hwnd), 0)
	pDestroyMenu.Call(hMenu)
	if cmd >= 1 && int(cmd) <= len(a.cfg.RecentOut) {
		d := a.cfg.RecentOut[cmd-1]
		a.setText(a.hOut, d)
		a.rememberOut(d)
	}
}

func (a *guiApp) saveCfg() {
	a.cfg.OutDir = a.getText(a.hOut)
	a.cfg.Format = "png"
	if sel, _, _ := pSendMessageW.Call(uintptr(a.hFmt), cbGetCurSel, 0, 0); int32(sel) == 1 {
		a.cfg.Format = "jpg"
	}
	if n := atoiSafe(a.getText(a.hDPI)); n >= 36 && n <= 1200 {
		a.cfg.DPI = n
	} else {
		a.cfg.DPI = 200
		a.setText(a.hDPI, "200")
	}
	if n := atoiSafe(a.getText(a.hQuality)); n >= 30 && n <= 100 {
		a.cfg.Quality = n
	} else {
		a.cfg.Quality = 92
		a.setText(a.hQuality, "92")
	}
	a.cfg.Pages = a.getText(a.hPages)
	a.cfg.SubFolder = a.getCheck(a.hChkSub)
	a.cfg.SkipExists = a.getCheck(a.hChkSkip)
	a.cfg.OpenWhenDone = a.getCheck(a.hChkOpen)
	a.cfg.AutoCheckUpd = true
	_ = a.cfg.save()
}

func (a *guiApp) startConvert() {
	if a.busy {
		return
	}
	if len(a.files) == 0 {
		msgBox(a.hwnd, "请先添加要转换的 PDF。\r\n\r\n可以点“添加 PDF…”，或直接把文件/文件夹拖进窗口。",
			"还差一步", mbOK|mbIconInfo)
		return
	}
	outDir := a.getText(a.hOut)
	if outDir == "" {
		msgBox(a.hwnd, "请先选择输出文件夹。", "还差一步", mbOK|mbIconInfo)
		return
	}
	if st, err := os.Stat(outDir); err != nil || !st.IsDir() {
		msgBox(a.hwnd, "输出文件夹不存在，请重新选择：\r\n"+outDir, "打不开", mbOK|mbIconWarn)
		return
	}
	a.saveCfg()
	a.rememberOut(outDir)

	opt := Options{
		DPI:         a.cfg.DPI,
		Format:      a.cfg.Format,
		Quality:     a.cfg.Quality,
		Pages:       a.cfg.Pages,
		Password:    a.getText(a.hPass),
		SubFolder:   a.cfg.SubFolder,
		SkipExists:  a.cfg.SkipExists,
		Gray:        a.cfg.Gray,
		NamePattern: a.cfg.NamePattern,
	}
	if _, err := parsePageRanges(opt.Pages); err != nil {
		msgBox(a.hwnd, err.Error(), "页码范围不对", mbOK|mbIconWarn)
		return
	}

	a.busy = true
	a.cancel = newCancelToken()
	a.donePages = 0
	a.runStart = time.Now()
	pSendMessageW.Call(uintptr(a.hProgress), pbmSetPos, 0, 0)
	a.enable(a.hBtnStart, false)
	a.enable(a.hBtnStop, true)
	a.enable(a.hBtnAdd, false)
	a.enable(a.hBtnAddDir, false)
	a.enable(a.hBtnClear, false)

	files := append([]string(nil), a.files...)
	a.logLine(fmt.Sprintf("开始转换：%d 个 PDF → %s（%d DPI，%s）", len(files), outDir, opt.DPI, strings.ToUpper(opt.Format)))

	a.goSafe("转换", func() { a.runConvert(files, outDir, opt) })
}

func (a *guiApp) runConvert(files []string, outDir string, opt Options) {
	tool, err := findTool("pdftoppm")
	if err != nil {
		a.logLine("✗ " + err.Error())
		a.finish(false, "缺少转换组件")
		return
	}
	_ = tool

	// 先统计总页数，进度条才有意义
	total := 0
	info, _ := findTool("pdfinfo")
	if info != "" {
		for _, f := range files {
			if n, e := pdfPageCount(info, f, opt.Password); e == nil {
				total += n
			}
		}
	}
	if total == 0 {
		total = len(files)
	}
	a.mu.Lock()
	a.totalPages = total
	a.mu.Unlock()
	a.events <- uiEvent{kind: evProgress, done: 0, total: total}

	done := 0
	filesOK, filesFail := 0, 0
	images := 0
	for _, pdf := range files {
		if a.cancel.stopped() {
			break
		}
		a.logLine("→ " + filepath.Base(pdf))
		rep := &convReporter{
			onLog: a.logLine,
			onAdd: func(int) {
				a.mu.Lock()
				done++
				d := done
				a.mu.Unlock()
				select {
				case a.events <- uiEvent{kind: evProgress, done: d, total: total}:
				default:
				}
			},
		}
		res := convertOne(pdf, outDir, opt, a.cancel, rep)
		if res.Err != nil {
			if res.Err == errStopped {
				break
			}
			filesFail++
			a.logLine(fmt.Sprintf("✗ %s：%v", filepath.Base(pdf), res.Err))
			continue
		}
		filesOK++
		images += res.Written
		if res.Skipped > 0 {
			a.logLine(fmt.Sprintf("✓ %s：写出 %d 页，跳过 %d 页", filepath.Base(pdf), res.Written, res.Skipped))
		} else {
			a.logLine(fmt.Sprintf("✓ %s：写出 %d 页", filepath.Base(pdf), res.Written))
		}
	}

	stopped := a.cancel.stopped()
	secs := int(time.Since(a.runStart).Seconds())
	var msg string
	if stopped {
		msg = fmt.Sprintf("已停止（%d 个文件，%d 张图片）", filesOK, images)
		a.logLine("已停止。已完成的图片保留在输出目录。")
	} else {
		msg = fmt.Sprintf("完成：%d 个文件，%d 张图片，用时 %d 秒", filesOK, images, secs)
		a.logLine(fmt.Sprintf("全部完成：成功 %d，失败 %d，共 %d 张图片，用时 %d 秒", filesOK, filesFail, images, secs))
		if filesFail > 0 {
			msg += fmt.Sprintf("，失败 %d 个", filesFail)
		}
	}

	a.finish(!stopped, msg)
	if !stopped && filesOK > 0 && a.cfg.OpenWhenDone {
		a.shellOpen(outDir)
	}
	if !stopped && len(files) > 0 {
		_ = a.cfg.save()
	}
}

func (a *guiApp) finish(ok bool, msg string) {
	select {
	case a.events <- uiEvent{kind: evDone, ok: ok, text: msg}:
	default:
	}
}

// ---------- 更新 ----------

func (a *guiApp) doCheckUpdate(urlStr string, manual bool) {
	a.logLine("正在检查更新…")
	man, err := fetchManifest(urlStr)
	if err != nil {
		a.logLine("检查更新失败：" + err.Error())
		if manual {
			msgBox(a.hwnd, "检查更新失败：\n\n"+err.Error(), "检查更新", mbOK|mbIconWarn)
		}
		return
	}
	if compareVersion(man.Version, appVersion) <= 0 {
		a.logLine("已是最新版本 v" + appVersion)
		if manual {
			msgBox(a.hwnd, "当前已是最新版本 v"+appVersion+"。", "检查更新", mbOK|mbIconInfo)
		}
		return
	}
	a.logLine("发现新版本 v" + man.Version)
	select {
	case a.events <- uiEvent{kind: evFound, man: man}:
	default:
	}
}

func (a *guiApp) promptUpdate(man *Manifest) {
	notes := strings.TrimSpace(man.Notes)
	if notes == "" {
		notes = "（无更新说明）"
	}
	text := fmt.Sprintf("发现新版本：v%s\n当前版本：v%s\n\n更新内容：\n%s\n\n是否现在下载并自动更新？\n（更新时程序会短暂重启）",
		man.Version, appVersion, notes)
	if msgBox(a.hwnd, text, "发现新版本", mbYesNo|mbIconQuest) != idYes {
		a.logLine("已跳过本次更新")
		return
	}
	a.goSafe("下载安装更新", func() {
		a.logLine("开始下载更新包…")
		path, err := downloadPackage(man)
		if err != nil {
			a.logLine("下载失败：" + err.Error())
			return
		}
		a.logLine("下载完成，正在解压并替换…（程序会短暂重启）")
		if err := applyUpdate(path, man); err != nil {
			a.logLine("更新失败：" + err.Error())
		}
	})
}

// ---------- Win32 辅助 ----------

func msgBox(owner syscall.Handle, text, caption string, flags uint32) int32 {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	r, _, _ := pMessageBoxW.Call(uintptr(owner), uintptr(unsafe.Pointer(t)),
		uintptr(unsafe.Pointer(c)), uintptr(flags))
	runtime.KeepAlive(t)
	runtime.KeepAlive(c)
	return int32(r)
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         syscall.Handle
	hInstance         syscall.Handle
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

func utf16z(s string) []uint16 {
	r := utf16.Encode([]rune(s))
	return append(r, 0)
}

const (
	ofnReadOnly       = 0x00000001
	ofnAllowMulti     = 0x00000200
	ofnPathMustExist  = 0x00000800
	ofnFileMustExist  = 0x00001000
	ofnExplorer       = 0x00080000
	ofnHideReadOnly   = 0x00000004
)

// browseForFiles 支持多选。返回选中的完整路径列表。
func browseForFiles(owner syscall.Handle, title, filter string) []string {
	fbuf := utf16z(filter)
	fileBuf := make([]uint16, 65536)
	titleP, _ := syscall.UTF16PtrFromString(title)
	ofn := openFileName{
		hwndOwner:    owner,
		lpstrFilter:  &fbuf[0],
		nFilterIndex: 1,
		lpstrFile:    &fileBuf[0],
		nMaxFile:     uint32(len(fileBuf)),
		lpstrTitle:   titleP,
		Flags:        ofnFileMustExist | ofnPathMustExist | ofnExplorer | ofnHideReadOnly | ofnAllowMulti,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	runtime.KeepAlive(fbuf)
	runtime.KeepAlive(fileBuf)
	runtime.KeepAlive(titleP)
	if r == 0 {
		return nil
	}
	// 多选时：目录\0文件1\0文件2\0\0；单选时就是一个完整路径
	var tokens []string
	i := 0
	for i < len(fileBuf) && fileBuf[i] != 0 {
		j := i
		for j < len(fileBuf) && fileBuf[j] != 0 {
			j++
		}
		tokens = append(tokens, syscall.UTF16ToString(fileBuf[i:j]))
		i = j + 1
	}
	if len(tokens) == 0 {
		return nil
	}
	if len(tokens) == 1 {
		return []string{tokens[0]}
	}
	dir := tokens[0]
	var out []string
	for _, name := range tokens[1:] {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

func browseForFolder(owner syscall.Handle, title string) string {
	display := make([]uint16, 260)
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	bi := browseInfo{
		hwndOwner:      owner,
		pszDisplayName: &display[0],
		lpszTitle:      titlePtr,
		ulFlags:        bifReturnOnlyFSDirs | bifEditBox | bifNewDialogStyle,
	}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	runtime.KeepAlive(titlePtr)
	runtime.KeepAlive(display)
	if pidl == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(pidl)
	buf := make([]uint16, 1024)
	ok, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	if ok == 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(buf))
}

func (a *guiApp) shellOpen(p string) {
	if strings.TrimSpace(p) == "" {
		return
	}
	op, _ := syscall.UTF16PtrFromString("open")
	arg, _ := syscall.UTF16PtrFromString(p)
	pShellExecuteW.Call(uintptr(a.hwnd), uintptr(unsafe.Pointer(op)),
		uintptr(unsafe.Pointer(arg)), 0, 0, 1)
	runtime.KeepAlive(op)
	runtime.KeepAlive(arg)
}

// ---------- 托盘 ----------

const (
	nidSize        = 976
	nidOffHWnd     = 8
	nidOffUID      = 16
	nidOffUFlags   = 20
	nidOffCallback = 24
	nidOffHIcon    = 32
	nidOffSzTip    = 40
	nidOffSzInfo   = 304
	nidOffSzTitle  = 820
	nidOffInfoFlag = 948

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010
	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
)

func putU32(b []byte, off int, v uint32)  { binary.LittleEndian.PutUint32(b[off:], v) }
func putPtr(b []byte, off int, v uintptr) { binary.LittleEndian.PutUint64(b[off:], uint64(v)) }

func putW(b []byte, off, maxW int, s string) {
	u := utf16.Encode([]rune(s))
	if len(u) > maxW-1 {
		u = u[:maxW-1]
	}
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[off+i*2:], uint16(c))
	}
}

func (a *guiApp) trayData(withInfo bool) []byte {
	b := make([]byte, nidSize)
	putU32(b, 0, nidSize)
	putPtr(b, nidOffHWnd, uintptr(a.hwnd))
	putU32(b, nidOffUID, 1)
	flags := uint32(nifMessage | nifIcon | nifTip)
	if withInfo {
		flags |= nifInfo
	}
	putU32(b, nidOffUFlags, flags)
	putU32(b, nidOffCallback, wmTray)
	putPtr(b, nidOffHIcon, uintptr(trayIcon))
	putW(b, nidOffSzTip, 128, "PDF 转图片助手")
	if withInfo {
		putW(b, nidOffSzTitle, 64, "PDF 转图片助手")
		putW(b, nidOffSzInfo, 256, "已最小化到托盘。双击图标可打开，右键可退出。")
		putU32(b, nidOffInfoFlag, 1)
	}
	return b
}

func (a *guiApp) addTray() bool {
	if a.trayAdded {
		return true
	}
	b := a.trayData(false)
	r, _, err := pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(b)
	if r == 0 {
		trace("托盘图标创建失败：%v", err)
		return false
	}
	a.trayAdded = true
	return true
}

func (a *guiApp) removeTray() {
	if !a.trayAdded {
		return
	}
	b := a.trayData(false)
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(b)
	a.trayAdded = false
}

func (a *guiApp) hideToTray() {
	if !a.addTray() {
		pShowWindow.Call(uintptr(a.hwnd), swMinimize)
		a.logLine("无法创建托盘图标，已最小化到任务栏")
		return
	}
	pShowWindow.Call(uintptr(a.hwnd), swHide)
	b := a.trayData(true)
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(b)
}

func (a *guiApp) showFromTray() {
	pShowWindow.Call(uintptr(a.hwnd), swShowNormal)
	pSetForegroundWindow.Call(uintptr(a.hwnd))
}

func (a *guiApp) onTray(lParam uintptr) {
	switch uint32(lParam) & 0xFFFF {
	case 0x0202, 0x0203:
		a.showFromTray()
	case 0x0205, 0x007B:
		a.showTrayMenu()
	}
}

func (a *guiApp) showTrayMenu() {
	hMenu, _, _ := pCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	items := []struct {
		id   uintptr
		text string
	}{{1, "打开主窗口"}, {2, "打开输出文件夹"}, {3, "退出"}}
	for _, it := range items {
		t, _ := syscall.UTF16PtrFromString(it.text)
		pAppendMenuW.Call(hMenu, 0, it.id, uintptr(unsafe.Pointer(t)))
		runtime.KeepAlive(t)
	}
	var pt struct{ x, y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(uintptr(a.hwnd))
	cmd, _, _ := pTrackPopupMenu.Call(hMenu,
		uintptr(0x0002|0x0100|0x0080), uintptr(pt.x), uintptr(pt.y), 0, uintptr(a.hwnd), 0)
	pDestroyMenu.Call(hMenu)
	switch cmd {
	case 1:
		a.showFromTray()
	case 2:
		a.shellOpen(a.getText(a.hOut))
	case 3:
		a.quitApp()
	}
}

func (a *guiApp) quitApp() {
	a.reallyQuit = true
	if a.busy && a.cancel != nil {
		a.cancel.stop()
	}
	a.saveCfg()
	pDestroyWindow.Call(uintptr(a.hwnd))
}

func (a *guiApp) onClose() {
	if a.reallyQuit {
		pDestroyWindow.Call(uintptr(a.hwnd))
		return
	}
	if a.busy {
		if msgBox(a.hwnd, "正在转换中，退出会中断任务。\r\n\r\n确定要退出吗？", "还在转换",
			mbYesNo|mbIconQuest) != idYes {
			return
		}
		a.quitApp()
		return
	}
	a.hideToTray()
}

var _ = exec.Command
var _ = pGetDlgItem
var _ = pSetWindowPos
var _ = pUpdateWindowProc
