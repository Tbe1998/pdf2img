//go:build windows

package main

// 界面美化：品牌头部、卡片阴影、步骤徽标、圆角自绘按钮 / 复选框 / 进度条。
// 配色：靛蓝主色 + 冷白卡片。

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// ---- 配色（Win32 COLORREF = 0x00BBGGRR）----
const (
	clrBg       uint32 = 0x00F7F5F5 // #F5F7F7 窗口底
	clrCard     uint32 = 0x00FFFFFF // 卡片
	clrBorder   uint32 = 0x00EFE7E5 // #E5E7EF 卡片/控件描边
	clrInk      uint32 = 0x002B1A17 // #171A2B 正文
	clrMuted    uint32 = 0x00887470 // #707488 次要文字
	clrAccent   uint32 = 0x00E5464F // #4F46E5 主色
	clrAccentD  uint32 = 0x00CA3843 // #4338CA 主色按下
	clrSoft     uint32 = 0x00FEF0EE // #EEF0FE 主色浅底
	clrHover    uint32 = 0x00FCF5F4 // #F4F5FC 悬停
	clrPress    uint32 = 0x00FAEBEA // #EAEBFA 按下
	clrDisable  uint32 = 0x00FCF8F7 // #F7F8FC 禁用底
	clrDisInk   uint32 = 0x00C0AEA8 // #A8AEC0 禁用字
	clrDanger   uint32 = 0x002B39C0 // #C0392B 危险文字
	clrDangBd   uint32 = 0x00C4C9EB // #EBC9C4 危险描边
	clrShadow   uint32 = 0x00F6EFF0 // #F0EFF6 卡片投影
	clrLogBg    uint32 = 0x00FEFBFA // #FAFBFE 日志底
	clrTrack    uint32 = 0x00F7EFF0 // #F0EFF7 进度槽
	clrHairline uint32 = 0x00F4ECEB // #EBECF4 分隔线
	clrBoxBd    uint32 = 0x00DFCEC9 // #C9CEDF 复选框描边
)

const (
	wmPaint     = 0x000F
	wmEraseBg   = 0x0014
	wmDrawItem  = 0x002B
	odsSelected = 0x0001
	odsDisabled = 0x0004

	dtLeft      = 0x00000000
	dtCenter    = 0x00000001
	dtRight     = 0x00000002
	dtVCenter   = 0x00000004
	dtSingle    = 0x00000020
	dtEndEllip  = 0x00008000
	transparent = 1
	opaqueBk    = 2
)

type point struct{ x, y int32 }

