package ui

import (
	"errors"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/registry"
)

// Manifest states of a browser, as the registry seam reports them.
const (
	browserMissing  = iota // no manifest registered
	browserOurs            // our manifest
	browserForeign         // a manifest we did not write (likely the official client)
	browserHijacked        // our manifest file exists, but the key points elsewhere
)

// browsersState is the Browsers page's working data, backed by the registry
// seam.
type browsersState struct {
	backend browsersBackend

	rows []browserRow

	// lastReload is when reload last ran, the tick throttle's timestamp.
	lastReload time.Time
	// ticked is whether tick has run at least once; the first tick reloads
	// whatever the throttle says, so switching to the page always shows a
	// fresh read.
	ticked bool

	// confirmCover and confirmUninstall are the row indexes (into rows)
	// awaiting a second confirming click; -1 when none. Overwriting or
	// deleting a manifest that may be the official client's needs the same
	// deliberateness as the reference's overwrite flow.
	confirmCover     int
	confirmUninstall int

	// bulk holds the foreign and hijacked rows "Install all" collected into
	// one confirmation, instead of skipping them row by row. Empty when no
	// dialog is up. bulkInstalled, bulkFailed and bulkAbsent carry the rest of
	// the button's summary (written, failed, and not installed), so the dialog
	// reports the full picture whatever the human decides.
	bulk          []browserRow
	bulkInstalled int
	bulkFailed    []string
	bulkAbsent    []string

	// openConfirmWindow and closeConfirmWindow are the GUI seams, nil in tests,
	// mirroring the login window's.
	openConfirmWindow  func()
	closeConfirmWindow func()

	notice pageNotice
}

type browserRow struct {
	browser registry.Browser // the registration point install/uninstall target
	name    string           // display name, from registry.Browsers
	path    string           // manifest path currently registered, "" when none
	// installPath is where biowarden's manifest for this browser lives. The
	// overwrite confirmation lists it as the target a takeover would write.
	installPath string
	state       int // browserMissing / browserOurs / browserForeign / browserHijacked
	// installed is whether the browser is present on this machine. A browser
	// that is not installed has no registration point to write, so the row is
	// marked and install all skips it.
	installed bool

	// lastConnectedAt is when this browser's extension last reached the host,
	// zero when it never has. The host writes it per connection; the row shows
	// "connected · <relative time>" from it and "extension never connected"
	// while zero.
	lastConnectedAt time.Time
}

// newBrowsersState takes the process's shared registrar; the injectable form
// below it is what tests use.
func newBrowsersState(reg registry.Registrar) browsersState {
	return newBrowsersStateWith(newRealBrowsers(reg))
}

// newBrowsersStateWith is the injectable form; tests pass an in-memory
// backend, the app builds the registry-backed one.
func newBrowsersStateWith(backend browsersBackend) browsersState {
	s := browsersState{
		backend:          backend,
		confirmCover:     -1,
		confirmUninstall: -1,
	}
	s.reload()
	return s
}

// browsersReloadInterval is how far apart the tick's own reloads stay. Hover
// animations keep the view rebuilding frame by frame (AnimateColor asks for
// frames until the fill settles), and a registration does not change while a
// button fill fades, so the background path re-reads at most this often; the
// actions still reload at once through reload itself.
const browsersReloadInterval = 2 * time.Second

// tick refreshes on rebuilds, so reopening or switching to the page shows the
// registry as it is now rather than what it was when the app started. A rebuild
// comes with every animation frame, so the reload is throttled to
// browsersReloadInterval; only this background path waits. The first rebuild
// with the page on screen is exempt, so switching to the page always shows a
// fresh read no matter how recently the state was built or reloaded.
func (s *browsersState) tick(c *ui.Context) {
	first := !s.ticked
	s.ticked = true
	if !first && time.Since(s.lastReload) < browsersReloadInterval {
		return
	}
	s.reload()
}

// reload re-reads every browser's manifest state, and stamps the throttle so a
// write the human just asked for is not immediately re-read by the tick.
func (s *browsersState) reload() {
	s.lastReload = time.Now()
	rows, err := s.backend.refresh()
	if err != nil {
		s.notice = pageNotice{kind: noticeBrowsersReadFailed, err: err}
		return
	}
	s.rows = rows
}

