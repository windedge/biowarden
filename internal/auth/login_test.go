package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

// loginServer serves the prelogin/token/profile endpoints the flow walks. The
// KDF parameters and the wrapped Key are the vectors from kdf_test.go and
// keys_test.go, so a successful flow must derive the known master key and
// unwrap the known userKey.
func loginServer(t *testing.T, twoFactorFirst bool) *Client {
	t.Helper()
	var tokenCalls int
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/accounts/prelogin/password":
			_, _ = w.Write([]byte(`{"KdfSettings":{"KdfType":0,"Iterations":5000},"Salt":"user@example.com"}`))
		case "/identity/connect/token":
			tokenCalls++
			if twoFactorFirst && tokenCalls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"TwoFactorProviders":[0],"TwoFactorProviders2":{"0":{}}}`))
				return
			}
			if twoFactorFirst {
				body, _ := io.ReadAll(r.Body)
				if !bytes.Contains(body, []byte("twoFactorProvider=0")) || !bytes.Contains(body, []byte("twoFactorToken=123456")) {
					t.Errorf("2FA retry body = %s", body)
				}
			}
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","Key":"2.EBAQEBAQEBAQEBAQEBAQEA==|lpnnGSmUGNu7ZCJ5Uolx7d9CVUBvEbcJaGOmPYHCx+il7Q6R8TAI/afRlpqIGZtpqu/pArTL27HDSsLvaof54O6vbgQ0GJV4LLaTRQwxr1M=|ap0f2828naJvmZGRUCidFoReX3tUbRxcPCn1bNmi1O4="}`))
		case "/api/accounts/profile":
			_, _ = w.Write([]byte(`{"Id":"user-1","Email":"user@example.com","Name":"Test User"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
}

func TestLoginFlowHappyPath(t *testing.T) {
	flow := NewLoginFlow(loginServer(t, false))
	ctx := context.Background()

	start, err := flow.Start(ctx, "  user@example.com ")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if start.State != LoginNeedPassword {
		t.Fatalf("Start state = %s, want need-password", start.State)
	}

	result, err := flow.Continue(ctx, LoginInput{Password: "password"})
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if result.State != LoginDone {
		t.Fatalf("Continue state = %s, want done", result.State)
	}
	wantKey := make([]byte, 64)
	for i := range wantKey {
		wantKey[i] = byte(i)
	}
	if !bytes.Equal(result.UserKey, wantKey) {
		t.Errorf("UserKey = %x, want %x", result.UserKey, wantKey)
	}
	if result.Account.UserID != "user-1" || result.Account.Email != "user@example.com" || result.Account.Name != "Test User" {
		t.Errorf("Account = %+v", result.Account)
	}
	if result.Account.ServerURL != "" {
		t.Errorf("Account.ServerURL = %q, want it left to the caller", result.Account.ServerURL)
	}
	if result.Token.AccessToken != "access-1" {
		t.Errorf("Token.AccessToken = %q", result.Token.AccessToken)
	}
}

func TestLoginFlowTwoFactor(t *testing.T) {
	flow := NewLoginFlow(loginServer(t, true))
	ctx := context.Background()

	if _, err := flow.Start(ctx, "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first, err := flow.Continue(ctx, LoginInput{Password: "password"})
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if first.State != LoginNeedTwoFactor {
		t.Fatalf("state = %s, want need-two-factor", first.State)
	}
	if first.Challenge == nil || !first.Challenge.Has(TwoFactorAuthenticator) {
		t.Fatalf("Challenge = %+v, want the authenticator provider", first.Challenge)
	}

	second, err := flow.Continue(ctx, LoginInput{TwoFactor: &TwoFactorSubmission{
		Provider: TwoFactorAuthenticator,
		Token:    "123456",
	}})
	if err != nil {
		t.Fatalf("Continue(2FA): %v", err)
	}
	if second.State != LoginDone {
		t.Fatalf("state = %s, want done", second.State)
	}
	if len(second.UserKey) != 64 {
		t.Errorf("UserKey length = %d, want 64", len(second.UserKey))
	}
}

// A two-factor 400 without a provider list must surface as a failure instead
// of a dead-end NeedTwoFactor state with a nil challenge.
func TestLoginFlowTwoFactorWithoutProvidersFails(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/accounts/prelogin/password":
			_, _ = w.Write([]byte(`{"KdfSettings":{"KdfType":0,"Iterations":5000},"Salt":"user@example.com"}`))
		case "/identity/connect/token":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"twoFactorRequired"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	flow := NewLoginFlow(client)
	if _, err := flow.Start(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	result, err := flow.Continue(context.Background(), LoginInput{Password: "password"})
	if err == nil {
		t.Fatal("Continue() = nil error, want the challenge-less two-factor error kept")
	}
	if !errors.Is(err, ErrTwoFactorRequired) {
		t.Errorf("Continue error = %v, want ErrTwoFactorRequired in the chain", err)
	}
	if result.State != LoginFailed {
		t.Errorf("state = %s, want failed", result.State)
	}
	if result.Challenge != nil {
		t.Errorf("Challenge = %+v, want nil", result.Challenge)
	}
}

func TestLoginFlowRejectsEmptyEmail(t *testing.T) {
	flow := NewLoginFlow(loginServer(t, false))
	if _, err := flow.Start(context.Background(), "   "); err == nil {
		t.Fatal("Start() with an empty email = nil error, want failure")
	}
}

func TestLoginFlowRejectsContinueFromWrongState(t *testing.T) {
	flow := NewLoginFlow(loginServer(t, false))
	if _, err := flow.Continue(context.Background(), LoginInput{Password: "password"}); err == nil {
		t.Fatal("Continue() before Start = nil error, want failure")
	}
}

func TestLoginFlowRejectsEmptyPassword(t *testing.T) {
	flow := NewLoginFlow(loginServer(t, false))
	if _, err := flow.Start(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := flow.Continue(context.Background(), LoginInput{}); err == nil {
		t.Fatal("Continue() with an empty password = nil error, want failure")
	}
}

func TestLoginFlowRejectsMissingWrappedKey(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/accounts/prelogin/password":
			_, _ = w.Write([]byte(`{"KdfSettings":{"KdfType":0,"Iterations":5000},"Salt":"user@example.com"}`))
		case "/identity/connect/token":
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	flow := NewLoginFlow(client)
	if _, err := flow.Start(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := flow.Continue(context.Background(), LoginInput{Password: "password"}); err == nil {
		t.Fatal("Continue() without a wrapped key = nil error, want failure")
	}
}
