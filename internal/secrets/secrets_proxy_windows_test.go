package secrets

import (
	"errors"
	"testing"

	"github.com/danieljoos/wincred"
)

// The proxy password shares the test store's isolated namespace, so neither
// round trips nor deletions can reach a real vault entry.
func newTestProxySecrets(t *testing.T) ProxySecrets {
	t.Helper()
	return newProxySecretsWithPrefix(newTestStore(t).prefix)
}

func TestProxyPasswordRoundTrip(t *testing.T) {
	secrets := newTestProxySecrets(t)
	if _, err := secrets.GetProxyPassword(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get before Put = %v, want ErrNotFound", err)
	}

	const password = "short"
	if err := secrets.PutProxyPassword(password); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := secrets.GetProxyPassword()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != password {
		t.Fatalf("Get = %q, want %q", got, password)
	}

	if err := secrets.PutProxyPassword("replaced"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, err = secrets.GetProxyPassword()
	if err != nil || got != "replaced" {
		t.Fatalf("Get after overwrite = %q, %v", got, err)
	}

	if err := secrets.RemoveProxyPassword(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := secrets.GetProxyPassword(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Remove = %v, want ErrNotFound", err)
	}
	if err := secrets.RemoveProxyPassword(); err != nil {
		t.Fatalf("Remove twice = %v, want nil", err)
	}
}

func TestProxyPasswordIsNotAnAccount(t *testing.T) {
	s := newTestStore(t)
	acc := sampleAccount("user-1")
	if err := s.Put(acc, Key("key")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := newProxySecretsWithPrefix(s.prefix).PutProxyPassword("short"); err != nil {
		t.Fatalf("PutProxyPassword: %v", err)
	}

	// Drop the index so List rebuilds it from the credentials themselves:
	// that is where a proxy entry could be mistaken for a user id.
	index, err := wincred.GetGenericCredential(s.indexTarget())
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if err := index.Delete(); err != nil {
		t.Fatalf("delete index: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].UserID != acc.UserID {
		t.Fatalf("rebuilt List = %+v, want only %s", list, acc.UserID)
	}

	creds, err := wincred.FilteredList(s.prefix + "/*")
	if err != nil {
		t.Fatalf("FilteredList: %v", err)
	}
	if len(creds) != 3 {
		t.Fatalf("FilteredList len = %d, want account, index and proxy entries", len(creds))
	}
}

func TestProxyTargetName(t *testing.T) {
	if got := proxyTargetName; got != "Biowarden/Proxy" {
		t.Fatalf("proxyTargetName = %q, want Biowarden/Proxy", got)
	}
}