// installAll is the page's one-click install. Browsers this machine does not
// have are skipped whole: there is no registration point to write. Rows with
// no registration and rows already ours are written straight away. A foreign
// or hijacked value is collected into one confirmation instead of being
// skipped row by row: the button opens a modal dialog listing every
// registration it would take over, and only the human's confirm writes them.
func (s *browsersState) installAll() {
	// A bulk write reloads the rows, so any per-row confirmation still open
	// would point at a stale index.
	s.confirmCover, s.confirmUninstall = -1, -1
	installed := 0
	var failed []string
	var absent []string
	var takeovers []browserRow
	for i := range s.rows {
		row := s.rows[i]
		if !row.installed {
			absent = append(absent, row.name)
			continue
		}
		switch row.state {
		case browserMissing, browserOurs:
			if err := s.backend.install(row); err != nil {
				failed = append(failed, row.name)
				continue
			}
			installed++
		case browserForeign, browserHijacked:
			takeovers = append(takeovers, row)
		}
	}
	s.reload()
	s.bulk = takeovers
	s.bulkInstalled, s.bulkFailed, s.bulkAbsent = installed, failed, absent
	if len(takeovers) == 0 {
		s.notice = bulkNotice(installed, failed, absent, 0)
		return
	}
	// The dialog owns the wording while it is up; the click that opened it
	// must not leave a summary that reads as if the takeovers were skipped.
	s.notice = pageNotice{}
	if s.openConfirmWindow != nil {
		s.openConfirmWindow()
	}
}

// confirmBulk takes over every registration the dialog listed and closes it.
func (s *browsersState) confirmBulk() {
	failed := append([]string(nil), s.bulkFailed...)
	absent := append([]string(nil), s.bulkAbsent...)
	taken := 0
	for _, row := range s.bulk {
		if err := s.backend.install(row); err != nil {
			failed = append(failed, row.name)
			continue
		}
		taken++
	}
	installed := s.bulkInstalled + taken
	s.bulk, s.bulkInstalled, s.bulkFailed, s.bulkAbsent = nil, 0, nil, nil
	s.reload()
	s.notice = bulkNotice(installed, failed, absent, 0)
	s.closeConfirm()
}

// cancelBulk leaves every listed registration alone and closes the dialog.
// The rows the button already wrote directly are kept, so a cancel never
// undoes work that needed no confirmation.
func (s *browsersState) cancelBulk() {
	s.applyBulkCancel()
	s.closeConfirm()
}

// applyBulkCancel sets the cancelled state without touching the window. The
// close event uses it when the human clicks the dialog's X, which destroys
// the window on its own path.
func (s *browsersState) applyBulkCancel() {
	installed, failed, absent, left := s.bulkInstalled, s.bulkFailed, s.bulkAbsent, len(s.bulk)
	s.bulk, s.bulkInstalled, s.bulkFailed, s.bulkAbsent = nil, 0, nil, nil
	if left == 0 {
		s.notice = pageNotice{ok: true}
		return
	}
	s.notice = bulkNotice(installed, failed, absent, left)
}

// closeConfirm asks the GUI to destroy the dialog. It is a no-op in tests.
func (s *browsersState) closeConfirm() {
	if s.closeConfirmWindow != nil {
		s.closeConfirmWindow()
	}
}

// bulkNotice is the one summary the bulk install keeps: what was written,
// what failed, the browsers this machine does not have, and the takeover rows
// a cancel left untouched (zero once the dialog was confirmed). A missing
// browser is not a failure, so it does not turn the line orange.
func bulkNotice(installed int, failed, absent []string, left int) pageNotice {
	ok := len(failed) == 0 && left == 0
	return pageNotice{kind: noticeBrowsersBulk, ok: ok, bulk: &bulkSummary{
		installed: installed,
		failed:    failed,
		absent:    absent,
		left:      left,
	}}
}

// install writes our manifest for the row, replacing whatever was registered.
func (s *browsersState) install(row *browserRow) {
	if err := s.backend.install(*row); err != nil {
		s.notice = pageNotice{kind: noticeBrowsersManifestWriteFailed, args: []string{row.name}, err: err}
		return
	}
	s.reload()
	s.notice = pageNotice{kind: noticeBrowsersManifestWritten, args: []string{row.name}, ok: true}
}

// uninstall removes our manifest, unless the registration is foreign: then the
// registry leaves it alone and the page says so.
func (s *browsersState) uninstall(row *browserRow) {
	err := s.backend.uninstall(*row)
	s.reload()
	switch {
	case errors.Is(err, errForeignRegistration):
		s.notice = pageNotice{kind: noticeBrowsersUninstallForeign, args: []string{row.name}}
	case err != nil:
		s.notice = pageNotice{kind: noticeBrowsersManifestDeleteFailed, args: []string{row.name}, err: err}
	default:
		s.notice = pageNotice{kind: noticeBrowsersManifestDeleted, args: []string{row.name}, ok: true}
	}
}
