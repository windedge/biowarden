package ui

import "testing"

// recordingBrowsersBackend records which rows a bulk install touched, so the
// "install all" test can prove it skipped the registrations that may belong to
// the official client.
type recordingBrowsersBackend struct {
	rows     []browserRow
	installs []string
}

func (r *recordingBrowsersBackend) refresh() ([]browserRow, error) { return r.rows, nil }

func (r *recordingBrowsersBackend) install(row browserRow) error {
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
	if s.noticeOK != true {
		t.Fatalf("noticeOK = false after a partial install")
	}
	if s.notice == "" {
		t.Fatalf("installAll produced no notice")
	}
}
