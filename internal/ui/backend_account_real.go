package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
	"github.com/windedge/biowarden/locales"
)

// deviceID is the deviceIdentifier biowarden sends to Bitwarden. The official
// desktop client uses its appId GUID; a fixed identifier keeps the server's
// device list stable across runs, which is what "remember this device" needs.
// Deriving a real per-install id is a later ticket.
const deviceID = "biowarden-windows"

// The backend reports failures as typed sentinels, never as user-facing text:
// login_text.go maps each to wording in the language the app shows.
var (
	// errTwoFactorUnsupported reports a second factor biowarden has no path
	// for: Duo (providers 2 and 6) and WebAuthn (7) complete through a browser
	// hand-off whose callback the map decision deferred, so the login window
	// points at the account page's extraction script instead.
	errTwoFactorUnsupported = errors.New("ui: two-factor method is not supported")
	// errNoTwoFactorChallenge reports a 2FA submission with nothing parked to
	// answer, which a stale window can reach after the attempt was abandoned.
	errNoTwoFactorChallenge = errors.New("ui: no pending two-factor challenge")
	// errTwoFactorEmptyCode reports a 2FA submission without a code. The window
	// disables its button on an empty field; this is the guard behind it.
	errTwoFactorEmptyCode = errors.New("ui: empty two-factor code")
	// The auth failures friendlyAuthError maps to, named for the wording the
	// login window shows.
	errWrongCredentials   = errors.New("ui: wrong email or master password")
	errLoginForbidden     = errors.New("ui: login forbidden")
	errCaptchaRequired    = errors.New("ui: captcha required")
	errNetworkUnreachable = errors.New("ui: the server is unreachable")
	errServerFailure      = errors.New("ui: the server returned an error")
	errNoTwoFactorMethod  = errors.New("ui: no usable two-factor method")
	// The failures below carry a detail their wording interpolates, through
	// loginError.
	errServerAddress      = errors.New("ui: invalid server address")
	errLoginFlowStart     = errors.New("ui: unexpected login flow start state")
	errLoginFlowState     = errors.New("ui: login did not finish")
	errSaveKey            = errors.New("ui: saving the key failed")
	errProviderNotOffered = errors.New("ui: provider not offered")
	errEmailMethodMissing = errors.New("ui: email code method missing")
	errSessionPersist     = errors.New("ui: login session could not be persisted")
	errSessionRead        = errors.New("ui: login session could not be read")
	errRemoveKey          = errors.New("ui: deleting the key failed")
	errClearSession       = errors.New("ui: clearing the login session failed")
)

// loginError wraps a sentinel with the detail its wording interpolates, such
// as the validator's own message for a bad server address. Unwrap keeps
// errors.Is(err, sentinel) working for the UI's mapping.
type loginError struct {
	sentinel error
	detail   string
}

// newLoginError returns sentinel with detail, said as a loginError.
func newLoginError(sentinel error, detail any) error {
	return &loginError{sentinel: sentinel, detail: fmt.Sprint(detail)}
}

func (e *loginError) Error() string { return e.sentinel.Error() + ": " + e.detail }
func (e *loginError) Unwrap() error { return e.sentinel }

// errEmailCodePending reports that the send-email-login request went out but
// no code was submitted with it. It is how the send-code button drives auth's
// email preparation without a token: the flow keeps the challenge and the
// answer is nothing to submit yet.
var errEmailCodePending = errors.New("ui: email code requested")

// accountAuth is the slice of auth.LoginFlow the account backend drives: one
// start with the email, one password step, then any second factor. It is
// defined in local terms so tests (and any future transport) can stand in
// without a network.
type accountAuth interface {
	start(ctx context.Context, email string) error
	password(ctx context.Context, password string) (authStep, error)
	// solveTwoFactor submits one token for the offered provider and returns
	// the next step: done with the account key, or a fresh challenge when the
	// server rejected the token or asks for another method.
	solveTwoFactor(ctx context.Context, provider int, token string) (authStep, error)
	// sendTwoFactorEmail runs the email method's preparation request. The
	// flow keeps the request single-shot, so a repeat call is a no-op.
	sendTwoFactorEmail(ctx context.Context) error
}

