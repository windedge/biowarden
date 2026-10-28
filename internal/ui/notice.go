package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// noticeSeverity is how loud a status line is, which is what the drawn banner
// takes its wash margin, its icon and its ink from. Three steps are enough: a
// line either reports progress or success, warns of something that may pass on
// its own, or reports a failure.
type noticeSeverity int

const (
	// severityInfo is progress and success: a check under way, a key saved, a
	// session confirmed.
	severityInfo noticeSeverity = iota
	// severityWarning is the temporary kind of trouble - a slow network, a rate
	// limit, a server that did not answer - and anything that needs a human's
	// attention without having failed.
	severityWarning
	// severityDanger is a failure that stays failed until someone acts: a typed
	// error's own wording, a revoked session, a read or write that came back
	// with an error.
	severityDanger
)

// The banner's three marks. They are drawn in the severity's ink, the way the
// sidebar's icons are drawn in the text color, so one set of paths serves both
// palettes.
var (
	iconNoticeInfo    = icon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 7.6h.01"/>`)
	iconNoticeDone    = icon(`<circle cx="12" cy="12" r="9"/><path d="m8.5 12.2 2.4 2.4 4.6-4.8"/>`)
	iconNoticeWarning = icon(`<path d="M10.3 3.9 2.4 18a2 2 0 0 0 1.7 3h15.8a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9.5v4"/><path d="M12 17.3h.01"/>`)
	iconNoticeDanger  = icon(`<circle cx="12" cy="12" r="9"/><path d="m14.8 9.2-5.6 5.6"/><path d="m9.2 9.2 5.6 5.6"/>`)
)

// noticeGlyph is the mark a status line leads with: a check for a line that
// reports a completed action, otherwise the severity's own mark.
func noticeGlyph(sev noticeSeverity, ok bool) *ui.SVG {
	if ok {
		return iconNoticeDone
	}
	switch sev {
	case severityDanger:
		return iconNoticeDanger
	case severityWarning:
		return iconNoticeWarning
	default:
		return iconNoticeInfo
	}
}

// plainNotice is a status line built from a localized string that has no kind
// behind it - the warning rows the browsers page composes inline. Its only
// severity signal is whether it reports a success.
type plainNotice struct {
	line string
	ok   bool
}

func (n plainNotice) text() string { return n.line }

func (n plainNotice) severity() noticeSeverity {
	if n.ok {
		return severityInfo
	}
	return severityWarning
}

func (n plainNotice) glyph() *ui.SVG { return noticeGlyph(n.severity(), n.ok) }

// noticeKind names the content of a page's status line. The line keeps the
// kind and its arguments rather than rendered text, so a language change
// re-says it at the next frame, as login_text.go does for the login window.
type noticeKind int

const (
	noticeNone noticeKind = iota
	// noticeFailure shows a typed error's own wording, through errorText.
	noticeFailure

	noticeAccountKeysReadFailed
	noticeAccountHelloCheckFailed
	noticeAccountHelloUnavailable
	noticeAccountSessionRevoked
	noticeAccountSessionUnverified
	noticeAccountSessionUnverifiedNetwork
	noticeAccountSessionUnverifiedRateLimited
	noticeAccountSessionUnverifiedServer
	noticeAccountSessionChecking
	noticeAccountSessionVerified
	noticeAccountSessionReadFailed
	noticeAccountKeySaved
	noticeAccountVerified
	noticeAccountRemoveSessionFailed
	noticeAccountKeyRemoved
	noticeAccountLoggedOut
	noticeAccountLoggedIn
	noticeAccountCancelled

	noticeSettingsAutostartReadFailed
	noticeSettingsHelloCheckFailed
	noticeSettingsHelloAvailable
	noticeSettingsHelloUnavailable
	noticeSettingsAutostartWriteFailed
	noticeSettingsAutostartOn
	noticeSettingsAutostartOff
	noticeSettingsTrayShown
	noticeSettingsTrayHidden

	noticeLogReadFailed
	noticeLogRefreshed
	noticeLogCleared
	noticeLogMissing
	noticeLogOpenFailed
	noticeLogOpened

	noticeBrowsersReadFailed
	noticeBrowsersManifestWriteFailed
	noticeBrowsersManifestWritten
	noticeBrowsersUninstallForeign
	noticeBrowsersManifestDeleteFailed
	noticeBrowsersManifestDeleted
	noticeBrowsersBulk
	noticeBrowsersFirstRun
)

// bulkSummary is the browsers page's one-click install summary, kept whole so
// the note is re-worded, list separators and all, when the language changes.
type bulkSummary struct {
	installed int
	failed    []string
	absent    []string
	left      int
}

