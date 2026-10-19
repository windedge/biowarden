package ui

import (
	"strings"

	"github.com/windedge/biowarden/locales"
)

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
