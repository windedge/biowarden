package hello

import (
	"fmt"
	"syscall"
)

// Foreground bookkeeping for the consent prompt. A native messaging host is a
// background process: Windows refuses SetForegroundWindow from a process the
// user is not interacting with, and a window the process never activated
// cannot raise the prompt it owns. The owner is created topmost (see
// hiddenwindow_windows.go) so the prompt is above other windows, and for the
// duration of the prompt the foreground is lent to the owner so the prompt can
// take keyboard focus, then handed back to whatever had it.
var (
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procSetLastError             = kernel32.NewProc("SetLastError")
)

const (
	gwlExStyle       = ^uintptr(19) // GWL_EXSTYLE (-20) as an unsigned index
	swHide           = 0
	swShowNoActivate = 4
)

// threadID returns the id of the calling thread.
func threadID() uintptr {
	r, _, _ := procGetCurrentThreadID.Call()
	return r
}

// lendOutcome is everything the debug log needs about one borrow of the
// foreground. It holds handles, thread ids and error codes only.
type lendOutcome struct {
	self       uintptr
	prevFg     uintptr
	prevThread uintptr
	shownByUs  bool
	attached   bool
	setFg      bool
	setFgErr   syscall.Errno
}

func (o lendOutcome) String() string {
	return fmt.Sprintf("selfThread=%d prevFg=%#x prevThread=%d shownByUs=%t attached=%t setFg=%t setFgErr=%d",
		o.self, o.prevFg, o.prevThread, o.shownByUs, o.attached, o.setFg, o.setFgErr)
}

// lendForeground gives the owner window the foreground whenever another window
// holds it. A hidden window cannot become the foreground window, so the owner
// is shown without activation first (it is zero-sized and topmost, so nothing
// is drawn). Windows only lets the foreground process call SetForegroundWindow,
// so the calling thread's input queue is attached to the foreground thread's
// for the call and detached immediately after; this is the classic way to pass
// the foreground lock without injecting input.
func lendForeground(hwnd uintptr) lendOutcome {
	out := lendOutcome{self: threadID()}
	out.prevFg, _, _ = procGetForegroundWindow.Call()
	if out.prevFg != 0 {
		out.prevThread, _, _ = procGetWindowThreadProcessID.Call(out.prevFg, 0)
	}
	if out.prevFg == hwnd {
		out.setFg = true
		return out
	}
	// ShowWindow returns the previous visibility: zero means it was hidden, so
	// this call is the one that made it visible and the one that must undo it.
	if r, _, _ := procShowWindow.Call(hwnd, swShowNoActivate); r == 0 {
		out.shownByUs = true
	}
	if out.prevThread != 0 && out.prevThread != out.self {
		if r, _, _ := procAttachThreadInput.Call(out.self, out.prevThread, 1); r != 0 {
			out.attached = true
		}
	}
	procSetLastError.Call(0)
	r, _, err := procSetForegroundWindow.Call(hwnd)
	out.setFg = r != 0
	if errno, ok := err.(syscall.Errno); ok {
		out.setFgErr = errno
	}
	if out.attached {
		procAttachThreadInput.Call(out.self, out.prevThread, 0)
	}
	return out
}

// returnForeground hands the foreground back to the window that held it before
// the prompt and hides the owner again if the lend had shown it. Both are
// best-effort: a refusal here must not affect the verification result.
func returnForeground(prev, owner uintptr, wasShown bool) (ok bool, errno syscall.Errno) {
	if prev != 0 {
		procSetLastError.Call(0)
		r, _, err := procSetForegroundWindow.Call(prev)
		ok = r != 0
		if e, isErr := err.(syscall.Errno); isErr {
			errno = e
		}
	}
	if wasShown {
		procShowWindow.Call(owner, swHide)
	}
	return ok, errno
}
