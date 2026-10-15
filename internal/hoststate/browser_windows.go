//go:build windows

package hoststate

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DetectBrowser names the browser process that launched this host, or "" when
// it cannot be read. A native messaging host has no browser argument to go on
// (the origin is the same Bitwarden extension id everywhere), so the parent
// process's executable name is the signal. It distinguishes Chrome from
// Chromium: the parents are chrome.exe and chromium.exe even though biowarden
// registers both through one Chrome row.
//
// The limitation is that the name is whatever started us: a launcher script or
// a browser's own helper would be misread, and a browser whose executable is
// renamed is unrecognized. The registry only points Chrome, Chromium, Edge and
// Firefox at us, so the practical set is known.
func DetectBrowser() string {
	exe, err := parentExeName()
	if err != nil {
		return ""
	}
	return Identify(exe)
}

// parentExeName is DetectBrowser's untranslated half: the parent's executable
// base name, kept separate so the mapping stays testable without a parent.
func parentExeName() (string, error) {
	parent := windows.Getppid()
	if parent <= 0 {
		return "", errors.New("hoststate: parent process id unavailable")
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return "", err
	}
	for {
		if entry.ProcessID == uint32(parent) {
			return windows.UTF16ToString(entry.ExeFile[:]), nil
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return "", err
		}
	}
	return "", errors.New("hoststate: parent process not in snapshot")
}
