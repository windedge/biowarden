package hello

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// The Windows Hello consent UI is owned by a window: without one it cannot
// position itself or be dismissed with its host. The owner is a hidden
// top-level window on a dedicated thread that keeps pumping messages, because
// the verifier worker blocks on the async operation and a window whose thread
// stopped pumping would deadlock any synchronous message the consent UI sends
// to its owner.

var (
	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procLoadCursorW           = user32.NewProc("LoadCursorW")
	procIsWindow              = user32.NewProc("IsWindow")
	procGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
)

const (
	helloWindowClass = "BiowardenHelloOwner"
	csHRedraw        = 0x0002
	csVRedraw        = 0x0001
	wmDestroy        = 0x0002
	wmClose          = 0x0010
	idcArrow         = 32512

	spiGetWorkArea = 0x0030
	swpNoSize      = 0x0001
	swpNoZOrder    = 0x0004
	swpNoActivate  = 0x0010
)

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

var (
	ownerOnce  sync.Once
	ownerHWND  uintptr
	ownerErr   error
	ownerReady = make(chan struct{})
)

// promptOwner returns a hidden top-level window the consent prompt can be
// owned by, creating it and its message pump thread on first use.
func promptOwner() (uintptr, error) {
	ownerOnce.Do(func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ownerHWND, ownerErr = createOwnerWindow()
			close(ownerReady)
			if ownerErr != nil {
				return
			}
			pumpMessages()
		}()
	})
	<-ownerReady
	return ownerHWND, ownerErr
}

// closePromptOwner asks the owner window to close, which ends its pump. The
// verifier keeps the window for the process lifetime; this is for callers that
// need a clean shutdown.
func closePromptOwner() {
	if ownerHWND == 0 {
		return
	}
	procPostMessageW.Call(ownerHWND, wmClose, 0, 0)
}

func createOwnerWindow() (uintptr, error) {
	className := syscall.StringToUTF16Ptr(helloWindowClass)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{
		Style:     csHRedraw | csVRedraw,
		WndProc:   syscall.NewCallback(ownerWndProc),
		Instance:  moduleHandle(),
		Cursor:    cursor,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return 0, fmt.Errorf("hello: RegisterClassExW: %v", err)
	}
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)), 0, 0,
		0, 0, 0, 0, 0, 0, moduleHandle(), 0)
	runtime.KeepAlive(className)
	if hwnd == 0 {
		return 0, fmt.Errorf("hello: CreateWindowExW: %v", err)
	}
	centerOwnerWindow(hwnd)
	return hwnd, nil
}

// centerOwnerWindow moves the hidden owner to the centre of the primary work
// area before the pump starts. The consent UI places itself relative to its
// owner, so an owner left at the default top-left drops the prompt in the
// corner; centring happens here so it is already in place when
// RequestVerificationForWindowAsync runs.
func centerOwnerWindow(hwnd uintptr) {
	var work rect
	if r, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0); r == 0 {
		return
	}
	var wr rect
	if r, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr))); r == 0 {
		wr.Right, wr.Bottom = wr.Left, wr.Top
	}
	width := int(wr.Right - wr.Left)
	height := int(wr.Bottom - wr.Top)
	x := int(work.Left) + (int(work.Right-work.Left)-width)/2
	y := int(work.Top) + (int(work.Bottom-work.Top)-height)/2
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
}

func moduleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}

// ownerWndProc only needs to finish WM_CLOSE's default DestroyWindow and end
// the pump on WM_DESTROY; the hidden window draws nothing.
func ownerWndProc(hwnd, m, wp, lp uintptr) uintptr {
	if m == wmDestroy {
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

func pumpMessages() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