// authStep is one step's outcome: either the flow finished with the account
// key, or it stopped for a second factor. refreshToken is the grant's refresh
// token on a finished login; it is the one secret that outlives the run (in the
// credential store), and the key write itself still needs Hello.
type authStep struct {
	needs2FA     bool
	challenge    *twoFactorPrompt
	userKey      []byte
	account      secrets.Account
	refreshToken string
}

// accountAuthFactory builds one login attempt's flow for a server URL.
type accountAuthFactory func(serverURL string) (accountAuth, error)

// realAccount is the accountBackend on the real seams: internal/auth performs
// the login, Windows Hello gates the write, and the Credential Manager stores
// the key. The access and refresh tokens stay inside the flow and are dropped
// as soon as the key is derived, and the master password is never kept: a key
// that has to be re-derived goes through a fresh sign-in in the login window.
type realAccount struct {
	store    secrets.Store
	verifier hello.Verifier
	newFlow  accountAuthFactory
	// sessions persists the login across restarts; refresh verifies a persisted
	// refresh token at startup. Both are seams so tests never touch the
	// credential store or the network.
	sessions sessionStore
	refresh  sessionRefresher
	// prefs remembers the address of the last successful login, so a signed-out
	// self-hosted user gets it back in the form. It is a seam like the two
	// above: the default forgets, and only the production wiring writes the
	// registry.
	prefs serverPref

	mu   sync.Mutex
	sess accountSession
	// lastLoggedIn is the session to fall back to when a pending login is
	// abandoned: cancelling a fresh sign-in returns to the account that was
	// already signed in, cancelling a first login lands back on signed out.
	lastLoggedIn accountSession
	// pending is the attempt waiting for a second factor. The login window
	// answers it through submit2FA; cancelLogin and a finished login drop it.
	pending *pendingTwoFactor
}

// pendingTwoFactor is a login attempt parked on a challenge. The flow holds
// the derived master key and the password hash for the retry; the plaintext
// password is not kept here, so a cancelled attempt leaves nothing behind.
type pendingTwoFactor struct {
	auth      accountAuth
	serverURL string
	email     string
	challenge *twoFactorPrompt
	// emailSent records that the code email already went out, so a second
	// press of the button does not ask for another one.
	emailSent bool
}

func newRealAccount(store secrets.Store, verifier hello.Verifier, factory accountAuthFactory) *realAccount {
	a := &realAccount{
		store: store, verifier: verifier, newFlow: factory,
		sessions: nopSessionStore{},
		refresh:  newSessionRefresher(),
		prefs:    nopServerPref{},
	}
	a.sess = accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
	a.lastLoggedIn = a.sess
	return a
}

// newRealAccountWith is newRealAccount with the persisted-login seams wired.
// The production GUI uses it so a restart can verify and restore the login;
// the tests that do not exercise persistence keep the no-op defaults.
func newRealAccountWith(store secrets.Store, verifier hello.Verifier, factory accountAuthFactory, sessions sessionStore, refresh sessionRefresher) *realAccount {
	a := newRealAccount(store, verifier, factory)
	a.sessions = sessions
	if refresh != nil {
		a.refresh = refresh
	}
	return a
}

// newRealAccountState wires the account page onto the real store, verifier and
// auth flow; newAccountState in state_account.go calls it. The account card and
// the login path share one store instance so their views cannot diverge.
func newRealAccountState() accountState {
	store := secrets.New()
	verifier := hello.New()
	sessions := newWincredSessionStore(secrets.SessionPrefix)
	prefs := newServerPref()
	account := newRealAccountWith(store, verifier, newAuthFlowFactory(), sessions, newSessionRefresher())
	account.prefs = prefs
	s := newAccountStateWith(newRealKeysWith(store, verifier), account)
	s.prefs = prefs
	return s
}

// newAuthFlowFactory builds real login flows through internal/auth. The
// verifier and store are nil because auth never calls them: this backend asks
// for consent and writes the key itself, in that order.
func newAuthFlowFactory() accountAuthFactory {
	return func(serverURL string) (accountAuth, error) {
		cfg, err := auth.NewServerConfig(serverURL)
		if err != nil {
			return nil, newLoginError(errServerAddress, err)
		}
		client := auth.NewClient(cfg, deviceID)
		return newAuthFlowAdapter(client), nil
	}
}

