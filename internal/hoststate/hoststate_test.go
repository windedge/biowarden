package hoststate

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIdentify(t *testing.T) {
	cases := map[string]string{
		"chrome.exe": BrowserChrome,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`: BrowserChrome,
		"CHROME.EXE":             BrowserChrome,
		"chromium.exe":           BrowserChromium,
		"msedge.exe":             BrowserEdge,
		"firefox.exe":            BrowserFirefox,
		"brave.exe":              BrowserBrave,
		"vivaldi.exe":            BrowserVivaldi,
		"notepad.exe":            "",
		"":                       "",
		"   ":                    "",
		`C:\tools\biowarden.exe`: "",
	}
	for exe, want := range cases {
		if got := Identify(exe); got != want {
			t.Errorf("Identify(%q) = %q, want %q", exe, got, want)
		}
	}
}

// Chrome and Chromium launch from different executables even though biowarden
// registers them at the same keys, so they must stay distinct here.
func TestIdentifySeparatesChromeAndChromium(t *testing.T) {
	if Identify("chrome.exe") == Identify("chromium.exe") {
		t.Fatal("Identify maps chrome.exe and chromium.exe to the same id")
	}
}

func TestRecordName(t *testing.T) {
	cases := map[string]string{
		"chrome":        "chrome",
		"Chrome":        "chrome",
		"":              BrowserUnknown,
		"../../evil":    "evil",
		`..\..\evil`:    "evil",
		"a b!c":         "abc",
		"chrome.exe":    "chromeexe",
		"brave-browser": "brave-browser",
		"a/b:c*d":       "abcd",
		"__proto__":     "__proto__",
	}
	for in, want := range cases {
		if got := recordName(in); got != want {
			t.Errorf("recordName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRecordAndRead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "connections")
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := RecordConnectionIn(dir, BrowserChrome, at); err != nil {
		t.Fatalf("RecordConnectionIn: %v", err)
	}
	got, err := LastConnectionsIn(dir)
	if err != nil {
		t.Fatalf("LastConnectionsIn: %v", err)
	}
	if !got[BrowserChrome].Equal(at) {
		t.Fatalf("chrome time = %v, want %v", got[BrowserChrome], at)
	}
	if len(got) != 1 {
		t.Fatalf("read %d records, want 1: %v", len(got), got)
	}
}

func TestRecordConnectionEmptyBrowserUsesUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := RecordConnectionIn(dir, "", time.Now()); err != nil {
		t.Fatalf("RecordConnectionIn: %v", err)
	}
	got, err := LastConnectionsIn(dir)
	if err != nil {
		t.Fatalf("LastConnectionsIn: %v", err)
	}
	if _, ok := got[BrowserUnknown]; !ok {
		t.Fatalf("records = %v, want an %q entry", got, BrowserUnknown)
	}
}

func TestRecordConnectionZeroTimeStampsNow(t *testing.T) {
	dir := t.TempDir()
	before := time.Now().Add(-time.Second)
	if err := RecordConnectionIn(dir, BrowserEdge, time.Time{}); err != nil {
		t.Fatalf("RecordConnectionIn: %v", err)
	}
	got, err := LastConnectionsIn(dir)
	if err != nil {
		t.Fatalf("LastConnectionsIn: %v", err)
	}
	at := got[BrowserEdge]
	if at.Before(before) || at.After(time.Now().Add(time.Second)) {
		t.Fatalf("stamped time = %v, want about now", at)
	}
}

func TestLastConnectionsMissingDirIsEmpty(t *testing.T) {
	got, err := LastConnectionsIn(filepath.Join(t.TempDir(), "never-created"))
	if err != nil {
		t.Fatalf("LastConnectionsIn on a missing dir = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("read %v from a missing dir, want empty", got)
	}
}

func TestLastConnectionsSkipsUnparsableAndTemp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "firefox"), []byte("not a time"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chrome.123.tmp"), []byte("2026-01-01T00:00:00Z"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LastConnectionsIn(dir)
	if err != nil {
		t.Fatalf("LastConnectionsIn: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("records = %v, want garbage and temp files skipped", got)
	}
}

// Many host processes can start at once; each browser's file must end up with
// one complete timestamp, never a mix of writers.
func TestRecordConnectionConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	browsers := []string{BrowserChrome, BrowserChromium, BrowserEdge, BrowserFirefox}
	const perBrowser = 20

	var wg sync.WaitGroup
	for _, browser := range browsers {
		for i := 0; i < perBrowser; i++ {
			wg.Add(1)
			go func(browser string, i int) {
				defer wg.Done()
				at := time.Unix(int64(1_700_000_000+i), 0).UTC()
				if err := RecordConnectionIn(dir, browser, at); err != nil {
					t.Errorf("RecordConnectionIn(%s): %v", browser, err)
				}
			}(browser, i)
		}
	}
	wg.Wait()

	got, err := LastConnectionsIn(dir)
	if err != nil {
		t.Fatalf("LastConnectionsIn: %v", err)
	}
	if len(got) != len(browsers) {
		t.Fatalf("read %d browsers, want %d: %v", len(got), len(browsers), got)
	}
	valid := map[time.Time]bool{}
	for i := 0; i < perBrowser; i++ {
		valid[time.Unix(int64(1_700_000_000+i), 0).UTC()] = true
	}
	for _, browser := range browsers {
		if !valid[got[browser]] {
			t.Fatalf("%s time = %v, want one of the written stamps", browser, got[browser])
		}
	}
	// No temp file may be left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("leftover temp file %s", entry.Name())
		}
	}
}
