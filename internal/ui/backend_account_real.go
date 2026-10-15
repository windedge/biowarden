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

// errNeedCredentials reports a refresh or relogin with no cached login in
// memory, which happens after a restart: the password is never persisted, so
// the user has to sign in again before the key can be re-derived.
var errNeedCredentials = errors.New("需要先登录账号，才能刷新密钥")

// errTwoFactorUnsupported reports the 2FA submit step the login window ticket
// owns. Until it lands the account page's inline prompt cannot finish, and the
// advanced fold's extraction script stays the fallback.
var errTwoFactorUnsupported = errors.New("两步验证提交由登录窗工单实现；可先用高级折叠区里的提取脚本")

// accountAuth is the slice of auth.LoginFlow the account backend drives: one
// start with the email, then one password step. It is defined in local terms
// so tests (and any future transport) can stand in without a network.
type accountAuth interface {
	start(ctx context.Context, email string) error
	password(ctx context.Context, password string) (authStep, error)
}

// authStep is one password outcome: either the flow finished with the account
// key, or it stopped for a second factor.
type authStep struct {
	needs2FA  bool
	challenge *twoFactorPrompt
	userKey   []byte
	account   secrets.Account
}

// accountAuthFactory builds one login attempt's flow for a server URL.
type accountAuthFactory func(serverURL string) (accountAuth, error)

// realAccount is the accountBackend on the real seams: internal/auth performs
// the login, Windows Hello gates the write, and the Credential Manager stores
// the key. The access and refresh tokens stay inside the flow and are dropped
// as soon as the key is derived; the login inputs are kept in memory for the
// run so refreshKey and relogin can re-derive the key. Nothing is persisted.
type realAccount struct {
	store    secrets.Store
	verifier hello.Verifier
	newFlow  accountAuthFactory

	mu     sync.Mutex
	sess   accountSession
	email  string
	pass   string
	server string
}

func newRealAccount(store secrets.Store, verifier hello.Verifier, factory accountAuthFactory) *realAccount {
	a := &realAccount{store: store, verifier: verifier, newFlow: factory}
	a.sess = accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
	return a
}

// newRealAccountState wires the account page onto the real store, verifier and
// auth flow; newAccountState in state_account.go calls it. The key card and
// the login path share one store instance so their views cannot diverge.
func newRealAccountState() accountState {
	store := secrets.New()
	verifier := hello.New()
	return newAccountStateWith(newRealKeysWith(store, verifier), newRealAccount(store, verifier, newAuthFlowFactory()))
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
		return &authFlowAdapter{flow: auth.NewLoginFlow(client, auth.UnsupportedSolver{}, nil, nil)}, nil
	}
}

// authFlowAdapter turns auth.LoginFlow into the local accountAuth vocabulary.
type authFlowAdapter struct {
	flow *auth.LoginFlow
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
	switch res.State {
	case auth.LoginNeedTwoFactor:
		return authStep{needs2FA: true, challenge: promptFromChallenge(res.Challenge)}, nil
	case auth.LoginDone:
		return authStep{userKey: []byte(res.UserKey), account: res.Account}, nil
	default:
		return authStep{}, fmt.Errorf("登录未完成：%s", res.State)
	}
}

// promptFromChallenge mirrors the auth challenge in local terms.
func promptFromChallenge(c *auth.TwoFactorChallenge) *twoFactorPrompt {
	if c == nil {
		return nil
	}
	p := &twoFactorPrompt{email: c.Email, duoURL: c.DuoURL, webAuthnURL: c.WebAuthnURL}
	for _, provider := range c.Providers {
		p.providers = append(p.providers, provider.String())
	}
	return p
}

// friendlyAuthError turns the auth sentinels into one line a human can act on.
func friendlyAuthError(err error) error {
	switch {
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
	default:
		return err
	}
}

func (a *realAccount) session() (accountSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sess, nil
}

func (a *realAccount) login(email, password, serverURL string) (accountSession, error) {
	return a.runLogin(strings.TrimSpace(email), password, serverURLOf(serverURL), "保存 biowarden 密钥")
}

// runLogin drives one full attempt. On success it asks for Hello consent
// before writing the key, so a refusal or a failed prompt leaves the store
// untouched. A 2FA stop is not a success: it returns the needs-2FA session
// with the challenge and writes nothing.
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
		a.mu.Lock()
		a.sess = accountSession{
			state:       loginNeeds2FA,
			email:       email,
			serverURL:   serverURL,
			twoFAMethod: firstProvider(step.challenge),
			challenge:   step.challenge,
		}
		// Keep the password in memory only, so the login window ticket can
		// answer the challenge without asking the user to retype it.
		a.email, a.pass, a.server = email, password, serverURL
		sess := a.sess
		a.mu.Unlock()
		return sess, nil
	}
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
	a.mu.Lock()
	a.sess = accountSession{
		state:     loginLoggedIn,
		email:     acc.Email,
		serverURL: serverURL,
		userID:    acc.UserID,
	}
	a.email, a.pass, a.server = email, password, serverURL
	sess := a.sess
	a.mu.Unlock()
	return sess, nil
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

func (a *realAccount) submit2FA(string) (accountSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sess, errTwoFactorUnsupported
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
	if sess.userID != "" {
		if err := verifyHello(a.verifier, "删除 biowarden 密钥"); err != nil {
			return err
		}
		if err := a.store.Remove(sess.userID); err != nil {
			return fmt.Errorf("删除密钥失败：%w", err)
		}
	}
	a.mu.Lock()
	a.sess = accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
	a.email, a.pass, a.server = "", "", ""
	a.mu.Unlock()
	return nil
}

func (a *realAccount) refreshKey() error {
	return a.reauthenticate("刷新 biowarden 密钥")
}

func (a *realAccount) relogin() error {
	return a.reauthenticate("重新登录 biowarden")
}

// reauthenticate re-runs the login with the credentials kept in memory and
// rewrites the stored key after a fresh Hello consent. Without a cached login
// (the app was restarted) it asks the user to sign in again. A stop for 2FA is
// reported as an error, not as a successful refresh: the key was not rewritten.
func (a *realAccount) reauthenticate(helloReason string) error {
	a.mu.Lock()
	email, password, serverURL := a.email, a.pass, a.server
	a.mu.Unlock()
	if email == "" || password == "" {
		return errNeedCredentials
	}
	sess, err := a.runLogin(email, password, serverURL, helloReason)
	if err != nil {
		return err
	}
	if sess.state == loginNeeds2FA {
		return errTwoFactorUnsupported
	}
	return nil
}

// serverURLOf falls back to the official server when the page has no address.
func serverURLOf(serverURL string) string {
	if s := strings.TrimSpace(serverURL); s != "" {
		return s
	}
	return defaultServerURL
}

// firstProvider names the first offered 2FA method, for the prompt's label.
func firstProvider(p *twoFactorPrompt) string {
	if p == nil || len(p.providers) == 0 {
		return ""
	}
	return p.providers[0]
}
