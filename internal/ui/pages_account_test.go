package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
	"github.com/windedge/biowarden/locales"
)

// emptyKeys stands where the secrets seam stands, answering that nothing is
// stored, so the account card can render its signed-out shape.
type emptyKeys struct{}

func (emptyKeys) accounts() ([]accountRow, error) { return nil, nil }

func (emptyKeys) save(string, string) error     { return errFakeBackend }
func (emptyKeys) remove(string) error           { return errFakeBackend }
func (emptyKeys) helloAvailable() (bool, error) { return true, nil }
func (emptyKeys) verify(reason string) error    { return errFakeBackend }

// The merged account card renders all three states in both languages: the
// title, the detail line, the hint and the action buttons come from the
// catalog at draw time.
func TestAccountCardRendersThreeStatesInBothLanguages(t *testing.T) {
	for _, tag := range []string{"en", "zh-Hans"} {
		t.Run(tag, func(t *testing.T) {
			appLanguage(t, tag)

			states := []struct {
				name   string
				keys   keysBackend
				sess   accountSession
				texts  []string
				absent []string
			}{
				{
					name: "signed in",
					keys: fakeKeys{},
					sess: loggedInSession(),
					texts: []string{
						locales.AccountCardGroup(),
						locales.AccountCardSignedIn(),
						locales.AccountCardSignedInKey("ada@example.com", defaultServerURL, "2026-01-12 09:24"),
						locales.AccountCardReLogin(),
					},
					absent: []string{locales.AccountNoKeyHint()},
				},
				{
					name: "configured",
					keys: fakeKeys{},
					sess: loggedOutSession(),
					texts: []string{
						locales.AccountCardConfigured(),
						locales.AccountCardConfiguredDetail("ada@example.com", defaultServerURL, "2026-01-12 09:24"),
						locales.AccountCardConfiguredHint(),
						locales.AccountCardReLogin(),
						locales.AccountCardSignOut(),
					},
					absent: []string{locales.LoginTitle()},
				},
				{
					name: "signed out",
					keys: emptyKeys{},
					sess: loggedOutSession(),
					texts: []string{
						locales.AccountCardSignedOut(),
						locales.AccountNoKeyHint(),
						locales.LoginTitle(),
						locales.Refresh(),
					},
					absent: []string{locales.AccountCardSignOut(), locales.AccountCardReLogin()},
				},
			}
			for _, tc := range states {
				t.Run(tc.name, func(t *testing.T) {
					s := newAccountStateWith(tc.keys, &stubAccount{sess: tc.sess})
					// The card only dereferences the login window when a button
					// is clicked, which no test here does.
					tester := ui.NewTester(func(c *ui.Context) { s.accountCard(c, nil) }, 1200, 800)
					tester.Frame()
					for _, want := range tc.texts {
						if !tester.HasText(want) {
							t.Errorf("the %s card does not show %q: %v", tc.name, want, tester.Texts())
						}
					}
					for _, miss := range tc.absent {
						if tester.HasText(miss) {
							t.Errorf("the %s card shows %q, which must stay hidden: %v", tc.name, miss, tester.Texts())
						}
					}
				})
			}
		})
	}
}

// The card's sign-out action removes the key (after the Windows Hello
// confirmation inside removeAccount) and forgets the session.
func TestAccountCardSignOut(t *testing.T) {
	store := newFakeStore()
	if err := store.Put(secrets.Account{UserID: "user-1", Email: "ada@example.com"}, secrets.Key("secret")); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	acct := &stubAccount{sess: loggedOutSession()}
	s := newAccountStateWith(newRealKeysWith(store, &fakeVerifier{result: hello.Verified}), acct)
	waitRestore(t, &s)

	tester := ui.NewTester(func(c *ui.Context) { s.accountCard(c, nil) }, 1200, 800)

	// The sign-out button reads the catalog at draw time, in both languages.
	for _, tag := range []string{"en", "zh-Hans"} {
		appLanguage(t, tag)
		tester.Frame()
		want := locales.AccountCardSignOut()
		if !tester.HasText(want) {
			t.Errorf("the card does not show the %q button in %s: %v", want, tag, tester.Texts())
		}
	}

	appLanguage(t, "en")
	tester.Frame()
	if err := tester.Click(locales.AccountCardSignOut()); err != nil {
		t.Fatalf("click the sign-out button: %v", err)
	}
	drain(t, &s)
	if store.removeCalls != 1 {
		t.Fatalf("Remove calls = %d, want 1", store.removeCalls)
	}
	if len(acct.forgotten) != 1 || acct.forgotten[0] != "user-1" {
		t.Fatalf("forgetSession calls = %v, want user-1", acct.forgotten)
	}
}

// The unverified-session notice must not claim the account is signed out: it
// says the key is still stored and unlocking works.
func TestAccountNoticeSessionUnverifiedWording(t *testing.T) {
	for _, tag := range []string{"en", "zh-Hans"} {
		t.Run(tag, func(t *testing.T) {
			appLanguage(t, tag)
			want := locales.AccountNoticeSessionUnverified()
			for _, banned := range []string{"signed out", "未登录"} {
				if strings.Contains(want, banned) {
					t.Errorf("%s wording %q must not claim %q", tag, want, banned)
				}
			}
			n := pageNotice{kind: noticeAccountSessionUnverified}
			if got := n.text(); got != want {
				t.Errorf("notice text = %q, want %q", got, want)
			}
		})
	}
}