var (
	gdi = syscall.NewLazyDLL("gdi32.dll")

	pCreateSolidBrush = gdi.NewProc("CreateSolidBrush")
	pCreatePen        = gdi.NewProc("CreatePen")
	pSelectObject     = gdi.NewProc("SelectObject")
	pRoundRect        = gdi.NewProc("RoundRect")
	pPolyline         = gdi.NewProc("Polyline")
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

// chip 是分区标题左侧的步骤圆点。
type chip struct {
	r rect
	n int32
}

type theme struct {
	brBg, brCard, brWhite, brHover, brPress, brDisable uintptr
	brAccent, brAccentD, brDanger                      uintptr
	brSoft, brShadow, brLog, brTrack, brHair, brBox    uintptr
	penBorder, penAccent, penDanger, penNull, penBox   uintptr
	penCheck                                           uintptr
	fTitle, fLabel, fBody, fSmall, fLog, fChip         syscall.Handle
	hoverBtn                                           syscall.Handle
}

func (a *guiApp) initTheme() {
	t := &a.th
	mkBrush := func(c uint32) uintptr {
		h, _, _ := pCreateSolidBrush.Call(uintptr(c))
		return h
	}
	mkPen := func(w int32, c uint32) uintptr {
		h, _, _ := pCreatePen.Call(0, uintptr(w), uintptr(c))
		return h
	}
	mkPenNull := func() uintptr {
		h, _, _ := pCreatePen.Call(5 /*PS_NULL*/, 0, 0) // 无边框，避免圆角处多一圈线
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
	t.brSoft = mkBrush(clrSoft)
	t.brShadow = mkBrush(clrShadow)
	t.brLog = mkBrush(clrLogBg)
	t.brTrack = mkBrush(clrTrack)
	t.brHair = mkBrush(clrHairline)
	t.brBox = mkBrush(clrBoxBd)
	t.penBorder = mkPen(1, clrBorder)
	t.penAccent = mkPen(1, clrAccent)
	t.penDanger = mkPen(1, clrDangBd)
	t.penBox = mkPen(1, clrBoxBd)
	t.penCheck = mkPen(2, 0x00FFFFFF)
	t.penNull = mkPenNull()

	t.fTitle = a.makeFont("Microsoft YaHei UI", 20, 700)
	t.fLabel = a.makeFont("Microsoft YaHei UI", 14, 600)
	t.fBody = a.makeFont("Microsoft YaHei UI", 14, 400)
	t.fSmall = a.makeFont("Microsoft YaHei UI", 12, 400)
	t.fLog = a.makeFont("Consolas", 12, 400)
	t.fChip = a.makeFont("Segoe UI", 11, 700)
}

func (a *guiApp) makeFont(face string, px int32, weight int32) syscall.Handle {
	name, _ := syscall.UTF16PtrFromString(face)
	h, _, _ := pCreateFontW.Call(
		uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(name)),
	)
	runtime.KeepAlive(name)
	return syscall.Handle(h)
}

func (a *guiApp) drawText(hdc uintptr, font syscall.Handle, color uint32, s string, r rect, flags uintptr) {
	old, _, _ := pSelectObject.Call(hdc, uintptr(font))
	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, uintptr(color))
	p, _ := syscall.UTF16PtrFromString(s)
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0),
		uintptr(unsafe.Pointer(&r)), flags)
	runtime.KeepAlive(p)
	pSelectObject.Call(hdc, old)
}

// paintWindow 画背景、品牌头部、卡片底板、步骤徽标、进度条和状态点。
func (a *guiApp) paintWindow(hdc uintptr) {
	t := &a.th
	var rc rect
	pGetClientRect.Call(uintptr(a.hwnd), uintptr(unsafe.Pointer(&rc)))
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), t.brBg)

	// —— 头部：白底 + 底部细线 ——
	head := rect{0, 0, rc.right, a.px(72)}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&head)), t.brCard)
	line := rect{0, a.px(71), rc.right, a.px(72)}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&line)), t.brHair)

	// 图标底托
	a.roundRectR(hdc, rect{a.px(20), a.px(14), a.px(64), a.px(58)}, t.brSoft, t.penNull, a.px(24))
	if trayIcon != 0 {
		pDrawIconEx.Call(hdc, uintptr(a.px(27)), uintptr(a.px(21)), uintptr(trayIcon),
			uintptr(a.px(30)), uintptr(a.px(30)), 0, 0, 0x0003)
	}

	a.drawText(hdc, t.fTitle, clrInk, "PDF 转图片助手",
		rect{a.px(78), a.px(12), a.px(600), a.px(42)}, dtLeft|dtVCenter|dtSingle)
	a.drawText(hdc, t.fSmall, clrMuted, "把 PDF 每一页批量导出为 PNG / JPG",
		rect{a.px(79), a.px(40), a.px(600), a.px(62)}, dtLeft|dtVCenter|dtSingle)

	// 版本徽标
	pill := rect{a.px(736), a.px(24), a.px(810), a.px(48)}
	a.roundRectR(hdc, pill, t.brSoft, t.penNull, a.px(24))
	a.drawText(hdc, t.fSmall, clrAccent, "v"+appVersion, pill, dtCenter|dtVCenter|dtSingle)

	// —— 卡片：先画 2px 投影，再画白底 ——
	for _, c := range a.cards {
		sh := rect{c.left + a.px(1), c.top + a.px(2), c.right + a.px(1), c.bottom + a.px(2)}
		a.roundRectR(hdc, sh, t.brShadow, t.penNull, a.px(28))
		a.roundRectR(hdc, c, t.brCard, t.penBorder, a.px(28))
	}
	for _, f := range a.editFrames {
		// 输入框保持纯白底，只描边框，悬停/输入区仍由控件自身绘制
		a.roundRectR(hdc, f, t.brCard, t.penBorder, a.px(16))
	}

	// —— 步骤徽标 ——
	for _, c := range a.chips {
		a.roundRectR(hdc, c.r, t.brAccent, t.penNull, a.px(18))
		a.drawText(hdc, t.fChip, 0x00FFFFFF, fmt.Sprintf("%d", c.n), c.r,
			dtCenter|dtVCenter|dtSingle)
	}

	// —— 进度条 ——
	track := rect{a.px(32), a.px(578), a.px(668), a.px(586)}
	a.roundRectR(hdc, track, t.brTrack, t.penNull, a.px(8))
	if a.progressVal > 0 {
		w := a.px(32) + (a.px(636))*a.progressVal/100
		if w < a.px(40) {
			w = a.px(40)
		}
		fill := rect{a.px(32), a.px(578), w, a.px(586)}
		a.roundRectR(hdc, fill, t.brAccent, t.penNull, a.px(8))
	}
	a.drawText(hdc, t.fSmall, clrMuted, fmt.Sprintf("%d%%", a.progressVal),
		rect{a.px(680), a.px(570), a.px(782), a.px(594)}, dtRight|dtVCenter|dtSingle)

	// —— 底部状态点 ——
	dot := rect{a.px(18), a.px(784), a.px(24), a.px(790)}
	a.roundRectR(hdc, dot, t.brAccent, t.penNull, a.px(6))
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
	a.roundRectR(hdc, r, brush, pen, a.px(28))
}

