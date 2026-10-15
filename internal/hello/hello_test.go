//go:build windows

package hello

import (
	"os"
	"testing"
	"time"
)

func TestResultFromConsent(t *testing.T) {
	cases := []struct {
		in   int32
		want Result
	}{
		{consentVerified, Verified},
		{consentDeviceNotPresent, Unavailable},
		{consentNotConfiguredForUser, Unavailable},
		{consentDisabledByPolicy, Unavailable},
		{consentDeviceBusy, Busy},
		{consentRetriesExhausted, Locked},
		{consentCanceled, Canceled},
		{42, Busy}, // outside the SDK enum: keep a failed check from reading as success
	}
	for _, c := range cases {
		if got := resultFromConsent(c.in); got != c.want {
			t.Errorf("resultFromConsent(%d) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAvailableFromAvailability(t *testing.T) {
	cases := []struct {
		in   int32
		want bool
	}{
		{availabilityAvailable, true},
		{availabilityDeviceBusy, true},
		{availabilityDeviceNotPresent, false},
		{availabilityNotConfiguredForUser, false},
		{availabilityDisabledByPolicy, false},
		{42, false},
	}
	for _, c := range cases {
		if got := availableFromAvailability(c.in); got != c.want {
			t.Errorf("availableFromAvailability(%d) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestGUIDConstants(t *testing.T) {
	cases := []struct {
		name string
		got  guid
		want guid
	}{
		{"IUserConsentVerifierInterop", iidConsentInterop,
			guid{0x39E050C3, 0x4E74, 0x441A, [8]byte{0x8D, 0xC0, 0xB8, 0x11, 0x04, 0xDF, 0x94, 0x9C}}},
		{"IAsyncOperation<UserConsentVerificationResult>", iidConsentAsync,
			guid{0xFD596FFD, 0x2318, 0x558F, [8]byte{0x9D, 0xBE, 0xD2, 0x1D, 0xF4, 0x37, 0x64, 0xA5}}},
		{"AsyncOperationCompletedHandler<UserConsentVerificationResult>", iidConsentHandler,
			guid{0x0CFFC6C9, 0x4C2B, 0x5CD4, [8]byte{0xB3, 0x8C, 0x7B, 0x8D, 0xF3, 0xFF, 0x5A, 0xFB}}},
		{"AsyncOperationCompletedHandler<UserConsentVerifierAvailability>", iidAvailabilityHandler,
			guid{0x28988174, 0xACE2, 0x5C15, [8]byte{0xA0, 0xDF, 0x58, 0x0A, 0x26, 0xD9, 0x42, 0x94}}},
		{"IUserConsentVerifierStatics", iidConsentStatics,
			guid{0xAF4F3F91, 0x564C, 0x4DDC, [8]byte{0xB8, 0xB5, 0x97, 0x34, 0x47, 0x62, 0x7C, 0x65}}},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

func TestHStringCreateDelete(t *testing.T) {
	for _, s := range []string{"", "biowarden", "验证 Windows Hello", "a\U0001F600b"} {
		h, err := newHString(s)
		if err != nil {
			t.Fatalf("newHString(%q): %v", s, err)
		}
		if s != "" && h == 0 {
			t.Fatalf("newHString(%q) returned a null handle", s)
		}
		deleteHString(h)
	}
	deleteHString(0) // deleting a null handle must be a no-op
	for i := 0; i < 1000; i++ {
		h, err := newHString("biowarden")
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		deleteHString(h)
	}
}

func TestPromptOwnerWindow(t *testing.T) {
	hwnd, err := promptOwner()
	if err != nil {
		t.Fatalf("promptOwner: %v", err)
	}
	if hwnd == 0 {
		t.Fatal("promptOwner returned a null window")
	}
	if r, _, _ := procIsWindow.Call(hwnd); r == 0 {
		t.Fatalf("owner window %#x is not a window", hwnd)
	}
	exists, visible, topmost := ownerWindowState(hwnd)
	if !exists || visible || !topmost {
		t.Fatalf("owner window state = exists:%t visible:%t topmost:%t, want exists:true visible:false topmost:true",
			exists, visible, topmost)
	}
}

// TestLendForeground exercises the foreground borrow without showing a Hello
// prompt: on an attended desktop the owner must briefly become the foreground
// window and the previous window must get it back. It borrows the real
// foreground, so it is opt-in like TestVerifyProbe. A session with no
// foreground window (locked or headless) has nothing to borrow, so the test
// skips the assertion instead of failing.
func TestLendForeground(t *testing.T) {
	if os.Getenv("BIOWARDEN_HELLO_PROBE") == "" {
		t.Skip("set BIOWARDEN_HELLO_PROBE=1 to run the foreground borrow")
	}
	hwnd, err := promptOwner()
	if err != nil {
		t.Fatalf("promptOwner: %v", err)
	}
	before, _, _ := procGetForegroundWindow.Call()
	lend := lendForeground(hwnd)
	t.Logf("before=%#x lend=%s", before, lend)
	if before == 0 {
		t.Skip("no foreground window in this session")
	}
	if !lend.setFg {
		t.Fatalf("SetForegroundWindow(owner) failed: %s", lend)
	}
	if !waitForeground(hwnd, 2*time.Second) {
		t.Fatalf("owner %#x did not become the foreground window", hwnd)
	}
	ok, errno := returnForeground(lend.prevFg, hwnd, lend.shownByUs)
	t.Logf("returnForeground ok=%t errno=%d", ok, errno)
	if !waitForeground(before, 2*time.Second) {
		t.Fatalf("foreground did not return to %#x", before)
	}
	if _, visible, topmost := ownerWindowState(hwnd); visible || !topmost {
		t.Fatalf("owner visible/not topmost after restore: visible=%t topmost=%t", visible, topmost)
	}
}

// waitForeground polls GetForegroundWindow until it reports hwnd; the switch is
// queued by the window manager, so a read immediately after the call can
// still see the old (or a null) value.
func waitForeground(hwnd uintptr, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if fg, _, _ := procGetForegroundWindow.Call(); fg == hwnd {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestVerifyProbe runs the real consent flow, which needs a human to answer
// the Windows Hello prompt. It is opt-in so the default suite stays
// unattended: set BIOWARDEN_HELLO_PROBE=1 and run it with a timeout, e.g.
// go test ./internal/hello/ -run TestVerifyProbe -v -timeout 90s.
func TestVerifyProbe(t *testing.T) {
	if os.Getenv("BIOWARDEN_HELLO_PROBE") == "" {
		t.Skip("set BIOWARDEN_HELLO_PROBE=1 to run the interactive probe")
	}
	v := New()
	available, err := v.Available()
	t.Logf("Available() = %v, err = %v", available, err)
	if err != nil || !available {
		return
	}
	result, err := v.Verify("Biowarden probe: confirm Windows Hello")
	t.Logf("Verify() = %v, err = %v", result, err)
}