// firstRunSummary is the launch-time setup summary, kept whole for the same
// reason as bulkSummary.
type firstRunSummary struct {
	registered []string
	skipped    []string
	failed     []string
	absent     []string
	markErr    error
}

// pageNotice is a page's status line. kind says what it is; args carries what
// its wording interpolates, err the failure errorText maps, and the bulk and
// firstRun summaries the multi-part lines. text is the one place it becomes a
// string, so the language change moves the drawn line with it.
type pageNotice struct {
	kind     noticeKind
	args     []string
	err      error
	ok       bool
	bulk     *bulkSummary
	firstRun *firstRunSummary
}

// failureNotice wraps a typed error as a status line that says its own
// wording, the shape the account page's key actions use.
func failureNotice(err error) pageNotice {
	return pageNotice{kind: noticeFailure, err: err}
}

// severity buckets the kinds by what the line means for the person reading it.
// The transient account checks are warnings because the key is still stored and
// unlocking still works; the revoked session and the typed failures are dangers
// because nothing will change until someone acts; a key that was saved or a
// login that was confirmed is info, the same step as the check that led to it.
func (n pageNotice) severity() noticeSeverity {
	switch n.kind {
	case noticeAccountSessionUnverified, noticeAccountSessionUnverifiedNetwork,
		noticeAccountSessionUnverifiedRateLimited, noticeAccountSessionUnverifiedServer,
		noticeAccountHelloUnavailable,
		noticeSettingsHelloUnavailable,
		noticeLogMissing,
		noticeBrowsersUninstallForeign:
		return severityWarning
	case noticeFailure,
		noticeAccountKeysReadFailed, noticeAccountHelloCheckFailed,
		noticeAccountSessionRevoked, noticeAccountSessionReadFailed,
		noticeAccountRemoveSessionFailed,
		noticeSettingsAutostartReadFailed, noticeSettingsHelloCheckFailed,
		noticeSettingsAutostartWriteFailed,
		noticeLogReadFailed, noticeLogOpenFailed,
		noticeBrowsersReadFailed, noticeBrowsersManifestWriteFailed,
		noticeBrowsersManifestDeleteFailed:
		return severityDanger
	case noticeBrowsersBulk:
		// The one-click summary is a success unless a browser refused, and the
		// summary it keeps is what says which.
		if n.bulk != nil && len(n.bulk.failed) > 0 {
			return severityDanger
		}
		return severityInfo
	case noticeBrowsersFirstRun:
		if n.firstRun != nil && (len(n.firstRun.failed) > 0 || n.firstRun.markErr != nil) {
			return severityDanger
		}
		return severityInfo
	default:
		return severityInfo
	}
}

// glyph is the mark the banner leads with for this notice.
func (n pageNotice) glyph() *ui.SVG { return noticeGlyph(n.severity(), n.ok) }