func (a *guiApp) roundRectR(hdc uintptr, r rect, brush, pen uintptr, radius int32) {
	ob, _, _ := pSelectObject.Call(hdc, brush)
	op, _, _ := pSelectObject.Call(hdc, pen)
	pRoundRect.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom),
		uintptr(radius), uintptr(radius))
	pSelectObject.Call(hdc, op)
	pSelectObject.Call(hdc, ob)
}

// drawButton 自绘按钮：主 / 次 / 幽灵 / 危险四种样式 + 悬停 / 按下 / 禁用。
func (a *guiApp) drawButton(dis *drawItem) {
	t := &a.th
	hdc := uintptr(dis.hDC)
	r := dis.rcItem

	if dis.hwndItem == a.hChkSub || dis.hwndItem == a.hChkSkip || dis.hwndItem == a.hChkOpen {
		a.drawCheck(dis)
		return
	}

	primary := dis.hwndItem == a.hBtnStart
	danger := dis.hwndItem == a.hBtnQuit
	ghost := dis.hwndItem == a.hBtnUpd || dis.hwndItem == a.hBtnQuit || dis.hwndItem == a.hBtnClear
	hover := dis.hwndItem == t.hoverBtn
	disabled := dis.itemState&odsDisabled != 0
	pressed := dis.itemState&odsSelected != 0

	var brush, penH uintptr
	var txt uint32
	switch {
	case disabled:
		brush, penH, txt = t.brDisable, t.penBorder, clrDisInk
	case primary:
		brush, penH, txt = t.brAccent, t.penAccent, 0x00FFFFFF
		if pressed || hover {
			brush = t.brAccentD
		}
	case danger:
		brush, txt = t.brCard, clrDanger
		penH = t.penNull
		if pressed {
			brush = t.brPress
		} else if hover {
			brush = t.brHover
		}
	case ghost:
		brush, penH, txt = t.brCard, t.penNull, clrMuted
		if pressed {
			brush = t.brPress
		} else if hover {
			brush, txt = t.brHover, clrAccent
		}
	default:
		brush, penH, txt = t.brCard, t.penBorder, clrInk
		if pressed {
			brush = t.brPress
		} else if hover {
			brush = t.brHover
		}
	}

	// 先用所在区域底色擦掉整块矩形，否则圆角四角会露出灰框
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), a.bgBrushAt(r))
	if ghost {
		if hover || pressed {
			a.roundRectR(hdc, r, brush, t.penNull, a.px(18))
		}
	} else {
		a.roundRectR(hdc, r, brush, penH, a.px(18))
	}

	font := t.fBody
	if primary {
		font = t.fLabel
	}
	buf := make([]uint16, 128)
	n, _, _ := pGetWindowTextW.Call(uintptr(dis.hwndItem), uintptr(unsafe.Pointer(&buf[0])), 128)
	old, _, _ := pSelectObject.Call(hdc, uintptr(font))
	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, uintptr(txt))
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&buf[0])), n,
		uintptr(unsafe.Pointer(&r)), dtCenter|dtVCenter|dtSingle|dtEndEllip)
	pSelectObject.Call(hdc, old)
}

