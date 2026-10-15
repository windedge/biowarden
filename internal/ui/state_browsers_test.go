package ui

import (
	"errors"
	"strings"
	"testing"
)

// recordingBrowsersBackend records which rows a bulk install touched, so the
// "install all" test can prove it skipped the registrations that may belong to
// the official client. failFor names rows whose install must fail.
type recordingBrowsersBackend struct {
	rows     []browserRow
	installs []string
	failFor  map[string]bool
}

func (r *recordingBrowsersBackend) refresh() ([]browserRow, error) { return r.rows, nil }

func (r *recordingBrowsersBackend) install(row browserRow) error {
	if r.failFor[row.name] {
		return errors.New("write failed")
	}
	r.installs = append(r.installs, row.name)
	return nil
}

func (r *recordingBrowsersBackend) uninstall(browserRow) error { return nil }

func TestBrowsersStateInstallAllSkipsForeign(t *testing.T) {
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{name: "Chrome", state: browserMissing},
		{name: "Edge", state: browserForeign},
		{name: "Firefox", state: browserOurs},
	}}
	s := newBrowsersStateWith(backend)
	s.installAll()

	want := []string{"Chrome", "Firefox"}
	if len(backend.installs) != len(want) {
		t.Fatalf("installs = %v, want %v", backend.installs, want)
	}
	for i := range want {
		if backend.installs[i] != want[i] {
			t.Fatalf("installs = %v, want %v", backend.installs, want)
		}
	}
	if s.noticeOK != false {
		t.Fatalf("noticeOK = true after skipping a foreign registration; the notice is a warning")
	}
	if s.notice == "" {
		t.Fatalf("installAll produced no notice")
	}
}

// TestBrowsersStateInstallAllReportsFailureAndSkip proves the single notice
// keeps both the failed write and the skipped foreign registration visible:
// the failure path must not swallow the skip.
func TestBrowsersStateInstallAllReportsFailureAndSkip(t *testing.T) {
	backend := &recordingBrowsersBackend{
		rows: []browserRow{
			{name: "Chrome", state: browserMissing},
			{name: "Edge", state: browserForeign},
			{name: "Firefox", state: browserMissing},
		},
		failFor: map[string]bool{"Firefox": true},
	}
	s := newBrowsersStateWith(backend)
	s.installAll()

	if s.noticeOK != false {
		t.Fatalf("noticeOK = true with a failed write and a skip")
	}
	for _, want := range []string{"已安装 1 个", "失败：Firefox", "1 个需逐行确认"} {
		if !strings.Contains(s.notice, want) {
			t.Fatalf("notice %q does not contain %q", s.notice, want)
		}
	}
}
