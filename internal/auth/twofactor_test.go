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

// fakeSolver is a scripted TwoFactorSolver. code and err are keyed by provider
// so one solver can stand in for several methods.
type fakeSolver struct {
	code           map[TwoFactorProvider]string
	err            map[TwoFactorProvider]error
	sends          int
	emailSendTwice bool
	browserURL     string
}

func (f *fakeSolver) value(p TwoFactorProvider) (string, error) {
	if err := f.err[p]; err != nil {
		return "", err
	}
	return f.code[p], nil
}

func (f *fakeSolver) TOTP(context.Context) (string, error) { return f.value(TwoFactorAuthenticator) }

func (f *fakeSolver) YubiKey(context.Context) (string, error) { return f.value(TwoFactorYubiKey) }

func (f *fakeSolver) EmailCode(ctx context.Context, send func(context.Context) error) (string, error) {
	f.sends++
	if err := f.err[TwoFactorEmail]; err != nil {
		return "", err
	}
	once := 1
	if f.emailSendTwice {
		once = 2
	}
	for i := 0; i < once; i++ {
		if err := send(ctx); err != nil {
			return "", err
		}
	}
	return f.value(TwoFactorEmail)
}

func (f *fakeSolver) OpenBrowser(_ context.Context, url string) (string, error) {
	f.browserURL = url
	return f.value(TwoFactorDuo)
}

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
func (h *twoFactorHarness) enterTwoFactor(t *testing.T, solver TwoFactorSolver) *LoginFlow {
	t.Helper()
	flow := NewLoginFlow(h.client, solver, nil, nil)
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

func TestSolveTwoFactorSubmitsProviderAndToken(t *testing.T) {
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
		{
			name:      "duo",
			provider:  TwoFactorDuo,
			challenge: `{"TwoFactorProviders":[2],"TwoFactorProviders2":{"2":{"AuthUrl":"https://api-abc.duosecurity.com/oauth/v1/authorize?x=1"}}}`,
			code:      "duo-code|duo-state",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTwoFactorHarness(t, tc.challenge)
			solver := &fakeSolver{code: map[TwoFactorProvider]string{tc.provider: tc.code}}
			flow := h.enterTwoFactor(t, solver)

			result, err := flow.SolveTwoFactor(context.Background(), tc.provider)
			if err != nil {
				t.Fatalf("SolveTwoFactor: %v", err)
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

func TestSolveTwoFactorDuoOpensTheAuthURL(t *testing.T) {
	const authURL = "https://api-abc.duosecurity.com/oauth/v1/authorize?x=1"
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[2],"TwoFactorProviders2":{"2":{"AuthUrl":"`+authURL+`"}}}`)
	solver := &fakeSolver{code: map[TwoFactorProvider]string{TwoFactorDuo: "code|state"}}
	flow := h.enterTwoFactor(t, solver)

	if _, err := flow.SolveTwoFactor(context.Background(), TwoFactorDuo); err != nil {
		t.Fatalf("SolveTwoFactor: %v", err)
	}
	if solver.browserURL != authURL {
		t.Errorf("OpenBrowser URL = %q, want %q", solver.browserURL, authURL)
	}
}

// The email method steps the user: the flow sends the mail once and only then
// collects the code. A solver that calls send twice must not send twice.
func TestSolveTwoFactorEmailSendsOnce(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`)
	solver := &fakeSolver{
		code:           map[TwoFactorProvider]string{TwoFactorEmail: "654321"},
		emailSendTwice: true,
	}
	flow := NewLoginFlow(h.client, solver, nil, nil)
	if _, err := flow.Start(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	need, err := flow.Continue(context.Background(), LoginInput{Password: "password"})
	if err != nil {
		t.Fatalf("Continue(password): %v", err)
	}
	if _, err := flow.SolveTwoFactor(context.Background(), TwoFactorEmail); err != nil {
		t.Fatalf("SolveTwoFactor: %v", err)
	}
	if solver.sends != 1 {
		t.Errorf("solver send calls = %d, want 1", solver.sends)
	}
	if h.emailCalls != 1 {
		t.Errorf("send-email-login calls = %d, want 1", h.emailCalls)
	}
	if !need.Challenge.EmailSent {
		t.Error("Challenge.EmailSent = false, want true after the send")
	}
	// The send request carries the entered login address, not the masked
	// destination the challenge reports (official TwoFactorEmailLoginRequest
	// takes loginStrategyService.getEmail()).
	wantHeader := "dXNlckBleGFtcGxlLmNvbQ" // base64url("user@example.com")
	if len(h.emailTo) != 1 || h.emailTo[0] != wantHeader {
		t.Errorf("Auth-Email header = %v, want [%s]", h.emailTo, wantHeader)
	}
}

// A rejected code is retryable: the flow keeps the challenge and the user can
// submit again.
func TestSolveTwoFactorRetriesAfterRejectedCode(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	h.submit = func(call int, _ map[string]string) (int, string) {
		if call == 2 {
			return http.StatusBadRequest, `{"ErrorModel":{"Message":"Invalid token."}}`
		}
		return 0, ""
	}
	solver := &fakeSolver{code: map[TwoFactorProvider]string{TwoFactorAuthenticator: "123456"}}
	flow := h.enterTwoFactor(t, solver)

	first, err := flow.SolveTwoFactor(context.Background(), TwoFactorAuthenticator)
	if err == nil {
		t.Fatal("SolveTwoFactor() with a rejected code = nil error, want the server error")
	}
	if first.State != LoginNeedTwoFactor {
		t.Errorf("state = %s, want need-two-factor for a retry", first.State)
	}
	if first.Challenge == nil || !first.Challenge.Has(TwoFactorAuthenticator) {
		t.Errorf("Challenge = %+v, want the original challenge kept", first.Challenge)
	}

	second, err := flow.SolveTwoFactor(context.Background(), TwoFactorAuthenticator)
	if err != nil {
		t.Fatalf("SolveTwoFactor(retry): %v", err)
	}
	if second.State != LoginDone {
		t.Errorf("state = %s, want done", second.State)
	}
	if h.tokenCalls != 3 {
		t.Errorf("token calls = %d, want 3 (password, rejected retry, accepted retry)", h.tokenCalls)
	}
}

// A solver that times out or is cancelled is a local failure: the grant is not
// resubmitted and the challenge stays answerable.
func TestSolveTwoFactorKeepsChallengeOnSolverFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"timeout", context.DeadlineExceeded},
		{"cancelled", context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
			solver := &fakeSolver{err: map[TwoFactorProvider]error{TwoFactorAuthenticator: tc.err}}
			flow := h.enterTwoFactor(t, solver)

			result, err := flow.SolveTwoFactor(context.Background(), TwoFactorAuthenticator)
			if !errors.Is(err, tc.err) {
				t.Fatalf("SolveTwoFactor error = %v, want %v", err, tc.err)
			}
			if result.State != LoginNeedTwoFactor {
				t.Errorf("state = %s, want need-two-factor", result.State)
			}
			if h.tokenCalls != 1 {
				t.Errorf("token calls = %d, want 1 (no resubmit on a solver failure)", h.tokenCalls)
			}
		})
	}
}