// authFlowAdapter turns auth.LoginFlow into the local accountAuth vocabulary.
type authFlowAdapter struct {
	flow   *auth.LoginFlow
	solver *flowSolver
}

func newAuthFlowAdapter(client *auth.Client) *authFlowAdapter {
	solver := &flowSolver{}
	return &authFlowAdapter{
		flow:   auth.NewLoginFlow(client, solver, nil, nil),
		solver: solver,
	}
}

func (a *authFlowAdapter) start(ctx context.Context, email string) error {
	res, err := a.flow.Start(ctx, email)
	if err != nil {
		return friendlyAuthError(err)
	}
	if res.State != auth.LoginNeedPassword {
		return newLoginError(errLoginFlowStart, res.State)
	}
	return nil
}

func (a *authFlowAdapter) password(ctx context.Context, password string) (authStep, error) {
	res, err := a.flow.Continue(ctx, auth.LoginInput{Password: password})
	if err != nil {
		return authStep{}, friendlyAuthError(err)
	}
	return stepFromResult(res)
}

// solveTwoFactor hands the flow the token the login window collected.
// auth's solver interface is written for a caller that blocks on a human; the
// window already has the answer, so the solver returns it immediately.
func (a *authFlowAdapter) solveTwoFactor(ctx context.Context, provider int, token string) (authStep, error) {
	a.solver.set(token, nil)
	res, err := a.flow.SolveTwoFactor(ctx, auth.TwoFactorProvider(provider))
	if err != nil {
		return authStep{}, friendlyAuthError(err)
	}
	return stepFromResult(res)
}

// sendTwoFactorEmail runs the email method's preparation with no code to
// submit: the request goes out, the flow stays on the challenge, and the
// pending sentinel says so.
func (a *authFlowAdapter) sendTwoFactorEmail(ctx context.Context) error {
	a.solver.set("", errEmailCodePending)
	if _, err := a.flow.SolveTwoFactor(ctx, auth.TwoFactorEmail); err != nil && !errors.Is(err, errEmailCodePending) {
		return friendlyAuthError(err)
	}
	return nil
}

// flowSolver answers one SolveTwoFactor call with a token the caller already
// collected. It is the seam that turns auth's "ask the user" interface into an
// event-driven UI: the login window reads the code off the human and hands it
// over, so the solver never waits for input.
type flowSolver struct {
	code     string
	sentinel error
}

// set arms the next solve with a token, or with a sentinel that stops after
// the method's preparation step.
func (s *flowSolver) set(code string, sentinel error) {
	s.code, s.sentinel = code, sentinel
}

func (s *flowSolver) TOTP(context.Context) (string, error) { return s.token() }

func (s *flowSolver) YubiKey(context.Context) (string, error) { return s.token() }

func (s *flowSolver) EmailCode(ctx context.Context, send func(context.Context) error) (string, error) {
	if err := send(ctx); err != nil {
		return "", err
	}
	if s.sentinel != nil {
		return "", s.sentinel
	}
	return s.token()
}

// OpenBrowser refuses Duo and WebAuthn. The login window never routes those
// here, and refusing is better than opening a page whose result cannot come
// back.
func (s *flowSolver) OpenBrowser(context.Context, string) (string, error) {
	return "", errTwoFactorUnsupported
}

func (s *flowSolver) token() (string, error) {
	if s.code == "" {
		return "", errTwoFactorEmptyCode
	}
	return s.code, nil
}

// stepFromResult mirrors one auth.LoginResult in local terms.
func stepFromResult(res auth.LoginResult) (authStep, error) {
	switch res.State {
	case auth.LoginNeedTwoFactor:
		return authStep{needs2FA: true, challenge: promptFromChallenge(res.Challenge)}, nil
	case auth.LoginDone:
		return authStep{
			userKey:      []byte(res.UserKey),
			account:      res.Account,
			refreshToken: res.Token.RefreshToken,
		}, nil
	default:
		return authStep{}, newLoginError(errLoginFlowState, res.State)
	}
}

