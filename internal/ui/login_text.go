package ui

import (
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// errorText says a typed failure in the language the app shows. The backends
// report sentinels; this is the one place they become wording, so a language
// change re-says an error the status line or the field still holds instead of
// leaving the previous language on screen.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	// A sentinel carrying a detail interpolates it.
	var le *loginError
	if errors.As(err, &le) {
		switch le.sentinel {
		case errServerAddress:
			return locales.LoginErrorServerAddress(le.detail)
		case errLoginFlowStart:
			return locales.LoginErrorFlowStart(le.detail)
		case errLoginFlowState:
			return locales.LoginErrorFlowState(le.detail)
		case errSaveKey:
			return locales.LoginErrorSaveKey(le.detail)
		case errProviderNotOffered:
			return locales.LoginErrorProviderNotOffered(le.detail)
		case errSessionPersist:
			return locales.LoginErrorSessionPersist(le.detail)
		case errSessionRead:
			return locales.AccountNoticeSessionReadFailed(le.detail)
		case errHelloCallFailed:
			return locales.HelloErrorCallFailed(le.detail)
		case errHelloUnknown:
			return locales.HelloErrorUnknown(le.detail)
		case errKeyNotBase64:
			return locales.KeyErrorNotBase64(le.detail)
		case errRemoveKey:
			return locales.AccountErrorRemoveKey(le.detail)
		case errClearSession:
			return locales.AccountErrorClearSession(le.detail)
		}
	}
	switch {
	case errors.Is(err, errWrongCredentials):
		return locales.LoginErrorBadCredentials()
	case errors.Is(err, errLoginForbidden):
		return locales.LoginErrorForbidden()
	case errors.Is(err, errCaptchaRequired):
		return locales.LoginErrorCaptcha()
	case errors.Is(err, errNetworkUnreachable):
		return locales.LoginErrorNetwork()
	case errors.Is(err, errServerFailure):
		return locales.LoginErrorServer()
	case errors.Is(err, errNoTwoFactorMethod):
		return locales.LoginErrorNoTwoFactorMethod()
	case errors.Is(err, errTwoFactorUnsupported):
		return locales.LoginUnsupportedTitle()
	case errors.Is(err, errNoTwoFactorChallenge):
		return locales.LoginErrorNoChallenge()
	case errors.Is(err, errTwoFactorEmptyCode):
		return locales.LoginErrorEmptyCode()
	case errors.Is(err, errEmailMethodMissing):
		return locales.LoginErrorEmailMethodMissing()
	case errors.Is(err, errHelloCanceled):
		return locales.HelloErrorCanceled()
	case errors.Is(err, errHelloUnavailable):
		return locales.HelloErrorUnavailable()
	case errors.Is(err, errHelloLocked):
		return locales.HelloErrorLocked()
	case errors.Is(err, errHelloBusy):
		return locales.HelloErrorBusy()
	default:
		// A failure the UI has no wording for (or a test's own error) says its
		// own message; the backend never writes a user-facing one.
		return err.Error()
	}
}

// loginNoticeKind names the content of the login window's status line. The line
// keeps the kind and its arguments rather than rendered text, so a language
// change re-says it at the next frame.
type loginNoticeKind int

const (
	loginNoticeNone loginNoticeKind = iota
	loginNoticeBadServer
	loginNoticeTwoFactor
	loginNoticeSigningIn
	loginNoticeCodeRejected
	loginNoticeVerifying
	loginNoticeEmailSent
	loginNoticeEmailSentTo
	loginNoticeBusy
	loginNoticeFailure
)

// loginNotice is the login window's status line. kind says what it is; args
// carries what its wording interpolates, and failure the error errorText
// maps. text is the one place it becomes a string, and the height target and
// the drawing both go through it.
type loginNotice struct {
	kind    loginNoticeKind
	args    []string
	failure error
	ok      bool
}

// arg returns args[i], or "" when a caller built the notice without it, so a
// missing argument renders a harmless sentence instead of panicking.
func (n loginNotice) arg(i int) string {
	if i < 0 || i >= len(n.args) {
		return ""
	}
	return n.args[i]
}

// severity buckets the login window's kinds the same way the pages' notices
// are bucketed: a refusal or a rejected code is a danger, a bad address is a
// warning to correct, and everything the window says about its own progress is
// info.
func (n loginNotice) severity() noticeSeverity {
	switch n.kind {
	case loginNoticeBadServer:
		return severityWarning
	case loginNoticeCodeRejected, loginNoticeFailure:
		return severityDanger
	default:
		return severityInfo
	}
}

// glyph is the mark the banner leads with for this notice.
func (n loginNotice) glyph() *ui.SVG { return noticeGlyph(n.severity(), n.ok) }

// text says the notice in the language the app shows, "" when there is none.
func (n loginNotice) text() string {
	switch n.kind {
	case loginNoticeBadServer:
		return locales.LoginNoticeBadServer()
	case loginNoticeTwoFactor:
		return locales.LoginNoticeTwoFactor()
	case loginNoticeSigningIn:
		return locales.LoginBusySigningIn()
	case loginNoticeCodeRejected:
		return locales.LoginNoticeCodeRejected()
	case loginNoticeVerifying:
		return locales.LoginNoticeVerifying()
	case loginNoticeEmailSent:
		return locales.LoginNoticeEmailSent()
	case loginNoticeEmailSentTo:
		return locales.LoginNoticeEmailSentTo(n.arg(0))
	case loginNoticeBusy:
		return locales.LoginNoticeBusy()
	case loginNoticeFailure:
		return errorText(n.failure)
	default:
		return ""
	}
}
