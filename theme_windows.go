//go:build windows

package main

// 界面美化：卡片分区、圆角自绘按钮、顶部品牌区、统一配色。
// 配色：靛蓝主色 + 白卡片，与“日报分拣助手”（深青）区分开。

import (
	"runtime"
	"syscall"
	"unsafe"
)

// ---- 配色（Win32 COLORREF = 0x00BBGGRR）----
const (
	clrBg      uint32 = 0x00FAF6F5 // #F5F6FA 窗口底
	clrCard    uint32 = 0x00FFFFFF // 卡片
	clrBorder  uint32 = 0x00F0E8E6 // #E6E8F0 卡片描边
	clrInk     uint32 = 0x0033231F // #1F2333 正文
	clrMuted   uint32 = 0x0086716B // #6B7186 次要文字
	clrAccent  uint32 = 0x00E5464F // #4F46E5 主色
	clrAccentD uint32 = 0x00C9373F // #3F37C9
	clrHover   uint32 = 0x00FBEEEF // 悬停
	clrPress   uint32 = 0x00F8E2E4
	clrDisable uint32 = 0x00F7F7F9
	clrDisInk  uint32 = 0x00B8B4A7
	clrDanger  uint32 = 0x002B39C0 // #C0392B
	clrDangBd  uint32 = 0x00C5C9E8
)

const (
	wmPaint     = 0x000F
	wmEraseBg   = 0x0014
	wmDrawItem  = 0x002B
	odsSelected = 0x0001
	odsDisabled = 0x0004

	dtLeft      = 0x00000000
	dtCenter    = 0x00000001
	dtVCenter   = 0x00000004
	dtSingle    = 0x00000020
	dtEndEllip  = 0x00008000
	transparent = 1
	opaqueBk    = 2
)

var (
	gdi = syscall.NewLazyDLL("gdi32.dll")

	pCreateSolidBrush = gdi.NewProc("CreateSolidBrush")
	pCreatePen        = gdi.NewProc("CreatePen")
	pSelectObject     = gdi.NewProc("SelectObject")
	pRoundRect        = gdi.NewProc("RoundRect")
	pSetBkMode        = gdi.NewProc("SetBkMode")
	pSetTextColor     = gdi.NewProc("SetTextColor")
	pSetBkColor       = gdi.NewProc("SetBkColor")

	pFillRect      = user32.NewProc("FillRect")
	pDrawTextW     = user32.NewProc("DrawTextW")
	pDrawIconEx    = user32.NewProc("DrawIconEx")
	pInvalidateRct = user32.NewProc("InvalidateRect")
	pGetClientRect = user32.NewProc("GetClientRect")
	pWindowFromPt  = user32.NewProc("WindowFromPoint")
)

type drawItem struct {
	CtlType    uint32
	CtlID      uint32
	itemID     uint32
	itemAction uint32
	itemState  uint32
	hwndItem   syscall.Handle
	hDC        syscall.Handle
	rcItem     rect
	itemData   uintptr
}

type theme struct {
	brBg, brCard, brWhite, brHover, brPress, brDisable uintptr
	brAccent, brAccentD, brDanger                      uintptr
	penBorder, penAccent, penDanger                    uintptr
	fTitle, fLabel, fBody, fSmall, fLog                syscall.Handle
	hoverBtn                                           syscall.Handle
}

func (a *guiApp) initTheme() {
	t := &a.th
	mkBrush := func(c uint32) uintptr {
		h, _, _ := pCreateSolidBrush.Call(uintptr(c))
		return h
	}
	mkPen := func(c uint32) uintptr {
		h, _, _ := pCreatePen.Call(0, 1, uintptr(c))
		return h
	}
	t.brBg = mkBrush(clrBg)
	t.brCard = mkBrush(clrCard)
	t.brWhite = mkBrush(clrCard)
	t.brHover = mkBrush(clrHover)
	t.brPress = mkBrush(clrPress)
	t.brDisable = mkBrush(clrDisable)
	t.brAccent = mkBrush(clrAccent)
	t.brAccentD = mkBrush(clrAccentD)
	t.brDanger = mkBrush(clrDanger)
	t.penBorder = mkPen(clrBorder)
	t.penAccent = mkPen(clrAccent)
	t.penDanger = mkPen(clrDangBd)

	t.fTitle = a.makeFont(21, 700)
	t.fLabel = a.makeFont(14, 600)
	t.fBody = a.makeFont(15, 400)
	t.fSmall = a.makeFont(13, 400)
	t.fLog = a.makeFont(13, 400)
}

func (a *guiApp) makeFont(px int32, weight int32) syscall.Handle {
	face, _ := syscall.UTF16PtrFromString("Microsoft YaHei UI")
	h, _, _ := pCreateFontW.Call(
		uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(face)),
	)
	runtime.KeepAlive(face)
	return syscall.Handle(h)
}