// drawCheck 自绘复选框：18px 圆角方块 + 勾。
func (a *guiApp) drawCheck(dis *drawItem) {
	t := &a.th
	hdc := uintptr(dis.hDC)
	r := dis.rcItem
	disabled := dis.itemState&odsDisabled != 0
	hover := dis.hwndItem == t.hoverBtn

	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), a.bgBrushAt(r))

	side := a.px(18)
	top := r.top + (r.bottom-r.top-side)/2
	box := rect{r.left + a.px(2), top, r.left + a.px(2) + side, top + side}

	on := a.getCheck(dis.hwndItem)
	brush, pen := t.brCard, t.penBox
	txt := clrInk
	switch {
	case disabled:
		brush, pen, txt = t.brDisable, t.penBorder, clrDisInk
	case on:
		brush, pen, txt = t.brAccent, t.penAccent, clrInk
	case hover:
		pen = t.penAccent
	}
	a.roundRectR(hdc, box, brush, pen, a.px(10))

	if on {
		pts := []point{
			{box.left + a.px(4), box.top + side/2},
			{box.left + side/2 - a.px(1), box.bottom - a.px(5)},
			{box.right - a.px(4), box.top + a.px(5)},
		}
		a.polylineWhite(hdc, pts)
	}

	buf := make([]uint16, 256)
	n, _, _ := pGetWindowTextW.Call(uintptr(dis.hwndItem), uintptr(unsafe.Pointer(&buf[0])), 256)
	txtR := rect{box.right + a.px(9), r.top, r.right, r.bottom}
	old, _, _ := pSelectObject.Call(hdc, uintptr(t.fBody))
	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, uintptr(txt))
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&buf[0])), n,
		uintptr(unsafe.Pointer(&txtR)), dtLeft|dtVCenter|dtSingle|dtEndEllip)
	pSelectObject.Call(hdc, old)
}

// polylineWhite 用 2px 白笔画勾。
func (a *guiApp) polylineWhite(hdc uintptr, pts []point) {
	old, _, _ := pSelectObject.Call(hdc, a.th.penCheck)
	pPolyline.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	pSelectObject.Call(hdc, old)
}

func (a *guiApp) ctlColorStatic(hdc uintptr, hwnd syscall.Handle) uintptr {
	t := &a.th
	// 多行只读 EDIT 必须用不透明底色，否则滚动时旧文字会叠影
	if hwnd == a.hLog || hwnd == a.hList {
		bg := clrCard
		br := t.brCard
		if hwnd == a.hLog {
			bg = clrLogBg
			br = t.brLog
		}
		pSetBkMode.Call(hdc, opaqueBk)
		pSetBkColor.Call(hdc, uintptr(bg))
		pSetTextColor.Call(hdc, uintptr(clrInk))
		return br
	}
	pSetBkMode.Call(hdc, transparent)
	if hwnd == a.hStatus || hwnd == a.hLblInfo {
		pSetTextColor.Call(hdc, uintptr(clrMuted))
		if hwnd == a.hStatus {
			return t.brBg
		}
		return t.brCard
	}
	for _, h := range a.hintLabels {
		if h == hwnd {
			pSetTextColor.Call(hdc, uintptr(clrMuted))
			return t.brCard
		}
	}
	for _, h := range a.sectionLabels {
		if h == hwnd {
			pSetTextColor.Call(hdc, uintptr(clrInk))
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
