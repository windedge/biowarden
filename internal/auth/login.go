package auth

import (
	"context"

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

// TwoFactorSubmission is the user's answer to a TwoFactorChallenge. Remember
// asks the server to issue a TwoFactorToken for the next login.
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
// written. The two fields below exist so a later ticket can offer the same
// wiring from inside the flow without changing this interface.
//
// Everything up to LoginDone is stubbed in this skeleton: Start and Continue
// return ErrNotImplemented.
type LoginFlow struct {
	client   *Client
	solver   TwoFactorSolver
	verifier hello.Verifier
	store    secrets.Store

	email     string
	masterKey MasterKey
	token     TokenResponse
	challenge *TwoFactorChallenge
}

// NewLoginFlow wires a flow's collaborators. client is required; solver
// collects 2FA answers, verifier and store are the Windows Hello and
// Credential Manager seams used at the end of a successful login.
func NewLoginFlow(client *Client, solver TwoFactorSolver, verifier hello.Verifier, store secrets.Store) *LoginFlow {
	return &LoginFlow{client: client, solver: solver, verifier: verifier, store: store}
}

// Start begins a login for email. It returns a result in LoginNeedPassword.
//
// The real sequence is: Prelogin(email) -> DeriveMasterKey ->
// MasterPasswordHash -> Login -> either LoginNeedTwoFactor or, on success,
// UnlockUserKey -> Profile -> LoginDone.
//
// Contract only: returns ErrNotImplemented today.
func (f *LoginFlow) Start(ctx context.Context, email string) (LoginResult, error) {
	_ = f
	_ = ctx
	_ = email
	return LoginResult{State: LoginFailed}, ErrNotImplemented
}

// Continue advances the flow, either with the master password
// (LoginNeedPassword) or with a 2FA answer (LoginNeedTwoFactor). It returns
// LoginNeedTwoFactor, LoginDone or LoginFailed.
//
// Contract only: returns ErrNotImplemented today.
func (f *LoginFlow) Continue(ctx context.Context, in LoginInput) (LoginResult, error) {
	_ = f
	_ = ctx
	_ = in
	return LoginResult{State: LoginFailed}, ErrNotImplemented
}
