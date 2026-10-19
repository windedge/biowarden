package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/windedge/biowarden/internal/registry"
	"github.com/windedge/biowarden/locales"
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

// The bulk install writes the safe rows - no registration, or already ours -
// straight away and hands the foreign and hijacked ones to the confirmation
// dialog. Nothing that belongs to another program is overwritten before a
// human has seen it.
func TestBrowsersStateInstallAllDefersTakeoversToConfirm(t *testing.T) {
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Chrome, name: "Chrome", state: browserMissing, installed: true},
		{browser: registry.Edge, name: "Edge", state: browserForeign, installed: true},
		{browser: registry.Firefox, name: "Firefox", state: browserOurs, installed: true},
	}}
	s := newBrowsersStateWith(backend)
	opened := false
	s.openConfirmWindow = func() { opened = true }
	s.installAll()

	if !opened {
		t.Fatalf("installAll did not open the confirmation dialog")
	}
	if got := strings.Join(backend.installs, ","); got != "Chrome,Firefox" {
		t.Fatalf("direct installs = %v, want the two safe rows", backend.installs)
	}
	if len(s.bulk) != 1 || s.bulk[0].name != "Edge" {
		t.Fatalf("bulk = %+v, want the Edge takeover", s.bulk)
	}
	if s.notice.text() != "" {
		t.Fatalf("notice = %q while the dialog owns the wording", s.notice.text())
	}

	s.confirmBulk()

	want := []string{"Chrome", "Firefox", "Edge"}
	if len(backend.installs) != len(want) {
		t.Fatalf("installs = %v, want %v", backend.installs, want)
	}
	for i := range want {
		if backend.installs[i] != want[i] {
			t.Fatalf("installs = %v, want %v", backend.installs, want)
		}
	}
	if got, want := s.notice.text(), locales.BrowsersNoticeInstalled(3); got != want || !s.notice.ok {
		t.Fatalf("after confirm notice = %q ok=%v, want %q", got, s.notice.ok, want)
	}
	if len(s.bulk) != 0 {
		t.Fatalf("bulk survived the confirm: %+v", s.bulk)
	}
}

// A hijacked row is our own config that another program overwrote, so it too
// waits for the one confirmation instead of being written silently.
func TestBrowsersStateInstallAllDefersHijacked(t *testing.T) {
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Firefox, name: "Firefox", state: browserHijacked, installed: true},
	}}
	s := newBrowsersStateWith(backend)
	s.installAll()

	if len(s.bulk) != 1 || s.bulk[0].state != browserHijacked {
		t.Fatalf("bulk = %+v, want the hijacked row", s.bulk)
	}
	if len(backend.installs) != 0 {
		t.Fatalf("hijacked row installed without confirmation: %v", backend.installs)
	}
}

// Cancelling the dialog writes none of the listings; the safe rows the button
// already installed stay installed.
func TestBrowsersStateInstallAllCancelLeavesTakeoversAlone(t *testing.T) {
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Chrome, name: "Chrome", state: browserMissing, installed: true},
		{browser: registry.Edge, name: "Edge", state: browserForeign, installed: true},
	}}
	s := newBrowsersStateWith(backend)
	s.installAll()
	s.cancelBulk()

	if got := strings.Join(backend.installs, ","); got != "Chrome" {
		t.Fatalf("installs = %v, want only the safe row", backend.installs)
	}
	for _, want := range []string{locales.BrowsersNoticeInstalled(1), locales.BrowsersNoticeSkippedCancelled(1)} {
		if !strings.Contains(s.notice.text(), want) {
			t.Fatalf("cancel notice %q does not contain %q", s.notice.text(), want)
		}
	}
	if s.notice.ok {
		t.Fatalf("notice.ok = true after cancelling a takeover")
	}
	if len(s.bulk) != 0 {
		t.Fatalf("bulk survived the cancel: %+v", s.bulk)
	}
}

// With nothing to take over there is no dialog at all: the click writes the
// safe rows and reports the plain summary.
func TestBrowsersStateInstallAllWithoutTakeoversWritesDirectly(t *testing.T) {
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Chrome, name: "Chrome", state: browserMissing, installed: true},
		{browser: registry.Firefox, name: "Firefox", state: browserOurs, installed: true},
	}}
	s := newBrowsersStateWith(backend)
	opened := false
	s.openConfirmWindow = func() { opened = true }
	s.installAll()

	if opened {
		t.Fatalf("installAll opened a dialog with no takeover rows")
	}
	if got, want := s.notice.text(), locales.BrowsersNoticeInstalled(2); got != want || !s.notice.ok {
		t.Fatalf("notice = %q ok=%v, want %q", got, s.notice.ok, want)
	}
}

