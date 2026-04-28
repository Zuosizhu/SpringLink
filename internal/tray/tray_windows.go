//go:build windows

package tray

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"sync"
	"syscall"
	"unsafe"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	user32             = syscall.NewLazyDLL("user32.dll")
	gdi32              = syscall.NewLazyDLL("gdi32.dll")
	shellNotifyIcon    = shell32.NewProc("Shell_NotifyIconW")
	createWindowEx     = user32.NewProc("CreateWindowExW")
	defWindowProc      = user32.NewProc("DefWindowProcW")
	destroyWindow      = user32.NewProc("DestroyWindow")
	registerClass      = user32.NewProc("RegisterClassExW")
	loadIcon           = user32.NewProc("LoadIconW")
	getMessage         = user32.NewProc("GetMessageW")
	translateMessage   = user32.NewProc("TranslateMessage")
	dispatchMessage    = user32.NewProc("DispatchMessageW")
	postQuitMessage    = user32.NewProc("PostQuitMessage")
	createPopupMenu    = user32.NewProc("CreatePopupMenu")
	appendMenu         = user32.NewProc("AppendMenuW")
	trackPopupMenu     = user32.NewProc("TrackPopupMenu")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	destroyMenu        = user32.NewProc("DestroyMenu")
	getCursorPos       = user32.NewProc("GetCursorPos")
	postMessage        = user32.NewProc("PostMessageW")
	createIconIndirect = user32.NewProc("CreateIconIndirect")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	createBitmap       = gdi32.NewProc("CreateBitmap")
	deleteObject       = gdi32.NewProc("DeleteObject")
	findWindowW        = user32.NewProc("FindWindowW")
	sendMessageW       = user32.NewProc("SendMessageW")
)

type point struct {
	x, y int32
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	bmiHeader bitmapInfoHeader
	bmiColors [1]uint32
}

type iconInfo struct {
	fIcon    int32
	xHotspot int32
	yHotspot int32
	hbmMask  uintptr
	hbmColor uintptr
}

const (
	WM_APP          = 0x8000
	WM_TRAYICON     = WM_APP + 1
	WM_TRAY_QUIT    = WM_APP + 2
	WM_COMMAND      = 0x0111
	NIM_ADD         = 0
	NIM_DELETE      = 2
	NIF_MESSAGE     = 1
	NIF_ICON        = 2
	NIF_TIP         = 4
	WS_EX_TOOLWINDOW = 0x00000080
	WS_POPUP        = 0x80000000
	MF_STRING       = 0
	TPM_BOTTOMALIGN = 0x0020
	TPM_LEFTALIGN   = 0x0000
	WM_SETICON      = 0x0080
	ICON_SMALL      = 0
	ICON_BIG        = 1
	ID_SHOW         = 1000
	ID_QUIT         = 1001
)

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

var (
	inst      uintptr
	wndHwnd   uintptr
	showFn    func()
	quitFn    func()
	done      = make(chan struct{})
	startedMu sync.Mutex
	iconData  []byte
)

func SetIcon(data []byte) {
	iconData = data
}

func classNamePtr() *uint16 {
	p, _ := syscall.UTF16PtrFromString("SpringLinkTray")
	return p
}

func init() {
	h, err := syscall.LoadLibrary("kernel32.dll")
	if err == nil {
		p, err := syscall.GetProcAddress(h, "GetModuleHandleW")
		if err == nil && p != 0 {
			ret, _, _ := syscall.Syscall(p, 1, 0, 0, 0)
			inst = ret
		}
	}
}

func SetWindowIcon() {
	hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("SpringLink"))))
	if hwnd == 0 {
		return
	}
	hicon := createHICON()
	if hicon == 0 {
		return
	}
	sendMessageW.Call(hwnd, WM_SETICON, ICON_BIG, hicon)
	sendMessageW.Call(hwnd, WM_SETICON, ICON_SMALL, hicon)
}