// paintWindow 画背景、品牌头部和卡片底板。
func (a *guiApp) paintWindow(hdc uintptr) {
	t := &a.th
	var rc rect
	pGetClientRect.Call(uintptr(a.hwnd), uintptr(unsafe.Pointer(&rc)))
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), t.brBg)

	icon := trayIcon
	if icon == 0 {
		return
	}
	pDrawIconEx.Call(hdc, uintptr(a.px(24)), uintptr(a.px(18)), uintptr(icon),
		uintptr(a.px(38)), uintptr(a.px(38)), 0, 0, 0x0003)

	old, _, _ := pSelectObject.Call(hdc, uintptr(t.fTitle))
	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, uintptr(clrInk))
	titleR := rect{a.px(76), a.px(14), a.px(620), a.px(44)}
	tt, _ := syscall.UTF16PtrFromString("PDF 转图片助手")
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(tt)), ^uintptr(0),
		uintptr(unsafe.Pointer(&titleR)), dtLeft|dtVCenter|dtSingle)
	runtime.KeepAlive(tt)

	pSelectObject.Call(hdc, uintptr(t.fSmall))
	pSetTextColor.Call(hdc, uintptr(clrMuted))
	subR := rect{a.px(77), a.px(42), a.px(760), a.px(64)}
	sub, _ := syscall.UTF16PtrFromString("把 PDF 每一页批量导出为 PNG / JPG  ·  v" + appVersion)
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(sub)), ^uintptr(0),
		uintptr(unsafe.Pointer(&subR)), dtLeft|dtVCenter|dtSingle)
	runtime.KeepAlive(sub)
	pSelectObject.Call(hdc, old)

	for _, c := range a.cards {
		a.roundRect(hdc, c, t.brCard, t.penBorder)
	}
	for _, f := range a.editFrames {
		a.roundRectR(hdc, f, t.brCard, t.penBorder, a.px(14))
	}
}

func (a *guiApp) bgBrushAt(r rect) uintptr {
	cx := (r.left + r.right) / 2
	cy := (r.top + r.bottom) / 2
	for _, c := range a.cards {
		if cx >= c.left && cx <= c.right && cy >= c.top && cy <= c.bottom {
			return a.th.brCard
		}
	}
	return a.th.brBg
}

func (a *guiApp) roundRect(hdc uintptr, r rect, brush, pen uintptr) {
	a.roundRectR(hdc, r, brush, pen, a.px(24))
}

func (a *guiApp) roundRectR(hdc uintptr, r rect, brush, pen uintptr, radius int32) {
	ob, _, _ := pSelectObject.Call(hdc, brush)
	op, _, _ := pSelectObject.Call(hdc, pen)
	pRoundRect.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom),
		uintptr(radius), uintptr(radius))
	pSelectObject.Call(hdc, op)
	pSelectObject.Call(hdc, ob)
}

// drawButton 自绘按钮：圆角 + 主/次/危险三种样式 + 悬停/按下/禁用。
func (a *guiApp) drawButton(dis *drawItem) {
	t := &a.th
	hdc := uintptr(dis.hDC)
	r := dis.rcItem

	primary := dis.hwndItem == a.hBtnStart
	danger := dis.hwndItem == a.hBtnQuit
	hover := dis.hwndItem == t.hoverBtn
	disabled := dis.itemState&odsDisabled != 0
	pressed := dis.itemState&odsSelected != 0

	var brush, penH uintptr
	var txt uint32
	switch {
	case disabled:
		pSetBkMode.Call(hdc, transparent)
		brush, penH, txt = t.brDisable, t.penBorder, clrDisInk
	case primary:
		brush, penH, txt = t.brAccent, t.penAccent, 0x00FFFFFF
		if pressed || hover {
			brush = t.brAccentD
		}
	case danger:
		if pressed {
			brush = t.brPress
		} else if hover {
			brush = t.brHover
		} else {
			brush = t.brCard
		}
		penH, txt = t.penDanger, clrDanger
	default:
		if pressed {
			brush = t.brPress
		} else if hover {
			brush = t.brHover
		} else {
			brush = t.brCard
		}
		penH, txt = t.penBorder, clrInk
	}

	// 先用所在区域底色擦掉整块矩形，否则圆角四角会露出灰框
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), a.bgBrushAt(r))
	a.roundRectR(hdc, r, brush, penH, a.px(20))

	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, uintptr(txt))
	font := t.fBody
	if primary {
		font = t.fLabel
	}
	old, _, _ := pSelectObject.Call(hdc, uintptr(font))
	buf := make([]uint16, 128)
	n, _, _ := pGetWindowTextW.Call(uintptr(dis.hwndItem), uintptr(unsafe.Pointer(&buf[0])), 128)
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&buf[0])), n,
		uintptr(unsafe.Pointer(&r)), dtCenter|dtVCenter|dtSingle|dtEndEllip)
	pSelectObject.Call(hdc, old)
}

func (a *guiApp) ctlColorStatic(hdc uintptr, hwnd syscall.Handle) uintptr {
	t := &a.th
	// 多行只读 EDIT 必须用不透明底色，否则滚动时旧文字会叠影
	if hwnd == a.hLog || hwnd == a.hList {
		pSetBkMode.Call(hdc, opaqueBk)
		pSetBkColor.Call(hdc, uintptr(clrCard))
		pSetTextColor.Call(hdc, uintptr(clrInk))
		return t.brCard
	}
	pSetBkMode.Call(hdc, transparent)
	if hwnd == a.hStatus || hwnd == a.hLblInfo {
		pSetTextColor.Call(hdc, uintptr(clrMuted))
		if hwnd == a.hStatus {
			return t.brBg
		}
		return t.brCard
	}
	for _, h := range a.sectionLabels {
		if h == hwnd {
			pSetTextColor.Call(hdc, uintptr(clrAccent))
			return t.brCard
		}
	}
	pSetTextColor.Call(hdc, uintptr(clrInk))
	return t.brCard
}
func (a *guiApp) trackHover() {
	var pt struct{ x, y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	h, _, _ := pWindowFromPt.Call(uintptr(unsafe.Pointer(&pt)))
	var found syscall.Handle
	if h != 0 {
		for _, b := range a.allButtons {
			if syscall.Handle(h) == b {
				found = b
				break
			}
		}
	}
	if found != a.th.hoverBtn {
		old := a.th.hoverBtn
		a.th.hoverBtn = found
		if old != 0 {
			pInvalidateRct.Call(uintptr(old), 0, 0)
		}
		if found != 0 {
			pInvalidateRct.Call(uintptr(found), 0, 0)
		}
	}
}