// TestBrowsersStateInstallAllReportsFailureAcrossConfirm proves the single
// notice keeps both stages: a failed direct write must not be swallowed by
// the confirmation dialog, and the old "needs a per-row confirmation" skip
// wording is gone.
func TestBrowsersStateInstallAllReportsFailureAcrossConfirm(t *testing.T) {
	backend := &recordingBrowsersBackend{
		rows: []browserRow{
			{browser: registry.Chrome, name: "Chrome", state: browserMissing, installed: true},
			{browser: registry.Edge, name: "Edge", state: browserForeign, installed: true},
			{browser: registry.Firefox, name: "Firefox", state: browserMissing, installed: true},
		},
		failFor: map[string]bool{"Firefox": true},
	}
	s := newBrowsersStateWith(backend)
	s.installAll()
	s.confirmBulk()

	if s.notice.ok {
		t.Fatalf("notice.ok = true with a failed write")
	}
	for _, want := range []string{locales.BrowsersNoticeInstalled(2), locales.BrowsersNoticeFailed("Firefox")} {
		if !strings.Contains(s.notice.text(), want) {
			t.Fatalf("notice %q does not contain %q", s.notice.text(), want)
		}
	}
}

// A browser this machine does not have is skipped whole: it is never written,
// never listed in the confirmation dialog, and the summary names it so the
// human knows why nothing happened to that row.
func TestBrowsersStateInstallAllSkipsAbsentBrowsers(t *testing.T) {
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Chrome, name: "Chrome", state: browserMissing, installed: true},
		{browser: registry.Edge, name: "Edge", state: browserForeign, installed: true},
		{browser: registry.Firefox, name: "Firefox", state: browserForeign},
	}}
	s := newBrowsersStateWith(backend)
	opened := false
	s.openConfirmWindow = func() { opened = true }
	s.installAll()

	if !opened {
		t.Fatalf("installAll did not open the confirmation dialog")
	}
	if got := strings.Join(backend.installs, ","); got != "Chrome" {
		t.Fatalf("direct installs = %v, want only the installed safe row", backend.installs)
	}
	if len(s.bulk) != 1 || s.bulk[0].name != "Edge" {
		t.Fatalf("bulk = %+v, want only the installed foreign row", s.bulk)
	}

	s.confirmBulk()

	if got := strings.Join(backend.installs, ","); got != "Chrome,Edge" {
		t.Fatalf("installs = %v, want the installed rows only", backend.installs)
	}
	for _, want := range []string{locales.BrowsersNoticeInstalled(2), locales.BrowsersNoticeSkippedAbsent("Firefox")} {
		if !strings.Contains(s.notice.text(), want) {
			t.Fatalf("notice %q does not contain %q", s.notice.text(), want)
		}
	}
	if !s.notice.ok {
		t.Fatalf("notice = %q ok = false, want a clean summary", s.notice.text())
	}
}

// With no browser installed there is nothing to write and no dialog at all;
// the summary reports the empty run and names every skipped browser.
func TestBrowsersStateInstallAllWithNoBrowsersInstalled(t *testing.T) {
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Chrome, name: "Chrome", state: browserMissing},
		{browser: registry.Firefox, name: "Firefox", state: browserForeign},
	}}
	s := newBrowsersStateWith(backend)
	opened := false
	s.openConfirmWindow = func() { opened = true }
	s.installAll()

	if opened {
		t.Fatalf("installAll opened a dialog with no installed browsers")
	}
	if len(backend.installs) != 0 {
		t.Fatalf("installs = %v, want none", backend.installs)
	}
	absent := "Chrome" + locales.ListSeparator() + "Firefox"
	for _, want := range []string{locales.BrowsersNoticeInstalled(0), locales.BrowsersNoticeSkippedAbsent(absent)} {
		if !strings.Contains(s.notice.text(), want) {
			t.Fatalf("notice %q does not contain %q", s.notice.text(), want)
		}
	}
	if !s.notice.ok {
		t.Fatalf("notice = %q ok = false, want a missing browser to read as information", s.notice.text())
	}
}