// promptFromChallenge mirrors the auth challenge in local terms.
func promptFromChallenge(c *auth.TwoFactorChallenge) *twoFactorPrompt {
	if c == nil {
		return nil
	}
	p := &twoFactorPrompt{}
	for _, m := range c.Methods() {
		p.methods = append(p.methods, twoFactorMethod{
			provider:    int(m.Provider),
			name:        m.Provider.String(),
			interaction: m.Interaction.String(),
			email:       m.Email,
			browserURL:  m.BrowserURL,
		})
	}
	return p
}

// friendlyAuthError turns the auth sentinels into the typed failures the login
// window maps to wording. It says no message itself.
func friendlyAuthError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, auth.ErrUnauthorized):
		return errWrongCredentials
	case errors.Is(err, auth.ErrForbidden):
		return errLoginForbidden
	case errors.Is(err, auth.ErrCaptchaRequired):
		return errCaptchaRequired
	case errors.Is(err, auth.ErrNetwork):
		return errNetworkUnreachable
	case errors.Is(err, auth.ErrServer):
		return errServerFailure
	case errors.Is(err, auth.ErrTwoFactorRequired):
		return errNoTwoFactorMethod
	case errors.Is(err, auth.ErrTwoFactorUnsupported):
		return errTwoFactorUnsupported
	default:
		return err
	}
}

func (a *realAccount) session() (accountSession, error) {
	return a.currentSession(), nil
}

// currentSession reads the session without the error channel, for the paths
// that already hold the lock's answer.
func (a *realAccount) currentSession() accountSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sess
}

// checkServer normalizes a user-entered address the way login will, so the
// login window can show a bad one before a network round trip. The login
// window submits the normalized value, which is what the session records.
func (a *realAccount) checkServer(serverURL string) (string, error) {
	cfg, err := auth.NewServerConfig(serverURLOf(serverURL))
	if err != nil {
		return "", newLoginError(errServerAddress, err)
	}
	return cfg.URLs().Base, nil
}

func (a *realAccount) login(email, password, serverURL string) (accountSession, error) {
	return a.runLogin(strings.TrimSpace(email), password, serverURLOf(serverURL), locales.HelloReasonSaveKey())
}

// runLogin drives one full attempt. On success it asks for Hello consent
// before writing the key, so a refusal or a failed prompt leaves the store
// untouched. A 2FA stop is not a success: it parks the flow for submit2FA and
// returns the needs-2FA session with the challenge.
func (a *realAccount) runLogin(email, password, serverURL, helloReason string) (accountSession, error) {
	flow, err := a.newFlow(serverURL)
	if err != nil {
		return a.stay(serverURL), err
	}
	ctx := context.Background()
	if err := flow.start(ctx, email); err != nil {
		return a.stay(serverURL), err
	}
	step, err := flow.password(ctx, password)
	if err != nil {
		return a.stay(serverURL), err
	}
	if step.needs2FA {
		a.parkTwoFactor(flow, serverURL, email, step.challenge)
		return a.currentSession(), nil
	}
	return a.completeLogin(step, serverURL, email, helloReason)
}

// parkTwoFactor records the challenge the login window must answer. The
// plaintext password is not kept while the prompt waits: nothing needs it
// until the flow finishes, and a cancelled attempt must not leave it behind.
func (a *realAccount) parkTwoFactor(flow accountAuth, serverURL, email string, challenge *twoFactorPrompt) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = &pendingTwoFactor{auth: flow, serverURL: serverURL, email: email, challenge: challenge}
	a.sess = accountSession{
		state:       loginNeeds2FA,
		email:       email,
		serverURL:   serverURL,
		twoFAMethod: challenge.first(),
		challenge:   challenge,
	}
}

