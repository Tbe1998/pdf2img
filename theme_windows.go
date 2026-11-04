//go:build windows

package main

// 界面美化：品牌头部、卡片阴影、步骤徽标、圆角自绘按钮 / 复选框 / 进度条。
// 圆角统一走“超采样 + HALFTONE 缩小”的抗锯齿路径（GDI 的 RoundRect 本身没有抗锯齿，
// 直接用会在高分屏上出现明显台阶）。

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

	// 超采样倍率：小控件 3 倍、大卡片 2 倍（再高收益不明显，反而拖慢重绘）
	aaFactorSmall = 3
	aaFactorBig   = 2
)

var (
	gdi = syscall.NewLazyDLL("gdi32.dll")

	pCreateSolidBrush = gdi.NewProc("CreateSolidBrush")
	pCreatePen        = gdi.NewProc("CreatePen")
	pSelectObject     = gdi.NewProc("SelectObject")
	pDeleteObject     = gdi.NewProc("DeleteObject")
	pRoundRect        = gdi.NewProc("RoundRect")
	pSetBkMode        = gdi.NewProc("SetBkMode")
	pSetTextColor     = gdi.NewProc("SetTextColor")
	pSetBkColor       = gdi.NewProc("SetBkColor")

	pCreateCompatibleDC     = gdi.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi.NewProc("CreateCompatibleBitmap")
	pDeleteDC               = gdi.NewProc("DeleteDC")
	pStretchBlt             = gdi.NewProc("StretchBlt")
	pSetStretchBltMode      = gdi.NewProc("SetStretchBltMode")

	pGetDC     = user32.NewProc("GetDC")
	pReleaseDC = user32.NewProc("ReleaseDC")

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
	t.penNull = mkPen(1, clrCard)

	// 字号也要按 DPI 缩放，否则 125%/150% 下会显得字小、控件大
	t.fTitle = a.makeFont("Microsoft YaHei UI", a.px(20), 700)
	t.fLabel = a.makeFont("Microsoft YaHei UI", a.px(14), 600)
	t.fBody = a.makeFont("Microsoft YaHei UI", a.px(14), 400)
	t.fSmall = a.makeFont("Microsoft YaHei UI", a.px(12), 400)
	t.fLog = a.makeFont("Consolas", a.px(12), 400)
	t.fChip = a.makeFont("Segoe UI", a.px(11), 700)
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

// bgColorAt 返回矩形中心所在的底色，抗锯齿边缘要按底色混合。
func (a *guiApp) bgColorAt(r rect) uint32 {
	cx := (r.left + r.right) / 2
	cy := (r.top + r.bottom) / 2
	for _, c := range a.cards {
		if cx >= c.left && cx <= c.right && cy >= c.top && cy <= c.bottom {
			return clrCard
		}
	}
	return clrBg
}

// fillRR 画抗锯齿圆角矩形：先把形状放大 N 倍画进位图，再用 HALFTONE 缩小，
// 缩小时的平均采样就是抗锯齿。fill==0 表示只描边，stroke==0 表示只填充。
func (a *guiApp) fillRR(hdc uintptr, r rect, fill, stroke uint32, dia int32, strokeW int32, bg uint32) {
	w := r.right - r.left
	h := r.bottom - r.top
	if w <= 0 || h <= 0 || dia < 0 {
		return
	}
	if bg == 0 {
		bg = a.bgColorAt(r)
	}
	if strokeW < 0 {
		strokeW = 0
	}

	f := int32(aaFactorSmall)
	if w > 320 || h > 220 {
		f = aaFactorBig
	}
	mw, mh := w*f, h*f

	mdc, _, _ := pCreateCompatibleDC.Call(hdc)
	if mdc == 0 {
		a.fillRRDirect(hdc, r, fill, stroke, dia, strokeW)
		return
	}
	// 必须用屏幕 DC 建兼容位图：若用按钮的临时 DC，可能拿到 1bpp 单色位图
	screen, _, _ := pGetDC.Call(uintptr(a.hwnd))
	bmp, _, _ := pCreateCompatibleBitmap.Call(screen, uintptr(mw), uintptr(mh))
	pReleaseDC.Call(uintptr(a.hwnd), screen)
	if bmp == 0 {
		pDeleteDC.Call(mdc)
		a.fillRRDirect(hdc, r, fill, stroke, dia, strokeW)
		return
	}
	oldBmp, _, _ := pSelectObject.Call(mdc, bmp)

	// 先铺底色（缩小后边缘像素与底色混合，不会出现白边/黑边）
	bgBr, _, _ := pCreateSolidBrush.Call(uintptr(bg))
	mr := rect{0, 0, mw, mh}
	pFillRect.Call(mdc, uintptr(unsafe.Pointer(&mr)), bgBr)
	pDeleteObject.Call(bgBr)

	var fb, pb uintptr
	if fill != 0 {
		fb, _, _ = pCreateSolidBrush.Call(uintptr(fill))
	}
	penW := strokeW * f
	if penW < 1 {
		penW = 1
	}
	if stroke != 0 && strokeW > 0 {
		pb, _, _ = pCreatePen.Call(0, uintptr(penW), uintptr(stroke))
	} else {
		pb, _, _ = pCreatePen.Call(5 /*PS_NULL*/, 0, 0)
	}
	ob, _, _ := pSelectObject.Call(mdc, fb)
	op, _, _ := pSelectObject.Call(mdc, pb)
	pRoundRect.Call(mdc, 0, 0, uintptr(mw), uintptr(mh), uintptr(dia*f), uintptr(dia*f))
	pSelectObject.Call(mdc, op)
	pSelectObject.Call(mdc, ob)
	if fb != 0 {
		pDeleteObject.Call(fb)
	}
	if pb != 0 {
		pDeleteObject.Call(pb)
	}

	pSetStretchBltMode.Call(hdc, 4 /*HALFTONE*/)
	pStretchBlt.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(w), uintptr(h),
		mdc, 0, 0, uintptr(mw), uintptr(mh), 0x00CC0020 /*SRCCOPY*/)

	pSelectObject.Call(mdc, oldBmp)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mdc)
}

