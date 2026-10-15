package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
)

// LoginState is the position in the password -> 2FA -> userKey sequence.
type LoginState int

const (
	// LoginNeedPassword means the flow needs the master password (from Start)
	// and then the password grant can be sent.
	LoginNeedPassword LoginState = iota
	// LoginNeedTwoFactor means the password was accepted but a second factor
	// is required; Result.Challenge lists the options.
	LoginNeedTwoFactor
	// LoginDone means Result.UserKey holds the account's 64-byte userKey.
	LoginDone
	// LoginFailed means the attempt cannot continue.
	LoginFailed
)

// String names the state for logs and UI.
func (s LoginState) String() string {
	switch s {
	case LoginNeedPassword:
		return "need-password"
	case LoginNeedTwoFactor:
		return "need-two-factor"
	case LoginDone:
		return "done"
	case LoginFailed:
		return "failed"
	}
	return "unknown"
}

// TwoFactorSubmission is the user's answer to a TwoFactorChallenge. Provider
// and Token are sent as twoFactorProvider and twoFactorToken.
//
// Remember is the "remember this device" flag (twoFactorRemember). The map
// decision is not to use it: biowarden gates access to the local key with
// Windows Hello instead of persisting a server-side remember token, so the
// GUI must keep Remember false and not render a toggle. The field and
// TokenResponse.TwoFactorToken stay so the decision is cheap to revisit: set
// Remember, then store the returned TwoFactorToken per account and resend it
// as TwoFactorProvider with TwoFactorRemember.
type TwoFactorSubmission struct {
	Provider TwoFactorProvider
	Token    string
	Remember bool
}

// LoginInput carries the data for the current step. Start consumes Email;
// Continue consumes Password, or TwoFactor after a challenge.
type LoginInput struct {
	Email     string
	Password  string
	TwoFactor *TwoFactorSubmission
}

// UserKey is the account's 64-byte symmetric key, the secret every vault item
// is encrypted with. It is a named type rather than a bare []byte so a stray
// %v prints the redacted String below instead of the raw key, like MasterKey.
type UserKey []byte

// String redacts the key so it cannot reach a log through %v.
func (UserKey) String() string { return "auth.UserKey(<redacted>)" }

// LoginResult is the outcome of one step.
type LoginResult struct {
	// State is the next state to act on.
	State LoginState
	// Challenge is set when State is LoginNeedTwoFactor.
	Challenge *TwoFactorChallenge
	// Token is the successful grant when State is LoginDone.
	Token TokenResponse
	// UserKey is the 64-byte account key when State is LoginDone. It is
	// secret: never log it, and never persist it outside secrets.Store.
	UserKey UserKey
	// Account is the account metadata for the stored key, taken from the
	// profile once the token is valid.
	Account secrets.Account
}

// LoginFlow drives one account login. It is the only entry point the GUI
// needs: the pages call Start with an email, answer NeedTwoFactor with
// Continue, and stop at LoginDone.
//
// Wiring the result into the OS. On LoginDone the caller passes
// Result.UserKey through the two existing seams, in this order:
//
//  1. hello.Verifier.Verify(reason) - the Windows Hello consent prompt. If it
//     does not return hello.Verified, the key is discarded.
//  2. secrets.Store.Put(Result.Account, secrets.Key(Result.UserKey)) - stores
//     the key in the Windows Credential Manager, where it waits for later
//     unlocks.
//
// auth does not call either seam itself and holds only their interfaces, so
// the GUI stays in charge of when consent is asked and when the key is
// written. The verifier and store fields are unused until that wiring ticket;
// they exist so the same interface keeps working.
type LoginFlow struct {
	client   *Client
	solver   TwoFactorSolver
	verifier hello.Verifier
	store    secrets.Store

	state              LoginState
	email              string
	params             KdfParams
	masterKey          MasterKey
	masterPasswordHash string
	token              TokenResponse
	challenge          *TwoFactorChallenge
}

// NewLoginFlow wires a flow's collaborators. client is required; solver
// collects 2FA answers, verifier and store are the Windows Hello and
// Credential Manager seams used at the end of a successful login.
func NewLoginFlow(client *Client, solver TwoFactorSolver, verifier hello.Verifier, store secrets.Store) *LoginFlow {
	return &LoginFlow{client: client, solver: solver, verifier: verifier, store: store}
}