// text says the notice in the language the app shows, "" when there is none.
func (n pageNotice) text() string {
	switch n.kind {
	case noticeFailure:
		return errorText(n.err)
	case noticeAccountKeysReadFailed:
		return locales.AccountNoticeKeysReadFailed(errorDetail(n.err))
	case noticeAccountHelloCheckFailed:
		return locales.AccountNoticeHelloCheckFailed(errorDetail(n.err))
	case noticeAccountHelloUnavailable:
		return locales.AccountNoticeHelloUnavailable()
	case noticeAccountSessionRevoked:
		return locales.AccountNoticeSessionRevoked()
	case noticeAccountSessionUnverified:
		return locales.AccountNoticeSessionUnverified()
	case noticeAccountSessionUnverifiedNetwork:
		return locales.AccountNoticeSessionUnverifiedNetwork()
	case noticeAccountSessionUnverifiedRateLimited:
		return locales.AccountNoticeSessionUnverifiedRateLimited()
	case noticeAccountSessionUnverifiedServer:
		return locales.AccountNoticeSessionUnverifiedServer()
	case noticeAccountSessionChecking:
		return locales.AccountNoticeSessionChecking()
	case noticeAccountSessionVerified:
		return locales.AccountNoticeSessionVerified()
	case noticeAccountSessionReadFailed:
		return locales.AccountNoticeSessionReadFailed(errorDetail(n.err))
	case noticeAccountKeySaved:
		return locales.AccountNoticeKeySaved()
	case noticeAccountVerified:
		return locales.AccountNoticeVerified(n.args[0])
	case noticeAccountRemoveSessionFailed:
		return locales.AccountNoticeRemoveSessionFailed(errorDetail(n.err))
	case noticeAccountKeyRemoved:
		return locales.AccountNoticeKeyRemoved()
	case noticeAccountLoggedOut:
		return locales.AccountNoticeLoggedOut()
	case noticeAccountLoggedIn:
		return locales.LoginNoticeLoggedIn(n.args[0])
	case noticeAccountCancelled:
		return locales.LoginNoticeCancelled()
	case noticeSettingsAutostartReadFailed:
		return locales.SettingsNoticeAutostartReadFailed(errorDetail(n.err))
	case noticeSettingsHelloCheckFailed:
		return locales.SettingsNoticeHelloCheckFailed(errorDetail(n.err))
	case noticeSettingsHelloAvailable:
		return locales.SettingsNoticeHelloAvailable()
	case noticeSettingsHelloUnavailable:
		return locales.SettingsNoticeHelloUnavailable()
	case noticeSettingsAutostartWriteFailed:
		return locales.SettingsNoticeAutostartWriteFailed(errorDetail(n.err))
	case noticeSettingsAutostartOn:
		return locales.SettingsNoticeAutostartOn()
	case noticeSettingsAutostartOff:
		return locales.SettingsNoticeAutostartOff()
	case noticeSettingsTrayShown:
		return locales.SettingsNoticeTrayShown()
	case noticeSettingsTrayHidden:
		return locales.SettingsNoticeTrayHidden()
	case noticeLogReadFailed:
		return locales.LogNoticeReadFailed(errorDetail(n.err))
	case noticeLogRefreshed:
		return locales.LogNoticeRefreshed()
	case noticeLogCleared:
		return locales.LogNoticeCleared()
	case noticeLogMissing:
		return locales.LogNoticeMissing(n.args[0])
	case noticeLogOpenFailed:
		return locales.LogNoticeOpenFailed(errorDetail(n.err))
	case noticeLogOpened:
		return locales.LogNoticeOpened()
	case noticeBrowsersReadFailed:
		return locales.BrowsersNoticeReadFailed(errorDetail(n.err))
	case noticeBrowsersManifestWriteFailed:
		return locales.BrowsersNoticeManifestWriteFailed(n.args[0], errorDetail(n.err))
	case noticeBrowsersManifestWritten:
		return locales.BrowsersNoticeManifestWritten(n.args[0])
	case noticeBrowsersUninstallForeign:
		return locales.BrowsersNoticeUninstallForeign(n.args[0])
	case noticeBrowsersManifestDeleteFailed:
		return locales.BrowsersNoticeManifestDeleteFailed(n.args[0], errorDetail(n.err))
	case noticeBrowsersManifestDeleted:
		return locales.BrowsersNoticeManifestDeleted(n.args[0])
	case noticeBrowsersBulk:
		return n.bulk.text()
	case noticeBrowsersFirstRun:
		return n.firstRun.text()
	default:
		return ""
	}
}

// errorDetail is the failure's own message for a notice's {detail} slot. A nil
// error has no detail, so the line renders without one rather than panicking.
func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// text renders the bulk install summary: what was written, what failed, the
// browsers this machine does not have, and the takeover rows a cancel left
// untouched (zero once the dialog was confirmed).
func (b *bulkSummary) text() string {
	if b == nil {
		return ""
	}
	parts := []string{locales.BrowsersNoticeInstalled(b.installed)}
	if len(b.failed) > 0 {
		parts = append(parts, locales.BrowsersNoticeFailed(joinNames(b.failed)))
	}
	if len(b.absent) > 0 {
		parts = append(parts, locales.BrowsersNoticeSkippedAbsent(joinNames(b.absent)))
	}
	if b.left > 0 {
		parts = append(parts, locales.BrowsersNoticeSkippedCancelled(b.left))
	}
	return strings.Join(parts, locales.NoticeSeparator())
}

// text renders the first-run summary: what was registered, what was skipped
// because the registration may be another client's, what failed and what is
// not installed, plus a lost marker.
func (f *firstRunSummary) text() string {
	if f == nil {
		return ""
	}
	intro := locales.FirstRunNoticeIntro(len(f.registered))
	if len(f.registered) > 0 {
		intro += locales.FirstRunNoticeNamed(joinNames(f.registered))
	}
	parts := []string{intro}
	if len(f.skipped) > 0 {
		parts = append(parts, locales.FirstRunNoticeSkipped(len(f.skipped), joinNames(f.skipped)))
	}
	if len(f.failed) > 0 {
		parts = append(parts, locales.FirstRunNoticeFailed(joinNames(f.failed)))
	}
	if len(f.absent) > 0 {
		parts = append(parts, locales.FirstRunNoticeAbsent(joinNames(f.absent)))
	}
	if f.markErr != nil {
		parts = append(parts, locales.FirstRunNoticeMarkerError(f.markErr.Error()))
	}
	return strings.Join(parts, locales.NoticeSeparator())
}

// joinNames joins names with the list separator the language uses.
func joinNames(names []string) string {
	return strings.Join(names, locales.ListSeparator())
}
