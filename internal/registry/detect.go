package registry

// Browser installation probe.
//
// Windows resolves an executable name through its App Paths keys, which the
// installer of every browser biowarden supports creates and its uninstaller
// removes. That makes App Paths the cheapest honest evidence that a browser is
// on this machine: the probe reads the default value of a fixed set of keys
// and reports nothing it did not see.
//
// Verified on the development machine (2026-02), which is why these keys are
// the ones read:
//   - HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe
//     exists and points at scoop's ungoogled-chromium. HKLM has no chrome.exe
//     key at all, so reading HKLM only would have missed that browser.
//   - HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe
//     exists, in the 64-bit view and again under the literal WOW6432Node path.
//   - firefox.exe and chromium.exe exist in neither, and correctly so: Firefox
//     is not installed here, and the installed Chromium registers itself under
//     the chrome.exe name. A missing key is therefore a real "not installed",
//     not a gap in the probe.
//
// A browser whose installation leaves no App Paths entry (an unpacked or
// portable copy) is not reported. The probe never guesses: an unreadable key
// means "not installed", and the result may legitimately be empty.

// appPathExecutables maps a browser to the App Paths names that identify it.
// Chrome and Chromium share one registration row, so either executable is
// enough evidence for that row.
var appPathExecutables = map[Browser][]string{
	Chrome:  {"chrome.exe", "chromium.exe"},
	Edge:    {"msedge.exe"},
	Firefox: {"firefox.exe"},
}

// detectInstalled maps an App Paths lookup to the installed browsers, in
// Browsers order so the result is stable between calls.
func detectInstalled(read func(exe string) (string, bool)) []Browser {
	var installed []Browser
	for _, b := range Browsers {
		for _, exe := range appPathExecutables[b] {
			if _, ok := read(exe); ok {
				installed = append(installed, b)
				break
			}
		}
	}
	return installed
}

// Detect reports the supported browsers installed for the current user. It is
// best-effort and cannot fail: a registry read that does not work simply means
// that browser is not reported.
func Detect() []Browser { return detectInstalled(readAppPath) }