func createHICON() uintptr {
	if iconData == nil || len(iconData) == 0 {
		hicon, _, _ := loadIcon.Call(0, 32514)
		return hicon
	}
	img, err := png.Decode(bytes.NewReader(iconData))
	if err != nil {
		hicon, _, _ := loadIcon.Call(0, 32514)
		return hicon
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	bmi := bitmapInfo{}
	bmi.bmiHeader.biSize = uint32(unsafe.Sizeof(bitmapInfoHeader{}))
	bmi.bmiHeader.biWidth = int32(w)
	bmi.bmiHeader.biHeight = -int32(h)
	bmi.bmiHeader.biPlanes = 1
	bmi.bmiHeader.biBitCount = 32
	bmi.bmiHeader.biCompression = 0

	var bits unsafe.Pointer
	hbmColor, _, _ := createDIBSection.Call(0, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmColor == 0 {
		hicon, _, _ := loadIcon.Call(0, 32514)
		return hicon
	}

	src := rgba.Pix
	rowBytes := w * 4
	dst := (*[1 << 30]byte)(unsafe.Pointer(bits))[:h*rowBytes]
	for y := 0; y < h; y++ {
		srcRow := src[y*rowBytes : (y+1)*rowBytes]
		dstRow := dst[y*rowBytes : (y+1)*rowBytes]
		for x := 0; x < w; x++ {
			si := x * 4
			dstRow[si+0] = srcRow[si+2] // B
			dstRow[si+1] = srcRow[si+1] // G
			dstRow[si+2] = srcRow[si+0] // R
			dstRow[si+3] = srcRow[si+3] // A
		}
	}

	maskRowBytes := ((w + 31) / 32) * 4
	maskTotalBytes := h * maskRowBytes
	maskBuf := make([]byte, maskTotalBytes)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := y*rowBytes + x*4
			alpha := src[si+3]
			if alpha < 128 {
				idx := y*maskRowBytes + x/8
				maskBuf[idx] |= 1 << byte(7-x%8)
			}
		}
	}

	hbmMask, _, _ := createBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&maskBuf[0])))
	if hbmMask == 0 {
		deleteObject.Call(hbmColor)
		hicon, _, _ := loadIcon.Call(0, 32514)
		return hicon
	}

	ii := iconInfo{
		fIcon:    1,
		hbmMask:  hbmMask,
		hbmColor: hbmColor,
	}
	hicon, _, _ := createIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	deleteObject.Call(hbmColor)
	deleteObject.Call(hbmMask)
	if hicon == 0 {
		hicon, _, _ = loadIcon.Call(0, 32514)
	}
	return hicon
}

func Start(onShow, onQuit func()) error {
	startedMu.Lock()
	defer startedMu.Unlock()
	showFn = onShow
	quitFn = onQuit

	hicon := createHICON()

	clsName := classNamePtr()
	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(trayWndProc),
		hInstance:     inst,
		hIcon:         hicon,
		lpszClassName: clsName,
	}

	ret, _, err := registerClass.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 {
		return err
	}

	hwnd, _, err := createWindowEx.Call(
		WS_EX_TOOLWINDOW, uintptr(unsafe.Pointer(clsName)), 0, WS_POPUP,
		0, 0, 0, 0, 0, 0, inst, 0,
	)
	if hwnd == 0 {
		return err
	}
	wndHwnd = hwnd

	nid := notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             hwnd,
		uID:              1,
		uFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		uCallbackMessage: WM_TRAYICON,
		hIcon:            hicon,
	}
	tip, _ := syscall.UTF16FromString("SpringLink")
	copy(nid.szTip[:], tip)

	ret, _, err = shellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		destroyWindow.Call(hwnd)
		return err
	}

	var m msg
	for {
		ret, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}

	nid2 := notifyIconData{cbSize: uint32(unsafe.Sizeof(notifyIconData{})), hWnd: hwnd, uID: 1}
	shellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid2)))
	destroyWindow.Call(hwnd)
	startedMu.Lock()
	wndHwnd = 0
	startedMu.Unlock()
	close(done)
	return nil
}

func Stop() {
	startedMu.Lock()
	if wndHwnd == 0 {
		startedMu.Unlock()
		return
	}
	wndHwnd = 0
	startedMu.Unlock()
	postQuitMessage.Call(0)
	<-done
}

func trayWndProc(hwnd uintptr, message uint32, wParam uintptr, lParam uintptr) uintptr {
	switch message {
	case WM_TRAYICON:
		switch lParam {
		case 0x202:
			if showFn != nil {
				showFn()
			}
		case 0x205:
			showMenu(hwnd)
		}
		return 0

	case WM_COMMAND:
		switch wParam & 0xFFFF {
		case ID_SHOW:
			if showFn != nil {
				showFn()
			}
		case ID_QUIT:
			postMessage.Call(hwnd, WM_TRAY_QUIT, 0, 0)
		}
		return 0

	case WM_TRAY_QUIT:
		if quitFn != nil {
			quitFn()
		}
		return 0
	}

	ret, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func showMenu(hwnd uintptr) {
	hMenu, _, _ := createPopupMenu.Call()
	if hMenu == 0 {
		return
	}
	appendMenu.Call(hMenu, MF_STRING, ID_SHOW, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("显示窗口"))))
	appendMenu.Call(hMenu, MF_STRING, ID_QUIT, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("退出"))))

	setForegroundWindow.Call(hwnd)
	var pt point
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	trackPopupMenu.Call(hMenu, TPM_BOTTOMALIGN|TPM_LEFTALIGN, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
	destroyMenu.Call(hMenu)
}