// Start begins a login for email and returns a result in LoginNeedPassword.
// It fetches the KDF parameters now, so Continue can derive the master key
// from the password without another round trip.
func (f *LoginFlow) Start(ctx context.Context, email string) (LoginResult, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return LoginResult{State: LoginFailed}, errors.New("auth: login: empty email")
	}
	if f.client == nil {
		return LoginResult{State: LoginFailed}, errors.New("auth: login: no client")
	}
	params, err := f.client.Prelogin(ctx, email)
	if err != nil {
		f.state = LoginFailed
		return LoginResult{State: LoginFailed}, err
	}
	f.state = LoginNeedPassword
	f.email = email
	f.params = params
	f.masterKey = nil
	f.masterPasswordHash = ""
	f.token = TokenResponse{}
	f.challenge = nil
	return LoginResult{State: LoginNeedPassword}, nil
}

// Continue advances the flow, either with the master password
// (LoginNeedPassword) or with a 2FA answer (LoginNeedTwoFactor). It returns
// LoginNeedTwoFactor, LoginDone or LoginFailed.
func (f *LoginFlow) Continue(ctx context.Context, in LoginInput) (LoginResult, error) {
	switch f.state {
	case LoginNeedPassword:
		return f.continuePassword(ctx, in.Password)
	case LoginNeedTwoFactor:
		return f.continueTwoFactor(ctx, in.TwoFactor)
	default:
		return LoginResult{State: LoginFailed}, fmt.Errorf("auth: login: cannot continue from state %s", f.state)
	}
}

// SolveTwoFactor answers the current challenge through the configured solver
// and resubmits the token request. It is the step a login window runs after
// Continue returned LoginNeedTwoFactor: the challenge's Methods tell the
// window what to render, the user picks a method, and SolveTwoFactor collects
// the answer and resends the grant.
//
// On success it returns LoginDone, or a fresh LoginNeedTwoFactor when the
// server asks for another method. A rejected code, a cancelled or timed-out
// solve, and an unsupported method all leave the flow in LoginNeedTwoFactor
// with the same challenge and return the error, so the caller can correct the
// code, retry, or pick a different offered method.
func (f *LoginFlow) SolveTwoFactor(ctx context.Context, provider TwoFactorProvider) (LoginResult, error) {
	if f.state != LoginNeedTwoFactor || f.challenge == nil {
		return LoginResult{State: LoginFailed}, fmt.Errorf("auth: login: no two-factor challenge to solve (state %s)", f.state)
	}
	submission, err := f.solve(ctx, provider)
	if err != nil {
		return f.retryTwoFactor(err)
	}
	result, err := f.continueTwoFactor(ctx, submission)
	if err != nil {
		return f.retryTwoFactor(err)
	}
	return result, nil
}

// retryTwoFactor rewinds to NeedTwoFactor after a failed solve or a rejected
// resubmission, keeping the current challenge so the user can try again. A
// new challenge the server sent is already stored on the flow.
func (f *LoginFlow) retryTwoFactor(err error) (LoginResult, error) {
	f.state = LoginNeedTwoFactor
	return LoginResult{State: LoginNeedTwoFactor, Challenge: f.challenge}, err
}

// solve asks the solver for the token of one offered method. The method's
// interaction decides which solver entry point is used.
func (f *LoginFlow) solve(ctx context.Context, provider TwoFactorProvider) (*TwoFactorSubmission, error) {
	method, ok := f.challenge.Method(provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s was not offered", ErrTwoFactorUnsupported, provider)
	}
	if f.solver == nil {
		return nil, fmt.Errorf("%w: no solver is configured", ErrTwoFactorUnsupported)
	}
	var (
		token string
		err   error
	)
	switch method.Interaction {
	case InteractionCode:
		token, err = f.solveCode(ctx, method)
	case InteractionBrowser:
		if method.BrowserURL == "" {
			return nil, fmt.Errorf("%w: %s needs a browser URL the flow does not have yet", ErrTwoFactorUnsupported, provider)
		}
		token, err = f.solver.OpenBrowser(ctx, method.BrowserURL)
	default:
		return nil, fmt.Errorf("%w: %s has no solver path", ErrTwoFactorUnsupported, provider)
	}
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("auth: login: the %s solver returned an empty token", provider)
	}
	return &TwoFactorSubmission{Provider: provider, Token: token}, nil
}

