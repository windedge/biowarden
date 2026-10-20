package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
	"github.com/windedge/biowarden/locales"
)

// deleteIconRed is the theme's Danger color: the trash button paints its
// icon with TextColor(t.Danger), so the red pixels are how the tests see it.
const deleteIconRed = "#dc2626"

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
						// The delete action is an icon now; its accessible label
						// (and tooltip) is still the catalog's delete word.
						locales.Delete(),
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
					absent: []string{locales.Delete(), locales.AccountCardReLogin()},
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

// The card's delete action is a trash icon painted in the Danger color; the
// icon carries the "Delete" label and tooltip, and clicking it removes the
// account.
func TestAccountCardDeleteIcon(t *testing.T) {
	store := newFakeStore()
	if err := store.Put(secrets.Account{UserID: "user-1", Email: "ada@example.com"}, secrets.Key("secret")); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	acct := &stubAccount{sess: loggedOutSession()}
	s := newAccountStateWith(newRealKeysWith(store, &fakeVerifier{result: hello.Verified}), acct)
	waitRestore(t, &s)

	tester := ui.NewTester(func(c *ui.Context) { s.accountCard(c, nil) }, 1200, 800)

	// The icon's label and tooltip is the old button's word, in both
	// languages; the card reads the catalog at draw time.
	for _, tag := range []string{"en", "zh-Hans"} {
		appLanguage(t, tag)
		tester.Frame()
		want := locales.Delete()
		if !tester.HasText(want) {
			t.Errorf("the trash icon does not carry the %q label in %s: %v", want, tag, tester.Texts())
		}
	}

	appLanguage(t, "en")
	tester.Frame()
	red := ui.Hex(deleteIconRed)
	img := tester.Image()
	r, ok := tester.Find(locales.Delete())
	if !ok {
		t.Fatal("the frame does not show the delete icon")
	}
	counts := 0
	for y := int(r.Y); y < int(r.Y+r.H) && y < img.Bounds().Dy(); y++ {
		for x := int(r.X); x < int(r.X+r.W) && x < img.Bounds().Dx(); x++ {
			px := img.RGBAAt(x, y)
			if px.R == red.R && px.G == red.G && px.B == red.B && px.A == red.A {
				counts++
			}
		}
	}
	if counts == 0 {
		t.Error("the delete icon area has no Danger-colored pixels")
	}

	// Clicking the icon removes the key, as the old text button did.
	if err := tester.Click(locales.Delete()); err != nil {
		t.Fatalf("click the delete icon: %v", err)
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
