package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
)

// successTokenBody is the successful grant from login_test.go: its wrapped Key
// unlocks to the 64-byte userKey for master password "password" and salt
// "user@example.com".
const successTokenBody = `{"access_token":"access-1","token_type":"Bearer","Key":"2.EBAQEBAQEBAQEBAQEBAQEA==|lpnnGSmUGNu7ZCJ5Uolx7d9CVUBvEbcJaGOmPYHCx+il7Q6R8TAI/afRlpqIGZtpqu/pArTL27HDSsLvaof54O6vbgQ0GJV4LLaTRQwxr1M=|ap0f2828naJvmZGRUCidFoReX3tUbRxcPCn1bNmi1O4="}`

// twoFactorHarness serves the login endpoints for a flow that is answered with
// one challenge and then resubmitted. submit, when non-nil, decides the
// response to every retry after the first: a zero status means the standard
// success body.
type twoFactorHarness struct {
	client     *Client
	tokenCalls int
	forms      []map[string]string
	emailCalls int
	emailTo    []string
	submit     func(call int, form map[string]string) (status int, body string)
}

func newTwoFactorHarness(t *testing.T, challenge string) *twoFactorHarness {
	t.Helper()
	h := &twoFactorHarness{}
	h.client = newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/accounts/prelogin/password":
			_, _ = w.Write([]byte(`{"KdfSettings":{"KdfType":0,"Iterations":5000},"Salt":"user@example.com"}`))
		case "/identity/connect/token":
			h.tokenCalls++
			if h.tokenCalls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(challenge))
				return
			}
			_ = r.ParseForm()
			form := make(map[string]string, len(r.PostForm))
			for k := range r.PostForm {
				form[k] = r.PostForm.Get(k)
			}
			h.forms = append(h.forms, form)
			if h.submit != nil {
				if status, body := h.submit(h.tokenCalls, form); status != 0 {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(body))
					return
				}
			}
			_, _ = w.Write([]byte(successTokenBody))
		case "/api/two-factor/send-email-login":
			h.emailCalls++
			h.emailTo = append(h.emailTo, r.Header.Get(authEmailHeader))
			_, _ = w.Write([]byte(`{}`))
		case "/api/accounts/profile":
			_, _ = w.Write([]byte(`{"Id":"user-1","Email":"user@example.com","Name":"Test User"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	return h
}

// enterTwoFactor drives Start and the password grant, and returns the flow in
// LoginNeedTwoFactor.
func (h *twoFactorHarness) enterTwoFactor(t *testing.T) *LoginFlow {
	t.Helper()
	flow := NewLoginFlow(h.client)
	if _, err := flow.Start(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	need, err := flow.Continue(context.Background(), LoginInput{Password: "password"})
	if err != nil {
		t.Fatalf("Continue(password): %v", err)
	}
	if need.State != LoginNeedTwoFactor {
		t.Fatalf("state = %s, want need-two-factor", need.State)
	}
	return flow
}

// submitTwoFactor answers the pending challenge through the flow's Continue,
// the way the GUI does.
func submitTwoFactor(t *testing.T, flow *LoginFlow, provider TwoFactorProvider, code string) (LoginResult, error) {
	t.Helper()
	return flow.Continue(context.Background(), LoginInput{TwoFactor: &TwoFactorSubmission{
		Provider: provider,
		Token:    code,
	}})
}

// The offered code methods are answered through Continue's TwoFactor step: the
// provider number and token travel to the server as twoFactorProvider and
// twoFactorToken, and a successful answer lands in LoginDone.
func TestContinueTwoFactorSubmitsProviderAndToken(t *testing.T) {
	cases := []struct {
		name      string
		provider  TwoFactorProvider
		challenge string
		code      string
		wantSend  int
	}{
		{
			name:      "authenticator",
			provider:  TwoFactorAuthenticator,
			challenge: `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`,
			code:      "123456",
		},
		{
			name:      "yubikey",
			provider:  TwoFactorYubiKey,
			challenge: `{"TwoFactorProviders":[3],"TwoFactorProviders2":{"3":{}}}`,
			code:      "ccccccvthubderdlnlghtcvlffjdnlibvkcjtvjrufkc",
		},
		{
			name:      "email",
			provider:  TwoFactorEmail,
			challenge: `{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`,
			code:      "654321",
			wantSend:  1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTwoFactorHarness(t, tc.challenge)
			flow := h.enterTwoFactor(t)
			if tc.wantSend > 0 {
				if _, err := flow.SendEmail(context.Background()); err != nil {
					t.Fatalf("SendEmail: %v", err)
				}
			}

			result, err := submitTwoFactor(t, flow, tc.provider, tc.code)
			if err != nil {
				t.Fatalf("Continue(2FA): %v", err)
			}
			if result.State != LoginDone {
				t.Fatalf("state = %s, want done", result.State)
			}
			if len(result.UserKey) != 64 {
				t.Errorf("UserKey length = %d, want 64", len(result.UserKey))
			}
			if h.tokenCalls != 2 || len(h.forms) != 1 {
				t.Fatalf("token calls = %d, recorded forms = %d, want 2 and 1", h.tokenCalls, len(h.forms))
			}
			form := h.forms[0]
			if got := form["twoFactorProvider"]; got != strconv.Itoa(int(tc.provider)) {
				t.Errorf("twoFactorProvider = %q, want %d", got, tc.provider)
			}
			if got := form["twoFactorToken"]; got != tc.code {
				t.Errorf("twoFactorToken = %q, want %q", got, tc.code)
			}
			if got := form["twoFactorRemember"]; got != "0" {
				t.Errorf("twoFactorRemember = %q, want 0 (remember is off by decision)", got)
			}
			if h.emailCalls != tc.wantSend {
				t.Errorf("send-email-login calls = %d, want %d", h.emailCalls, tc.wantSend)
			}
		})
	}
}

// A rejected code is retryable: the flow rewinds to NeedTwoFactor with the
// challenge kept, and a corrected answer finishes the login.
func TestContinueTwoFactorRetriesAfterRejectedCode(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	h.submit = func(call int, _ map[string]string) (int, string) {
		if call == 2 {
			return http.StatusBadRequest, `{"ErrorModel":{"Message":"Invalid token."}}`
		}
		return 0, ""
	}
	flow := h.enterTwoFactor(t)

	first, err := submitTwoFactor(t, flow, TwoFactorAuthenticator, "123456")
	if err == nil {
		t.Fatal("Continue(2FA) with a rejected code = nil error, want the server error")
	}
	if first.State != LoginNeedTwoFactor {
		t.Errorf("state = %s, want need-two-factor for a retry", first.State)
	}
	if first.Challenge == nil || !first.Challenge.Has(TwoFactorAuthenticator) {
		t.Errorf("Challenge = %+v, want the original challenge kept", first.Challenge)
	}

	second, err := submitTwoFactor(t, flow, TwoFactorAuthenticator, "654321")
	if err != nil {
		t.Fatalf("Continue(2FA retry): %v", err)
	}
	if second.State != LoginDone {
		t.Errorf("state = %s, want done", second.State)
	}
	if h.tokenCalls != 3 {
		t.Errorf("token calls = %d, want 3 (password, rejected retry, accepted retry)", h.tokenCalls)
	}
}

// If the server rejects the first factor and asks for a different one, the new
// challenge replaces the old and the flow stays in NeedTwoFactor.
func TestContinueTwoFactorHandlesServerSwitchingMethod(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	const emailChallenge = `{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`
	h.submit = func(call int, _ map[string]string) (int, string) {
		if call == 2 {
			return http.StatusBadRequest, emailChallenge
		}
		return 0, ""
	}
	flow := h.enterTwoFactor(t)

	switched, err := submitTwoFactor(t, flow, TwoFactorAuthenticator, "123456")
	if err != nil {
		t.Fatalf("Continue(authenticator): %v", err)
	}
	if switched.State != LoginNeedTwoFactor {
		t.Fatalf("state = %s, want need-two-factor after the switch", switched.State)
	}
	if switched.Challenge == nil || !switched.Challenge.Has(TwoFactorEmail) {
		t.Fatalf("Challenge = %+v, want the email method offered", switched.Challenge)
	}
	if switched.Challenge.Email != "ada@example.com" {
		t.Errorf("Challenge.Email = %q", switched.Challenge.Email)
	}

	if _, err := flow.SendEmail(context.Background()); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	done, err := submitTwoFactor(t, flow, TwoFactorEmail, "654321")
	if err != nil {
		t.Fatalf("Continue(email): %v", err)
	}
	if done.State != LoginDone {
		t.Errorf("state = %s, want done", done.State)
	}
}

// Continue needs a submission in NeedTwoFactor; from any other state the two
// factor answer is refused.
func TestContinueTwoFactorRequiresASubmission(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	flow := h.enterTwoFactor(t)
	if _, err := flow.Continue(context.Background(), LoginInput{}); err == nil {
		t.Fatal("Continue() without a submission = nil error, want a state error")
	}

	flow2 := NewLoginFlow(h.client)
	if _, err := flow2.Continue(context.Background(), LoginInput{TwoFactor: &TwoFactorSubmission{Provider: TwoFactorAuthenticator, Token: "123456"}}); err == nil {
		t.Fatal("Continue(2FA) before Start = nil error, want a state error")
	}
}

func TestChallengeMethodsClassifyProviders(t *testing.T) {
	ch := &TwoFactorChallenge{
		Providers: []TwoFactorProvider{
			TwoFactorAuthenticator, TwoFactorEmail, TwoFactorDuo, TwoFactorYubiKey,
			TwoFactorRemember, TwoFactorOrganizationDuo, TwoFactorWebAuthn, TwoFactorRecoveryCode, TwoFactorProvider(99),
		},
		Email:       "ada@example.com",
		DuoURL:      "https://duo.example/auth",
		WebAuthnURL: "https://vault.example/webauthn-connector.html",
	}
	want := []struct {
		provider    TwoFactorProvider
		interaction ChallengeInteraction
		email       string
		browserURL  string
	}{
		{TwoFactorAuthenticator, InteractionCode, "", ""},
		{TwoFactorEmail, InteractionCode, "ada@example.com", ""},
		{TwoFactorDuo, InteractionBrowser, "", "https://duo.example/auth"},
		{TwoFactorYubiKey, InteractionCode, "", ""},
		{TwoFactorRemember, InteractionUnsupported, "", ""},
		{TwoFactorOrganizationDuo, InteractionBrowser, "", "https://duo.example/auth"},
		{TwoFactorWebAuthn, InteractionBrowser, "", "https://vault.example/webauthn-connector.html"},
		{TwoFactorRecoveryCode, InteractionUnsupported, "", ""},
		{TwoFactorProvider(99), InteractionUnsupported, "", ""},
	}
	got := ch.Methods()
	if len(got) != len(want) {
		t.Fatalf("Methods() = %d entries, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Provider != w.provider || got[i].Interaction != w.interaction || got[i].Email != w.email || got[i].BrowserURL != w.browserURL {
			t.Errorf("Methods()[%d] = %+v, want provider %s interaction %s email %q url %q",
				i, got[i], w.provider, w.interaction, w.email, w.browserURL)
		}
	}

	method, ok := ch.Method(TwoFactorYubiKey)
	if !ok || method.Interaction != InteractionCode {
		t.Errorf("Method(yubikey) = %+v, %v", method, ok)
	}
	// An offered but unknown provider still resolves, as unsupported.
	unknown, ok := ch.Method(TwoFactorProvider(99))
	if !ok || unknown.Interaction != InteractionUnsupported {
		t.Errorf("Method(99) = %+v, %v, want an unsupported method", unknown, ok)
	}
}

func TestChallengeMethodsOnNilChallenge(t *testing.T) {
	var ch *TwoFactorChallenge
	if ch.Methods() != nil {
		t.Error("Methods() on a nil challenge = non-nil")
	}
	if _, ok := ch.Method(TwoFactorAuthenticator); ok {
		t.Error("Method() on a nil challenge = ok")
	}
}

// SendEmail is the pull-model send. The send happens in NeedTwoFactor only,
// marks the challenge so a resend is a no-op, and a failed send stays
// retryable.
func TestSendEmailSendsOnceInNeedTwoFactor(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`)
	flow := h.enterTwoFactor(t)

	result, err := flow.SendEmail(context.Background())
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if result.State != LoginNeedTwoFactor {
		t.Errorf("state = %s, want need-two-factor", result.State)
	}
	if result.Challenge != flow.challenge || result.Challenge == nil {
		t.Errorf("Challenge = %+v, want the flow's live challenge", result.Challenge)
	}
	if !result.Challenge.EmailSent {
		t.Error("Challenge.EmailSent = false, want true after the send")
	}
	if h.emailCalls != 1 {
		t.Errorf("send-email-login calls = %d, want 1", h.emailCalls)
	}

	// Second call: the challenge is already marked, so no network request.
	again, err := flow.SendEmail(context.Background())
	if err != nil {
		t.Fatalf("SendEmail(resend): %v", err)
	}
	if again.State != LoginNeedTwoFactor {
		t.Errorf("resend state = %s, want need-two-factor", again.State)
	}
	if h.emailCalls != 1 {
		t.Errorf("send-email-login calls after resend = %d, want 1 (single-send guarantee)", h.emailCalls)
	}
	if !again.Challenge.EmailSent {
		t.Error("resend Challenge.EmailSent = false, want still true")
	}
}

// The send request carries the entered login address, not the masked
// destination the challenge reports (official TwoFactorEmailLoginRequest
// takes loginStrategyService.getEmail()).
func TestSendEmailUsesTheLoginAddress(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`)
	flow := h.enterTwoFactor(t)

	if _, err := flow.SendEmail(context.Background()); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	wantHeader := "dXNlckBleGFtcGxlLmNvbQ" // base64url("user@example.com")
	if len(h.emailTo) != 1 || h.emailTo[0] != wantHeader {
		t.Errorf("Auth-Email header = %v, want [%s]", h.emailTo, wantHeader)
	}
}

// A failed send must not mark the challenge: the user can just retry.
func TestSendEmailKeepsRetryableAfterNetworkFailure(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`)
	emailAttempts := 0
	h.client = newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/accounts/prelogin/password":
			_, _ = w.Write([]byte(`{"KdfSettings":{"KdfType":0,"Iterations":5000},"Salt":"user@example.com"}`))
		case "/identity/connect/token":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`))
		case "/api/two-factor/send-email-login":
			emailAttempts++
			if emailAttempts == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"Rate limit exceeded."}`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	flow := NewLoginFlow(h.client)
	ctx := context.Background()
	if _, err := flow.Start(ctx, "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	need, err := flow.Continue(ctx, LoginInput{Password: "password"})
	if err != nil {
		t.Fatalf("Continue(password): %v", err)
	}
	if need.State != LoginNeedTwoFactor {
		t.Fatalf("state = %s, want need-two-factor", need.State)
	}

	first, err := flow.SendEmail(ctx)
	if err == nil {
		t.Fatal("SendEmail() with a failed send = nil error, want the server error")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("SendEmail error = %v, want ErrRateLimited in the chain", err)
	}
	if first.State != LoginNeedTwoFactor {
		t.Errorf("state = %s, want need-two-factor (retryable)", first.State)
	}
	if first.Challenge.EmailSent {
		t.Error("Challenge.EmailSent = true after a failed send, want false for a retry")
	}

	second, err := flow.SendEmail(ctx)
	if err != nil {
		t.Fatalf("SendEmail(retry): %v", err)
	}
	if !second.Challenge.EmailSent {
		t.Error("Challenge.EmailSent = false after the retry, want true")
	}
	if emailAttempts != 2 {
		t.Errorf("send-email-login calls = %d, want 2 (failed send + retry)", emailAttempts)
	}
}

// SendEmail is a NeedTwoFactor action: before the password grant and after
// login it is a state error, with no network request.
func TestSendEmailRejectsWrongState(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	flow := NewLoginFlow(h.client)
	ctx := context.Background()

	if _, err := flow.SendEmail(ctx); err == nil {
		t.Fatal("SendEmail() before Start = nil error, want a state error")
	}
	if _, err := flow.Start(ctx, "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := flow.SendEmail(ctx); err == nil {
		t.Fatal("SendEmail() in need-password = nil error, want a state error")
	}
	if h.emailCalls != 0 {
		t.Errorf("send-email-login calls = %d, want 0 (guarded before the grant)", h.emailCalls)
	}

	// Drive to LoginDone and confirm Done is rejected too.
	if _, err := flow.Continue(ctx, LoginInput{Password: "password"}); err != nil {
		t.Fatalf("Continue(password): %v", err)
	}
	if _, err := flow.Continue(ctx, LoginInput{TwoFactor: &TwoFactorSubmission{Provider: TwoFactorAuthenticator, Token: "123456"}}); err != nil {
		t.Fatalf("Continue(2FA): %v", err)
	}
	if _, err := flow.SendEmail(ctx); err == nil {
		t.Fatal("SendEmail() in done = nil error, want a state error")
	}
	if h.emailCalls != 0 {
		t.Errorf("send-email-login calls = %d, want 0 (no request outside NeedTwoFactor)", h.emailCalls)
	}
}

func TestChallengeInteractionString(t *testing.T) {
	cases := map[ChallengeInteraction]string{
		InteractionCode:         "code",
		InteractionBrowser:      "browser",
		InteractionUnsupported:  "unsupported",
		ChallengeInteraction(9): "unknown",
	}
	for interaction, want := range cases {
		if got := interaction.String(); got != want {
			t.Errorf("ChallengeInteraction(%d).String() = %q, want %q", interaction, got, want)
		}
	}
}