// fillRRDirect 是抗锯齿位图创建失败时的退路（老 GDI 圆角，会有锯齿但不至于不显示）。
func (a *guiApp) fillRRDirect(hdc uintptr, r rect, fill, stroke uint32, dia, strokeW int32) {
	var fb, pb uintptr
	if fill != 0 {
		fb, _, _ = pCreateSolidBrush.Call(uintptr(fill))
	}
	if stroke != 0 && strokeW > 0 {
		pb, _, _ = pCreatePen.Call(0, uintptr(strokeW), uintptr(stroke))
	} else {
		pb, _, _ = pCreatePen.Call(5, 0, 0)
	}
	ob, _, _ := pSelectObject.Call(hdc, fb)
	op, _, _ := pSelectObject.Call(hdc, pb)
	pRoundRect.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom),
		uintptr(dia), uintptr(dia))
	pSelectObject.Call(hdc, op)
	pSelectObject.Call(hdc, ob)
	if fb != 0 {
		pDeleteObject.Call(fb)
	}
	if pb != 0 {
		pDeleteObject.Call(pb)
	}
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
	a.fillRR(hdc, rect{a.px(20), a.px(14), a.px(64), a.px(58)}, clrSoft, 0, a.px(24), 0, clrCard)
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
	a.fillRR(hdc, pill, clrSoft, 0, a.px(24), 0, clrCard)
	a.drawText(hdc, t.fSmall, clrAccent, "v"+appVersion, pill, dtCenter|dtVCenter|dtSingle)

	// —— 卡片：先画 2px 投影，再画白底 ——
	for _, c := range a.cards {
		sh := rect{c.left + a.px(1), c.top + a.px(2), c.right + a.px(1), c.bottom + a.px(2)}
		a.fillRR(hdc, sh, clrShadow, 0, a.px(28), 0, 0)
		a.fillRR(hdc, c, clrCard, clrBorder, a.px(28), 1, 0)
	}
	for _, f := range a.editFrames {
		a.fillRR(hdc, f, clrCard, clrBorder, a.px(16), 1, 0)
	}

	// —— 步骤徽标 ——
	for _, c := range a.chips {
		a.fillRR(hdc, c.r, clrAccent, 0, a.px(18), 0, 0)
		a.drawText(hdc, t.fChip, 0x00FFFFFF, fmt.Sprintf("%d", c.n), c.r,
			dtCenter|dtVCenter|dtSingle)
	}

	// —— 进度条 ——
	track := rect{a.px(32), a.px(578), a.px(668), a.px(586)}
	a.fillRR(hdc, track, clrTrack, 0, a.px(8), 0, 0)
	if a.progressVal > 0 {
		w := a.px(32) + (a.px(636))*a.progressVal/100
		if w < a.px(40) {
			w = a.px(40)
		}
		fill := rect{a.px(32), a.px(578), w, a.px(586)}
		a.fillRR(hdc, fill, clrAccent, 0, a.px(8), 0, clrTrack)
	}
	a.drawText(hdc, t.fSmall, clrMuted, fmt.Sprintf("%d%%", a.progressVal),
		rect{a.px(680), a.px(570), a.px(782), a.px(594)}, dtRight|dtVCenter|dtSingle)

	// —— 底部状态点 ——
	dot := rect{a.px(18), a.px(784), a.px(24), a.px(790)}
	a.fillRR(hdc, dot, clrAccent, 0, a.px(6), 0, 0)
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

// drawButton 自绘按钮：主 / 次 / 幽灵 / 危险四种样式 + 悬停 / 按下 / 禁用。
func (a *guiApp) drawButton(dis *drawItem) {
	t := &a.th
	hdc := uintptr(dis.hDC)
	r := dis.rcItem

	primary := dis.hwndItem == a.hBtnStart
	danger := dis.hwndItem == a.hBtnQuit
	ghost := dis.hwndItem == a.hBtnUpd || dis.hwndItem == a.hBtnQuit || dis.hwndItem == a.hBtnClear
	hover := dis.hwndItem == t.hoverBtn
	disabled := dis.itemState&odsDisabled != 0
	pressed := dis.itemState&odsSelected != 0
	bg := a.bgColorAt(r)

	var fill, stroke uint32
	var txt uint32
	switch {
	case disabled:
		fill, stroke, txt = clrDisable, clrBorder, clrDisInk
	case primary:
		fill, stroke, txt = clrAccent, clrAccent, 0x00FFFFFF
		if pressed || hover {
			fill = clrAccentD
		}
	case danger:
		fill, stroke, txt = clrCard, 0, clrDanger
		if pressed {
			fill = clrPress
		} else if hover {
			fill = clrHover
		}
	case ghost:
		fill, stroke, txt = bg, 0, clrMuted
		if pressed {
			fill = clrPress
		} else if hover {
			fill, txt = clrHover, clrAccent
		}
	default:
		fill, stroke, txt = clrCard, clrBorder, clrInk
		if pressed {
			fill = clrPress
		} else if hover {
			fill = clrHover
		}
	}

	// 先用所在区域底色擦掉整块矩形，否则圆角四角会露出灰框
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), a.bgBrushAt(r))
	a.fillRR(hdc, r, fill, stroke, a.px(18), 1, 0)

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