func TestSolveTwoFactorRejectsUnsupportedMethod(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[8],"TwoFactorProviders2":{"8":{}}}`)
	flow := h.enterTwoFactor(t, &fakeSolver{})

	result, err := flow.SolveTwoFactor(context.Background(), TwoFactorRecoveryCode)
	if !errors.Is(err, ErrTwoFactorUnsupported) {
		t.Fatalf("SolveTwoFactor error = %v, want ErrTwoFactorUnsupported", err)
	}
	if result.State != LoginNeedTwoFactor {
		t.Errorf("state = %s, want need-two-factor", result.State)
	}
}

func TestSolveTwoFactorRejectsUnOfferedMethod(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	flow := h.enterTwoFactor(t, &fakeSolver{})

	if _, err := flow.SolveTwoFactor(context.Background(), TwoFactorYubiKey); !errors.Is(err, ErrTwoFactorUnsupported) {
		t.Fatalf("SolveTwoFactor error = %v, want ErrTwoFactorUnsupported", err)
	}
}

// WebAuthn is a browser method whose connector URL is not built yet, so it
// must not reach the solver and must stay retryable.
func TestSolveTwoFactorWebAuthnWithoutConnectorURLIsUnsupported(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[7],"TwoFactorProviders2":{"7":{"challenge":"abc"}}}`)
	solver := &fakeSolver{}
	flow := h.enterTwoFactor(t, solver)

	result, err := flow.SolveTwoFactor(context.Background(), TwoFactorWebAuthn)
	if !errors.Is(err, ErrTwoFactorUnsupported) {
		t.Fatalf("SolveTwoFactor error = %v, want ErrTwoFactorUnsupported", err)
	}
	if result.State != LoginNeedTwoFactor {
		t.Errorf("state = %s, want need-two-factor", result.State)
	}
	if solver.browserURL != "" {
		t.Errorf("OpenBrowser was called with %q, want no call without a URL", solver.browserURL)
	}
}

// If the server rejects the first factor and asks for a different one, the new
// challenge replaces the old and the flow stays in NeedTwoFactor.
func TestSolveTwoFactorHandlesServerSwitchingMethod(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	const emailChallenge = `{"TwoFactorProviders":[1],"TwoFactorProviders2":{"1":{"Email":"ada@example.com"}},"Email":"ada@example.com"}`
	h.submit = func(call int, _ map[string]string) (int, string) {
		if call == 2 {
			return http.StatusBadRequest, emailChallenge
		}
		return 0, ""
	}
	solver := &fakeSolver{
		code: map[TwoFactorProvider]string{
			TwoFactorAuthenticator: "123456",
			TwoFactorEmail:         "654321",
		},
	}
	flow := h.enterTwoFactor(t, solver)

	switched, err := flow.SolveTwoFactor(context.Background(), TwoFactorAuthenticator)
	if err != nil {
		t.Fatalf("SolveTwoFactor(authenticator): %v", err)
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

	done, err := flow.SolveTwoFactor(context.Background(), TwoFactorEmail)
	if err != nil {
		t.Fatalf("SolveTwoFactor(email): %v", err)
	}
	if done.State != LoginDone {
		t.Errorf("state = %s, want done", done.State)
	}
}

func TestSolveTwoFactorRequiresAChallenge(t *testing.T) {
	h := newTwoFactorHarness(t, `{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`)
	flow := NewLoginFlow(h.client, &fakeSolver{}, nil, nil)
	if _, err := flow.SolveTwoFactor(context.Background(), TwoFactorAuthenticator); err == nil {
		t.Fatal("SolveTwoFactor() before a challenge = nil error, want a state error")
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
