package hello

import (
	"fmt"
	"runtime"
	"unsafe"
)

// The UserConsentVerifier runtime class and the enum values of
// UserConsentVerificationResult / UserConsentVerifierAvailability, from the
// Windows SDK (winrt/windows.security.credentials.ui.h). The numeric order is
// not the order the results read in: DisabledByPolicy is 3 while DeviceBusy is
// 4, and RetriesExhausted is 5 while Canceled is 6.
const (
	userConsentVerifierClass = "Windows.Security.Credentials.UI.UserConsentVerifier"

	consentVerified             = 0
	consentDeviceNotPresent     = 1
	consentNotConfiguredForUser = 2
	consentDisabledByPolicy     = 3
	consentDeviceBusy           = 4
	consentRetriesExhausted     = 5
	consentCanceled             = 6

	availabilityAvailable            = 0
	availabilityDeviceNotPresent     = 1
	availabilityNotConfiguredForUser = 2
	availabilityDisabledByPolicy     = 3
	availabilityDeviceBusy           = 4
)

const (
	infinite         = 0xFFFFFFFF
	waitObject0      = 0
	eventManualReset = 1
)

// New returns the Windows Hello verifier backed by the UserConsentVerifier
// interop: the check gates access to a key held elsewhere, it encrypts
// nothing.
func New() Verifier {
	v := &winVerifier{jobs: make(chan *job)}
	go v.run()
	return v
}

type winVerifier struct {
	jobs chan *job
}

type job struct {
	checkAvailability bool
	reason            string
	avail             bool
	result            Result
	err               error
	done              chan struct{}
}

func (v *winVerifier) Available() (bool, error) {
	j := &job{checkAvailability: true, done: make(chan struct{})}
	v.jobs <- j
	<-j.done
	return j.avail, j.err
}

func (v *winVerifier) Verify(reason string) (Result, error) {
	j := &job{reason: reason, done: make(chan struct{})}
	v.jobs <- j
	<-j.done
	return j.result, j.err
}

// run owns one OS thread for its whole life: WinRT activation is per thread,
// so every call must happen in the same MTA, and the synchronous event wait
// must never occupy a goroutine the caller cares about.
func (v *winVerifier) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	uninit, err := roInitialize()
	defer uninit()
	for j := range v.jobs {
		switch {
		case err != nil:
			j.err = err
		case j.checkAvailability:
			j.avail, j.err = checkAvailability()
		default:
			j.result, j.err = requestConsent(j.reason)
		}
		close(j.done)
	}
}

func checkAvailability() (bool, error) {
	factory, err := activationFactory(userConsentVerifierClass, &iidConsentStatics)
	if err != nil {
		return false, err
	}
	defer release(factory)
	var op uintptr
	if hr := comCall(factory, 6, uintptr(unsafe.Pointer(&op))); failed(hr) { // CheckAvailabilityAsync
		return false, hresultErr("CheckAvailabilityAsync", hr)
	}
	if op == 0 {
		return false, fmt.Errorf("hello: CheckAvailabilityAsync returned no operation")
	}
	defer release(op)
	var availability int32
	if err := waitForAsync(op, iidAvailabilityHandler, unsafe.Pointer(&availability)); err != nil {
		return false, err
	}
	return availableFromAvailability(availability), nil
}

func requestConsent(reason string) (Result, error) {
	hwnd, err := promptOwner()
	if err != nil {
		return Unavailable, err
	}
	// Borrow the foreground so the prompt can take keyboard focus, and give it
	// back when the prompt is gone. The topmost owner keeps the prompt above
	// other windows even if the borrow is refused.
	lend := lendForeground(hwnd)
	exists, visible, topmost := ownerWindowState(hwnd)
	logDebug(fmt.Sprintf("hello: prompt request ownerExists=%t ownerVisible=%t ownerTopmost=%t %s",
		exists, visible, topmost, lend))
	defer func() {
		ok, errno := returnForeground(lend.prevFg, hwnd, lend.shownByUs)
		exists, visible, topmost = ownerWindowState(hwnd)
		logDebug(fmt.Sprintf("hello: prompt done restoreFg=%#x restoreOK=%t restoreErr=%d ownerVisible=%t ownerTopmost=%t",
			lend.prevFg, ok, errno, visible, topmost))
	}()
	factory, err := activationFactory(userConsentVerifierClass, &iidConsentInterop)
	if err != nil {
		return Unavailable, err
	}
	defer release(factory)
	message, err := newHString(reason)
	if err != nil {
		return Unavailable, err
	}
	defer deleteHString(message)
	var op uintptr
	hr := comCall(factory, 6, hwnd, message,
		uintptr(unsafe.Pointer(&iidConsentAsync)), uintptr(unsafe.Pointer(&op)))
	if failed(hr) {
		return Unavailable, hresultErr("RequestVerificationForWindowAsync", hr)
	}
	if op == 0 {
		return Unavailable, fmt.Errorf("hello: RequestVerificationForWindowAsync returned no operation")
	}
	defer release(op)
	var result int32
	if err := waitForAsync(op, iidConsentHandler, unsafe.Pointer(&result)); err != nil {
		return Unavailable, err
	}
	return resultFromConsent(result), nil
}

// waitForAsync blocks until op completes and writes its result through
// GetResults (vtable slot 8). handlerIID is the operation's completed-handler
// interface: put_Completed queries its argument for it. The completed handler
// runs on a WinRT thread pool thread, so no message pump is needed here; the
// event is the only cross-thread signal. put_Completed also delivers
// completion when op has already finished, which is why the status is not
// queried through IAsyncInfo.
func waitForAsync(op uintptr, handlerIID guid, results unsafe.Pointer) error {
	event, _, err := procCreateEventW.Call(0, eventManualReset, 0, 0)
	if event == 0 {
		return fmt.Errorf("hello: CreateEventW: %v", err)
	}
	defer procCloseHandle.Call(event)

	handler := newHandler(handlerIID, func(_, _ uintptr) { procSetEvent.Call(event) })
	hr := comCall(op, 6, handler) // put_Completed
	release(handler)
	if failed(hr) {
		return hresultErr("put_Completed", hr)
	}
	if r, _, _ := procWaitForSingleObject.Call(event, infinite); r != waitObject0 {
		return fmt.Errorf("hello: WaitForSingleObject returned %#x", uint32(r))
	}
	if hr := comCall(op, 8, uintptr(results)); failed(hr) { // GetResults
		return hresultErr("GetResults", hr)
	}
	return nil
}

// resultFromConsent maps the UserConsentVerificationResult enum. An
// out-of-range value cannot happen against the shipped runtime; treating it as
// Busy keeps the caller from reading a failed check as success.
func resultFromConsent(v int32) Result {
	switch v {
	case consentVerified:
		return Verified
	case consentCanceled:
		return Canceled
	case consentRetriesExhausted:
		return Locked
	case consentDeviceNotPresent, consentNotConfiguredForUser, consentDisabledByPolicy:
		return Unavailable
	case consentDeviceBusy:
		return Busy
	default:
		return Busy
	}
}

func availableFromAvailability(v int32) bool {
	switch v {
	case availabilityAvailable, availabilityDeviceBusy:
		return true
	default:
		return false
	}
}
