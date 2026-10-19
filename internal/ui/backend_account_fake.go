package ui

import "errors"

// errAccountBackend marks the prototype account backend: the Account page
// stays clickable and every action reports that the auth seam has not
// landed yet, instead of pretending a login succeeded.
var errAccountBackend = errors.New("ui: account login backend is not wired up")

// fakeAccount stands where the real Bitwarden account backend will stand.
// Its session stays logged out, which is what a fresh install is.
type fakeAccount struct{}

// session reports the logged-out state the prototype starts in. The server
// is the official one so the read-only row has something honest to show.
func (fakeAccount) session() (accountSession, error) {
	return accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, nil
}

// restoreSession reports the same logged-out, keyless state the prototype
// starts in; there is nothing on disk to re-validate yet.
func (fakeAccount) restoreSession() (accountSession, error) {
	return accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, nil
}

// checkServer accepts any address, because this backend never dials it.
func (fakeAccount) checkServer(serverURL string) (string, error) {
	return serverURLOf(serverURL), nil
}

func (fakeAccount) login(string, string, string) (accountSession, error) {
	return accountSession{}, errAccountBackend
}

// cancelLogin is the one account action the prototype can honour: it has no
// pending attempt to abandon, so it simply reports the signed-out state.
func (fakeAccount) cancelLogin() (accountSession, error) {
	return accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, nil
}

func (fakeAccount) submit2FA(int, string) (accountSession, error) {
	return accountSession{}, errAccountBackend
}

func (fakeAccount) sendEmailCode() error { return errAccountBackend }

func (fakeAccount) logout() error { return errAccountBackend }

// forgetSession has no persisted session to drop, so it is a no-op that only
// exists to satisfy the seam.
func (fakeAccount) forgetSession(string) error { return nil }
