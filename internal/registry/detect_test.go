package registry

import "testing"

// The probe is a mapping from App Paths entries to browsers, so it is tested
// with a lookup that answers from a set of executable names: no registry, no
// machine state.
func TestDetectInstalledMapsAppPathsToBrowsers(t *testing.T) {
	tests := []struct {
		name  string
		found []string
		want  []Browser
	}{
		{"nothing installed", nil, nil},
		{"chrome", []string{"chrome.exe"}, []Browser{Chrome}},
		{"chromium covers the chrome row", []string{"chromium.exe"}, []Browser{Chrome}},
		{"both chrome executables are one row", []string{"chrome.exe", "chromium.exe"}, []Browser{Chrome}},
		{"edge", []string{"msedge.exe"}, []Browser{Edge}},
		{"firefox", []string{"firefox.exe"}, []Browser{Firefox}},
		{"installation order does not decide the result", []string{"firefox.exe", "msedge.exe", "chrome.exe"}, Browsers},
		{"an unknown executable is ignored", []string{"opera.exe", "brave.exe"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := make(map[string]bool, len(tt.found))
			for _, exe := range tt.found {
				found[exe] = true
			}
			read := func(exe string) (string, bool) {
				if found[exe] {
					return `C:\program files\` + exe, true
				}
				return "", false
			}

			got := detectInstalled(read)
			if len(got) != len(tt.want) {
				t.Fatalf("detectInstalled = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("detectInstalled = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// Every executable the probe knows about has to be asked for, or a browser
// would silently become undetectable.
func TestDetectInstalledAsksForEveryKnownExecutable(t *testing.T) {
	asked := map[string]bool{}
	detectInstalled(func(exe string) (string, bool) {
		asked[exe] = true
		return "", false
	})

	for _, b := range Browsers {
		for _, exe := range appPathExecutables[b] {
			if !asked[exe] {
				t.Errorf("detectInstalled never asked for %s", exe)
			}
		}
	}
}

// Detect reads the real App Paths keys. Asserting what this machine has would
// make the test machine-specific, so it only checks that the answer is a
// subset of the supported browsers; the paths are logged for a human.
func TestDetectReadsTheRealRegistry(t *testing.T) {
	got := Detect()
	supported := map[Browser]bool{}
	for _, b := range Browsers {
		supported[b] = true
	}
	for _, b := range got {
		if !supported[b] {
			t.Fatalf("Detect returned an unsupported browser %q", b)
		}
	}
	for _, b := range Browsers {
		for _, exe := range appPathExecutables[b] {
			if path, ok := readAppPath(exe); ok {
				t.Logf("%s: %s", exe, path)
			}
		}
	}
}
