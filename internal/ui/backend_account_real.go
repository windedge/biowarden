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
)

// deviceID is the deviceIdentifier biowarden sends to Bitwarden. The official
// desktop client uses its appId GUID; a fixed identifier keeps the server's
// device list stable across runs, which is what "remember this device" needs.
// Deriving a real per-install id is a later ticket.
const deviceID = "biowarden-windows"

// errTwoFactorUnsupported reports a second factor biowarden has no path for:
// Duo (providers 2 and 6) and WebAuthn (7) complete through a browser hand-off
// whose callback the map decision deferred, so the login window points at the
// account page's extraction script instead. The login window renders this
// wording, so it stays one source of truth.
var errTwoFactorUnsupported = errors.New("此方式 biowarden 暂不支持，请在账号页高级选项里用提取脚本录入密钥")

// errNoTwoFactorChallenge reports a 2FA submission with nothing parked to
// answer, which a stale window can reach after the attempt was abandoned.
var errNoTwoFactorChallenge = errors.New("没有等待验证码的登录，请先输入邮箱和主密码")

// errTwoFactorEmptyCode reports a 2FA submission without a code. The window
// disables its button on an empty field; this is the guard behind it.
var errTwoFactorEmptyCode = errors.New("请输入两步验证码")

// errEmailCodePending reports that the send-email-login request went out but
// no code was submitted with it. It is how the "发送验证码" button drives
// auth's email preparation without a token: the flow keeps the challenge and
// the answer is nothing to submit yet.
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
// auth flow; newAccountState in state_account.go calls it. The key card and
// the login path share one store instance so their views cannot diverge.
func newRealAccountState() accountState {
	store := secrets.New()
	verifier := hello.New()
	sessions := newWincredSessionStore(secrets.SessionPrefix)
	account := newRealAccountWith(store, verifier, newAuthFlowFactory(), sessions, newSessionRefresher())
	return newAccountStateWith(newRealKeysWith(store, verifier), account)
}

// newAuthFlowFactory builds real login flows through internal/auth. The
// verifier and store are nil because auth never calls them: this backend asks
// for consent and writes the key itself, in that order.
func newAuthFlowFactory() accountAuthFactory {
	return func(serverURL string) (accountAuth, error) {
		cfg, err := auth.NewServerConfig(serverURL)
		if err != nil {
			return nil, fmt.Errorf("服务器地址无效：%w", err)
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
		return fmt.Errorf("登录流程起始状态异常：%s", res.State)
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
		return authStep{}, fmt.Errorf("登录未完成：%s", res.State)
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

// friendlyAuthError turns the auth sentinels into one line a human can act on.
func friendlyAuthError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, auth.ErrUnauthorized):
		return errors.New("邮箱或主密码不正确")
	case errors.Is(err, auth.ErrForbidden):
		return errors.New("服务器拒绝登录：账号可能被锁定或需要额外验证")
	case errors.Is(err, auth.ErrCaptchaRequired):
		return errors.New("服务器要求人机验证，请先在浏览器里登录一次再重试")
	case errors.Is(err, auth.ErrNetwork):
		return errors.New("无法连接服务器，请检查网络和服务器地址")
	case errors.Is(err, auth.ErrServer):
		return errors.New("服务器返回错误，请稍后重试")
	case errors.Is(err, auth.ErrTwoFactorRequired):
		return errors.New("服务器要求两步验证，但没有返回可用的验证方式")
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
		return "", fmt.Errorf("服务器地址无效：%w", err)
	}
	return cfg.URLs().Base, nil
}

func (a *realAccount) login(email, password, serverURL string) (accountSession, error) {
	return a.runLogin(strings.TrimSpace(email), password, serverURLOf(serverURL), "保存 biowarden 密钥")
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
	if err := a.store.Put(acc, secrets.Key(step.userKey)); err != nil {
		return a.stay(serverURL), fmt.Errorf("保存密钥失败：%w", err)
	}
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
// human-readable warning and never as an error: the key is already stored and
// unlocking still works.
func (a *realAccount) persistSession(rec sessionRecord) string {
	if rec.RefreshToken == "" {
		return ""
	}
	if err := a.sessions.save(rec); err != nil {
		return "登录会话无法持久化，下次启动需要重新登录：" + err.Error()
	}
	return ""
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
		return a.currentSession(), fmt.Errorf("服务器没有提供这种方式（provider %d）", provider)
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
	return a.completeLogin(step, p.serverURL, p.email, "保存 biowarden 密钥")
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
		return errors.New("服务器没有提供邮箱验证码方式")
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
		if err := verifyHello(a.verifier, "删除 biowarden 密钥"); err != nil {
			return err
		}
		if err := a.store.Remove(sess.userID); err != nil {
			return fmt.Errorf("删除密钥失败：%w", err)
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
		return fmt.Errorf("清除登录会话失败：%w", clearErr)
	}
	return nil
}

// restoreSession re-validates the login persisted in the OS credential store
// and returns the session to show. The refresh token is the only credential
// involved: the master password is never stored, so a restored session can
// prove the sign-in but cannot re-derive the key without a fresh sign-in.
//
// Three outcomes:
//   - valid: the signed-in session is restored (account page: 已登录)
//   - rejected: the stored session is cleared, the key is kept (已配置)
//   - unreachable: the stored session is kept, the page stays configured
func (a *realAccount) restoreSession() (accountSession, error) {
	rec, err := a.sessions.load()
	if err != nil {
		if errors.Is(err, errNoStoredSession) {
			return a.currentSession(), nil
		}
		return a.currentSession(), err
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
// exists. It touches no key material and asks for no consent.
func (a *realAccount) forgetSession(userID string) error {
	return a.sessions.clear(userID)
}

// serverURLOf falls back to the official server when the page has no address.
func serverURLOf(serverURL string) string {
	if s := strings.TrimSpace(serverURL); s != "" {
		return s
	}
	return defaultServerURL
}