// completeLogin applies a finished step: Hello consent first, then the key
// write, then the session. The order is the point - a refused or failed
// prompt must leave the store untouched. The session (refresh token +
// identity) is persisted after the key; a failed session write only warns,
// because the login itself succeeded.
func (a *realAccount) completeLogin(step authStep, serverURL, email, helloReason string) (accountSession, error) {
	if err := verifyHello(a.verifier, helloReason); err != nil {
		return a.stay(serverURL), err
	}
	acc := step.account
	if acc.Email == "" {
		acc.Email = email
	}
	// Remember the address next to the key, so a later re-login can prefill it
	// even when the persisted session is gone.
	acc.ServerURL = serverURL
	if err := a.store.Put(acc, secrets.Key(step.userKey)); err != nil {
		return a.stay(serverURL), newLoginError(errSaveKey, err)
	}
	// The address outlives this login: the key and the session record go on
	// sign-out, so remembering it here is what keeps a self-hosted address in
	// the next form. Best-effort, and it never holds up the login.
	a.prefs.save(serverURL)
	warning := a.persistSession(sessionRecord{
		UserID:       acc.UserID,
		Email:        acc.Email,
		ServerURL:    serverURL,
		RefreshToken: step.refreshToken,
	})
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sess = accountSession{
		state:     loginLoggedIn,
		email:     acc.Email,
		serverURL: serverURL,
		userID:    acc.UserID,
		warning:   warning,
	}
	a.lastLoggedIn = a.sess
	a.pending = nil
	return a.sess, nil
}

// persistSession writes the login's refresh token and identity to the
// credential store. An empty refresh token means the server gave nothing to
// restore with, so there is nothing worth saving. A failure is returned as a
// typed warning and never as an error: the key is already stored and unlocking
// still works.
func (a *realAccount) persistSession(rec sessionRecord) error {
	if rec.RefreshToken == "" {
		return nil
	}
	if err := a.sessions.save(rec); err != nil {
		return newLoginError(errSessionPersist, err)
	}
	return nil
}

// stay keeps the previous session but records the server the attempt was
// against, so the page shows the right address after a failure.
func (a *realAccount) stay(serverURL string) accountSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sess.state == loginLoggedOut {
		a.sess.serverURL = serverURL
	}
	return a.sess
}

// submit2FA finishes a login the server parked on a second factor: the login
// window collected the token for one offered method and the flow resubmits the
// grant. Success follows the same order as a password login - Hello consent,
// then the key write - so a refused prompt never reaches the store, and the
// code never leaves this call.
func (a *realAccount) submit2FA(provider int, code string) (accountSession, error) {
	a.mu.Lock()
	p := a.pending
	a.mu.Unlock()
	if p == nil {
		return a.currentSession(), errNoTwoFactorChallenge
	}
	m, ok := p.challenge.method(provider)
	if !ok {
		return a.currentSession(), newLoginError(errProviderNotOffered, provider)
	}
	if !m.takesCode() {
		// Duo and WebAuthn need a browser round trip biowarden does not have;
		// never start one from here.
		return a.currentSession(), errTwoFactorUnsupported
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return a.currentSession(), errTwoFactorEmptyCode
	}
	step, err := p.auth.solveTwoFactor(context.Background(), provider, code)
	if err != nil {
		return a.currentSession(), err
	}
	if step.needs2FA {
		// The server rejected the code or asked for another method; keep the
		// attempt parked so the window can correct it.
		a.parkTwoFactor(p.auth, p.serverURL, p.email, step.challenge)
		return a.currentSession(), nil
	}
	return a.completeLogin(step, p.serverURL, p.email, locales.HelloReasonSaveKey())
}

// sendEmailCode asks the server to mail the code for the pending email
// challenge. The attempt records that the request ran, so the submission that
// follows sends the code without mailing a second one.
func (a *realAccount) sendEmailCode() error {
	a.mu.Lock()
	p := a.pending
	a.mu.Unlock()
	if p == nil {
		return errNoTwoFactorChallenge
	}
	m, ok := p.challenge.method(twoFactorEmail)
	if !ok || !m.takesCode() {
		return errEmailMethodMissing
	}
	a.mu.Lock()
	already := p.emailSent
	a.mu.Unlock()
	if already {
		return nil
	}
	if err := p.auth.sendTwoFactorEmail(context.Background()); err != nil {
		return err
	}
	a.mu.Lock()
	if a.pending == p {
		p.emailSent = true
	}
	a.mu.Unlock()
	return nil
}

// cancelLogin abandons a pending login attempt: the session falls back to the
// one before it, and the parked flow and the challenge are dropped. A sign-in
// started while already signed in therefore returns to that account, while a
// first login lands back on signed out.
func (a *realAccount) cancelLogin() (accountSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sess = a.lastLoggedIn
	a.pending = nil
	return a.sess, nil
}