// solveCode handles the three methods that need a string from the user. Email
// is the only one with a server-side preparation step.
func (f *LoginFlow) solveCode(ctx context.Context, method TwoFactorMethod) (string, error) {
	switch method.Provider {
	case TwoFactorAuthenticator:
		return f.solver.TOTP(ctx)
	case TwoFactorYubiKey:
		return f.solver.YubiKey(ctx)
	case TwoFactorEmail:
		return f.solver.EmailCode(ctx, f.sendEmailCode)
	}
	return "", fmt.Errorf("%w: %s has no code solver", ErrTwoFactorUnsupported, method.Provider)
}

// sendEmailCode posts the send-email-login request for the email method. The
// challenge records that it ran, so a solver that calls send again does not
// produce a second email.
func (f *LoginFlow) sendEmailCode(ctx context.Context) error {
	if f.challenge != nil && f.challenge.EmailSent {
		return nil
	}
	if err := f.client.SendEmailLogin(ctx, f.email, f.masterPasswordHash); err != nil {
		return err
	}
	if f.challenge != nil {
		f.challenge.EmailSent = true
	}
	return nil
}

// continuePassword derives the master key, hashes it and sends the password
// grant.
func (f *LoginFlow) continuePassword(ctx context.Context, password string) (LoginResult, error) {
	if password == "" {
		return LoginResult{State: LoginFailed}, errors.New("auth: login: empty master password")
	}
	masterKey, err := DeriveMasterKey(f.params, password)
	if err != nil {
		f.state = LoginFailed
		return LoginResult{State: LoginFailed}, err
	}
	hash, err := MasterPasswordHash(masterKey, password)
	if err != nil {
		f.state = LoginFailed
		return LoginResult{State: LoginFailed}, err
	}
	f.masterKey = masterKey
	f.masterPasswordHash = hash
	return f.sendPasswordGrant(ctx, TwoFactorSubmission{})
}

// continueTwoFactor resends the password grant with the user's second factor.
func (f *LoginFlow) continueTwoFactor(ctx context.Context, submission *TwoFactorSubmission) (LoginResult, error) {
	if submission == nil {
		return LoginResult{State: LoginFailed}, errors.New("auth: login: missing two-factor submission")
	}
	return f.sendPasswordGrant(ctx, *submission)
}

// sendPasswordGrant posts the token request and routes the outcome. The
// password hash is the one computed in continuePassword, reused for the 2FA
// retry so the password never has to be held.
func (f *LoginFlow) sendPasswordGrant(ctx context.Context, submission TwoFactorSubmission) (LoginResult, error) {
	req := TokenRequest{
		Email:              f.email,
		MasterPasswordHash: f.masterPasswordHash,
		DeviceID:           f.client.DeviceID(),
		TwoFactorProvider:  submission.Provider,
		TwoFactorToken:     submission.Token,
		Remember:           submission.Remember,
	}
	resp, err := f.client.Login(ctx, req)
	// A two-factor error without a challenge cannot be answered or retried;
	// fall through so the caller receives the error instead of a dead-end
	// NeedTwoFactor state.
	if errors.Is(err, ErrTwoFactorRequired) && resp.Challenge != nil {
		f.state = LoginNeedTwoFactor
		f.challenge = resp.Challenge
		return LoginResult{State: LoginNeedTwoFactor, Challenge: resp.Challenge}, nil
	}
	if err != nil {
		f.state = LoginFailed
		return LoginResult{State: LoginFailed}, err
	}
	return f.finish(ctx, resp)
}

// finish unwraps the userKey from the grant and reads the account profile.
func (f *LoginFlow) finish(ctx context.Context, resp TokenResponse) (LoginResult, error) {
	if resp.Key.isZero() {
		f.state = LoginFailed
		return LoginResult{State: LoginFailed}, errors.New("auth: login: token response has no wrapped user key (passwordless or key-migration account)")
	}
	userKey, err := UnlockUserKey(f.masterKey, resp.Key)
	if err != nil {
		f.state = LoginFailed
		return LoginResult{State: LoginFailed}, err
	}
	profile, err := f.client.Profile(ctx, resp.AccessToken)
	if err != nil {
		f.state = LoginFailed
		return LoginResult{State: LoginFailed}, err
	}
	f.state = LoginDone
	f.token = resp
	return LoginResult{
		State:   LoginDone,
		Token:   resp,
		UserKey: UserKey(userKey),
		Account: secrets.Account{
			UserID:    profile.ID,
			Email:     profile.Email,
			Name:      profile.Name,
			CreatedAt: time.Now(),
		},
	}, nil
}
