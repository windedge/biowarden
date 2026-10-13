//go:build windows

package hello

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Windows Hello has no syscall projection, so the WinRT UserConsentVerifier
// ABI is driven by hand: HSTRINGs, activation factories, COM vtables and a
// hand-built completion handler. None of this is reachable from cgo-free Go
// otherwise.

// kernel32 is a KnownDLL, so a bare name is loaded from System32; user32 and
// combase are not on Go's known-system-DLL list and need an absolute path.
var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemDirectoryW = kernel32.NewProc("GetSystemDirectoryW")
)

var (
	combase = systemDLL("combase.dll")
	user32  = systemDLL("user32.dll")
)

func systemDLL(name string) *syscall.LazyDLL {
	buf := make([]uint16, syscall.MAX_PATH)
	n, _, _ := procGetSystemDirectoryW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) >= len(buf) {
		return syscall.NewLazyDLL(name)
	}
	return syscall.NewLazyDLL(syscall.UTF16ToString(buf[:n]) + `\` + name)
}

var (
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoUninitialize         = combase.NewProc("RoUninitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
)

// kernel32 synchronization, used to wait for the async consent operation.
var (
	procCreateEventW        = kernel32.NewProc("CreateEventW")
	procSetEvent            = kernel32.NewProc("SetEvent")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
)

// HRESULTs are signed 32-bit values carried in a uintptr, so comparisons are
// done through uint32 and failure through int32.
const (
	hresultOK           uint32 = 0
	hresultFalse        uint32 = 1
	hresultNoInterface  uint32 = 0x80004002
	rpcChangedMode      uint32 = 0x80010106
	roInitMultithreaded        = 1 // RO_INIT_MULTITHREADED
)

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hresultErr(what string, hr uintptr) error {
	return fmt.Errorf("hello: %s failed (HRESULT %#08x)", what, uint32(hr))
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// guidFromString parses "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx".
func guidFromString(s string) guid {
	hex := func(s string) uint64 {
		var v uint64
		for _, c := range s {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= uint64(c - '0')
			case c >= 'a' && c <= 'f':
				v |= uint64(c-'a') + 10
			case c >= 'A' && c <= 'F':
				v |= uint64(c-'A') + 10
			}
		}
		return v
	}
	g := guid{Data1: uint32(hex(s[0:8])), Data2: uint16(hex(s[9:13])), Data3: uint16(hex(s[14:18]))}
	g.Data4[0], g.Data4[1] = byte(hex(s[19:21])), byte(hex(s[21:23]))
	for i := 0; i < 6; i++ {
		g.Data4[2+i] = byte(hex(s[24+2*i : 26+2*i]))
	}
	return g
}

// Interface identities the consent ABI depends on. Values are copied from the
// Windows SDK: um/UserConsentVerifierInterop.h, winrt/asyncinfo.h and
// winrt/windows.security.credentials.ui.h (the parameterized specialization
// UUIDs that the winmd carries).
var (
	iidIUnknown       = guidFromString("00000000-0000-0000-C000-000000000046")
	iidIAgileObject   = guidFromString("94EA2B94-E9CC-49E0-C0FF-EE64CA8F5B90")
	iidConsentInterop = guidFromString("39E050C3-4E74-441A-8DC0-B81104DF949C")
	iidConsentStatics = guidFromString("AF4F3F91-564C-4DDC-B8B5-973447627C65")
	iidConsentAsync   = guidFromString("FD596FFD-2318-558F-9DBE-D21DF43764A5")
	iidConsentHandler = guidFromString("0CFFC6C9-4C2B-5CD4-B38C-7B8DF3FF5AFB")
	// AsyncOperationCompletedHandler<UserConsentVerifierAvailability>: the
	// availability operation QIs its handler for this specialization, so the
	// same object cannot serve both unless it advertises the matching IID.
	iidAvailabilityHandler = guidFromString("28988174-ACE2-5C15-A0DF-580A26D94294")
)

// native turns an address of memory not managed by Go into a pointer without
// tripping go vet's uintptr-to-pointer check.
func native(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

// comCall calls method i of the COM interface obj. Pointers converted to
// uintptr in the call stay valid until it returns (go:uintptrescapes).
//
//go:uintptrescapes
func comCall(obj uintptr, i int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(native(obj))
	fn := *(*uintptr)(unsafe.Add(native(vtbl), i*int(unsafe.Sizeof(uintptr(0)))))
	var a [8]uintptr
	a[0] = obj
	n := copy(a[1:], args)
	r, _, _ := syscall.SyscallN(fn, a[:n+1]...)
	return r
}

func addRef(obj uintptr) {
	if obj != 0 {
		comCall(obj, 1)
	}
}

func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

// queryInterface returns obj's implementation of iid, or 0.
func queryInterface(obj uintptr, iid *guid) uintptr {
	if obj == 0 {
		return 0
	}
	var out uintptr
	if failed(comCall(obj, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))) {
		return 0
	}
	return out
}

// newHString creates an HSTRING. The caller deletes it with deleteHString.
// HSTRINGs carry their own length rather than a NUL terminator, so embedded
// NULs would survive too. An empty string returns 0, which WinRT treats as a
// valid empty HSTRING.
func newHString(s string) (uintptr, error) {
	units := utf16.Encode([]rune(s))
	var h uintptr
	var ptr *uint16
	if len(units) > 0 {
		ptr = &units[0]
	}
	hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(ptr)), uintptr(len(units)), uintptr(unsafe.Pointer(&h)))
	runtime.KeepAlive(units)
	if failed(hr) {
		return 0, hresultErr("WindowsCreateString", hr)
	}
	return h, nil
}

func deleteHString(h uintptr) {
	if h != 0 {
		procWindowsDeleteString.Call(h)
	}
}

// roInitialize initializes the WinRT apartment for the calling thread and
// returns a function balancing it. Initialization is per thread, so the caller
// must keep its OS thread pinned until the returned function runs.
func roInitialize() (func(), error) {
	hr, _, _ := procRoInitialize.Call(roInitMultithreaded)
	switch uint32(hr) {
	case hresultOK:
		return func() { procRoUninitialize.Call() }, nil
	case hresultFalse, rpcChangedMode:
		// S_FALSE: an earlier call on this thread already initialized WinRT and
		// owns the matching uninitialize. RPC_E_CHANGED_MODE: the thread lives
		// in another apartment and RoUninitialize would unbalance it.
		return func() {}, nil
	default:
		return func() {}, hresultErr("RoInitialize", hr)
	}
}

// activationFactory activates the WinRT runtime class and asks for one of its
// interfaces. The class-name HSTRING is only borrowed by the call.
func activationFactory(class string, iid *guid) (uintptr, error) {
	name, err := newHString(class)
	if err != nil {
		return 0, err
	}
	defer deleteHString(name)
	var factory uintptr
	hr, _, _ := procRoGetActivationFactory.Call(name,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&factory)))
	if failed(hr) {
		return 0, hresultErr("RoGetActivationFactory("+class+")", hr)
	}
	return factory, nil
}

// A handler is the single-method COM object passed to IAsyncOperation's
// put_Completed. All handlers share one vtable; each object routes Invoke to
// its Go function and stays reachable through the handlers map until COM drops
// its last reference. This is the mygo/com.go pattern, widened to include the
// completion method.
type handler struct {
	vtbl *[4]uintptr
	refs int32
	iid  guid // the operation-specific completed-handler interface to answer QI for
	fn   func(a, b uintptr)
}

var (
	handlerVtbl [4]uintptr
	handlersMu  sync.Mutex
	handlers    = map[uintptr]*handler{}
)

func init() {
	handlerVtbl = [4]uintptr{
		syscall.NewCallback(handlerQueryInterface),
		syscall.NewCallback(func(this uintptr) uintptr { return handlerRef(this, 1) }),
		syscall.NewCallback(func(this uintptr) uintptr { return handlerRef(this, -1) }),
		syscall.NewCallback(func(this, a, b uintptr) uintptr {
			handlersMu.Lock()
			h := handlers[this]
			handlersMu.Unlock()
			if h != nil {
				h.fn(a, b)
			}
			return uintptr(hresultOK)
		}),
	}
}

func handlerQueryInterface(this, riid, out uintptr) uintptr {
	if out == 0 {
		return uintptr(hresultNoInterface)
	}
	iid := *(*guid)(native(riid))
	handlersMu.Lock()
	h := handlers[this]
	handlersMu.Unlock()
	// Advertising IAgileObject is a promise the handler is free-threaded; it
	// only sets an event, so the WinRT thread pool may call Invoke directly.
	// IInspectable is deliberately not advertised: this object has no
	// GetIids/GetRuntimeClassName/GetTrustLevel slots.
	if iid == iidIUnknown || iid == iidIAgileObject || (h != nil && iid == h.iid) {
		*(*uintptr)(native(out)) = this
		handlerRef(this, 1)
		return uintptr(hresultOK)
	}
	*(*uintptr)(native(out)) = 0
	return uintptr(hresultNoInterface)
}

func handlerRef(this uintptr, delta int32) uintptr {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	h := handlers[this]
	if h == nil {
		return 0
	}
	h.refs += delta
	if h.refs <= 0 {
		delete(handlers, this)
		return 0
	}
	return uintptr(h.refs)
}

// newHandler creates a handler object holding one reference, which the caller
// releases once it has handed the object to COM. iid is the operation-specific
// completed-handler interface the object must answer QueryInterface for.
func newHandler(iid guid, fn func(a, b uintptr)) uintptr {
	h := &handler{vtbl: &handlerVtbl, refs: 1, iid: iid, fn: fn}
	p := uintptr(unsafe.Pointer(h))
	handlersMu.Lock()
	handlers[p] = h
	handlersMu.Unlock()
	return p
}