// logout deletes the signed-in account's key behind a Hello consent and
// clears the in-memory session and token. It is idempotent: logging out when
// already logged out does nothing and asks for nothing.
func (a *realAccount) logout() error {
	a.mu.Lock()
	sess := a.sess
	a.mu.Unlock()
	if sess.state == loginLoggedOut && sess.userID == "" {
		return nil
	}
	var clearErr error
	if sess.userID != "" {
		if err := verifyHello(a.verifier, locales.HelloReasonRemoveKey()); err != nil {
			return err
		}
		if err := a.store.Remove(sess.userID); err != nil {
			return newLoginError(errRemoveKey, err)
		}
		// The key is gone, so the persisted login for it must go too. A failed
		// clear is reported but does not keep the session alive in memory.
		clearErr = a.sessions.clear(sess.userID)
	}
	a.mu.Lock()
	a.sess = accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
	a.lastLoggedIn = a.sess
	a.pending = nil
	a.mu.Unlock()
	if clearErr != nil {
		return newLoginError(errClearSession, clearErr)
	}
	return nil
}

// restoreSession re-validates the login persisted in the OS credential store
// and returns the session to show. The refresh token is the only credential
// involved: the master password is never stored, so a restored session can
// prove the sign-in but cannot re-derive the key without a fresh sign-in.
//
// Three outcomes:
//   - valid: the signed-in session is restored (account page: signed in)
//   - rejected: the stored session is cleared, the key is kept (configured)
//   - unreachable: the stored session is kept, the page stays configured
func (a *realAccount) restoreSession() (accountSession, error) {
	rec, err := a.sessions.load()
	if err != nil {
		if errors.Is(err, errNoStoredSession) {
			return a.currentSession(), nil
		}
		return a.currentSession(), newLoginError(errSessionRead, err)
	}
	fallback := accountSession{state: loginLoggedOut, serverURL: serverURLOf(rec.ServerURL)}
	if rec.RefreshToken == "" {
		a.adoptRestored(fallback)
		return fallback, nil
	}
	tok, err := a.refresh(context.Background(), rec.ServerURL, rec.RefreshToken)
	if err != nil {
		if sessionRevoked(err) {
			_ = a.sessions.clear(rec.UserID)
			a.adoptRestored(fallback)
			return fallback, errSessionRevoked
		}
		a.adoptRestored(fallback)
		return fallback, errSessionUnverified
	}
	// The server may rotate the refresh token; keep the newest one so the next
	// restart still verifies.
	if tok.RefreshToken != "" && tok.RefreshToken != rec.RefreshToken {
		rec.RefreshToken = tok.RefreshToken
		_ = a.sessions.save(rec)
	}
	sess := accountSession{
		state:     loginLoggedIn,
		email:     rec.Email,
		serverURL: serverURLOf(rec.ServerURL),
		userID:    rec.UserID,
	}
	a.mu.Lock()
	a.sess = sess
	a.lastLoggedIn = sess
	a.mu.Unlock()
	return sess, nil
}

// adoptRestored records the configured-but-signed-out session restore fell
// back to, so every later session() read agrees with what the page shows.
func (a *realAccount) adoptRestored(sess accountSession) {
	a.mu.Lock()
	a.sess = sess
	a.mu.Unlock()
}

// forgetSession drops the persisted login for an account whose stored key was
// just deleted, so a restart cannot restore a session for a key that no longer
// exists. When the in-memory session is that account's it is reset to signed
// out as well, or the card would keep claiming a login for a key that is gone;
// a failed clear still resets it, matching logout. It touches no key material
// and asks for no consent.
func (a *realAccount) forgetSession(userID string) error {
	err := a.sessions.clear(userID)
	a.mu.Lock()
	defer a.mu.Unlock()
	// lastLoggedIn is checked too: a pending login keeps it as the session a
	// cancel falls back to, and that must not revive the deleted account.
	if userID != "" && (a.sess.userID == userID || a.lastLoggedIn.userID == userID) {
		a.sess = accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
		a.lastLoggedIn = a.sess
	}
	return err
}

// serverURLOf falls back to the official server when the page has no address.
func serverURLOf(serverURL string) string {
	if s := strings.TrimSpace(serverURL); s != "" {
		return s
	}
	return defaultServerURL
}
