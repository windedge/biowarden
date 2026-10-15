package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg, err := NewServerConfig(srv.URL)
	if err != nil {
		t.Fatalf("NewServerConfig: %v", err)
	}
	return NewClient(cfg, "device-123")
}

func TestPreloginPasswordVariant(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/identity/accounts/prelogin/password" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		if err := json.Unmarshal(body, &req); err != nil || req["email"] != "ada@example.com" {
			t.Errorf("request body = %s", body)
		}
		_, _ = w.Write([]byte(`{"KdfSettings":{"KdfType":0,"Iterations":600000},"Salt":"ada@example.com"}`))
	})

	params, err := client.Prelogin(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("Prelogin: %v", err)
	}
	want := KdfParams{Type: KdfPBKDF2SHA256, Iterations: 600000, Salt: "ada@example.com"}
	if params != want {
		t.Errorf("Prelogin() = %+v, want %+v", params, want)
	}
}

func TestPreloginArgon2id(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"KdfSettings":{"KdfType":1,"Iterations":3,"Memory":64,"Parallelism":4},"Salt":"server-salt"}`))
	})
	params, err := client.Prelogin(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("Prelogin: %v", err)
	}
	want := KdfParams{Type: KdfArgon2id, Iterations: 3, Memory: 64, Parallelism: 4, Salt: "server-salt"}
	if params != want {
		t.Errorf("Prelogin() = %+v, want %+v", params, want)
	}
}

func TestPreloginFallsBackToLegacy(t *testing.T) {
	var legacyHits int
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/accounts/prelogin/password":
			w.WriteHeader(http.StatusNotFound)
		case "/identity/accounts/prelogin":
			legacyHits++
			body, _ := io.ReadAll(r.Body)
			var req map[string]string
			if err := json.Unmarshal(body, &req); err != nil || req["email"] != "Ada@Example.COM" {
				t.Errorf("legacy request body = %s", body)
			}
			// The legacy response has no salt; case must not matter for the
			// KDF/KDFIterations names.
			_, _ = w.Write([]byte(`{"Kdf":0,"KdfIterations":100000,"KdfMemory":null,"KdfParallelism":null}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	params, err := client.Prelogin(context.Background(), "  Ada@Example.COM  ")
	if err != nil {
		t.Fatalf("Prelogin: %v", err)
	}
	if legacyHits != 1 {
		t.Errorf("legacy endpoint hit %d times, want 1", legacyHits)
	}
	want := KdfParams{Type: KdfPBKDF2SHA256, Iterations: 100000, Salt: "ada@example.com"}
	if params != want {
		t.Errorf("Prelogin() = %+v, want %+v", params, want)
	}
}

func TestPreloginLegacyArgon2id(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity/accounts/prelogin" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"KDF":1,"KDFIterations":3,"KDFMemory":64,"KDFParallelism":4}`))
	})
	params, err := client.Prelogin(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("Prelogin: %v", err)
	}
	want := KdfParams{Type: KdfArgon2id, Iterations: 3, Memory: 64, Parallelism: 4, Salt: "ada@example.com"}
	if params != want {
		t.Errorf("Prelogin() = %+v, want %+v", params, want)
	}
}

func TestPreloginRejectsBadResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"unknown kdf type", `{"KdfSettings":{"KdfType":9,"Iterations":1},"Salt":"s"}`},
		{"zero iterations", `{"KdfSettings":{"KdfType":0,"Iterations":0},"Salt":"s"}`},
		{"argon2 missing memory", `{"KdfSettings":{"KdfType":1,"Iterations":3,"Parallelism":4},"Salt":"s"}`},
		{"no salt", `{"KdfSettings":{"KdfType":0,"Iterations":1}}`},
		{"no kdf settings", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity/accounts/prelogin/password" {
					_, _ = w.Write([]byte(tc.body))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			if _, err := client.Prelogin(context.Background(), "ada@example.com"); err == nil {
				t.Fatal("Prelogin() = nil error, want failure")
			}
		})
	}
}

func TestKdfParamsValidateMinimums(t *testing.T) {
	cases := []struct {
		name    string
		params  KdfParams
		wantErr bool
	}{
		// PBKDF2 lower bound: 5000 (kdf-config.ts:19,46-52).
		{"pbkdf2 at the 5000 minimum", KdfParams{Type: KdfPBKDF2SHA256, Iterations: 5000}, false},
		{"pbkdf2 one below the 5000 minimum", KdfParams{Type: KdfPBKDF2SHA256, Iterations: 4999}, true},
		{"pbkdf2 zero iterations", KdfParams{Type: KdfPBKDF2SHA256, Iterations: 0}, true},
		// Argon2id lower bounds: iterations 2, memory 16 MiB, parallelism 1
		// (kdf-config.ts:75-77,126-144).
		{"argon2id at every minimum", KdfParams{Type: KdfArgon2id, Iterations: 2, Memory: 16, Parallelism: 1}, false},
		{"argon2id one iteration below", KdfParams{Type: KdfArgon2id, Iterations: 1, Memory: 16, Parallelism: 1}, true},
		{"argon2id one MiB below", KdfParams{Type: KdfArgon2id, Iterations: 2, Memory: 15, Parallelism: 1}, true},
		{"argon2id one lane below", KdfParams{Type: KdfArgon2id, Iterations: 2, Memory: 16, Parallelism: 0}, true},
		{"unsupported kdf type", KdfParams{Type: KdfType(9), Iterations: 5000}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.params.validate()
			if tc.wantErr && err == nil {
				t.Fatal("validate() = nil error, want a rejection")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestPreloginRejectsDowngradedKdf(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"pbkdf2 below the minimum", `{"KdfSettings":{"KdfType":0,"Iterations":4999},"Salt":"s"}`},
		{"argon2id below the iteration minimum", `{"KdfSettings":{"KdfType":1,"Iterations":1,"Memory":16,"Parallelism":1},"Salt":"s"}`},
		{"argon2id below the memory minimum", `{"KdfSettings":{"KdfType":1,"Iterations":2,"Memory":15,"Parallelism":1},"Salt":"s"}`},
		{"argon2id below the parallelism minimum", `{"KdfSettings":{"KdfType":1,"Iterations":2,"Memory":16,"Parallelism":0},"Salt":"s"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity/accounts/prelogin/password" {
					_, _ = w.Write([]byte(tc.body))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			if _, err := client.Prelogin(context.Background(), "ada@example.com"); err == nil {
				t.Fatal("Prelogin() = nil error, want a downgrade rejection")
			}
		})
	}
}

func TestLoginSuccess(t *testing.T) {
	var gotForm map[string]string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity/connect/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded; charset=utf-8" {
			t.Errorf("Content-Type = %q", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotForm = make(map[string]string)
		for k := range r.PostForm {
			gotForm[k] = r.PostForm.Get(k)
		}
		_, _ = w.Write([]byte(`{
			"access_token":"access-1",
			"expires_in":3600,
			"refresh_token":"refresh-1",
			"token_type":"Bearer",
			"Key":"2.EBAQEBAQEBAQEBAQEBAQEA==|lpnnGSmUGNu7ZCJ5Uolx7d9CVUBvEbcJaGOmPYHCx+il7Q6R8TAI/afRlpqIGZtpqu/pArTL27HDSsLvaof54O6vbgQ0GJV4LLaTRQwxr1M=|ap0f2828naJvmZGRUCidFoReX3tUbRxcPCn1bNmi1O4=",
			"PrivateKey":null
		}`))
	})

	resp, err := client.Login(context.Background(), TokenRequest{
		Email:              "ada@example.com",
		MasterPasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	wantForm := map[string]string{
		"scope":            "api offline_access",
		"client_id":        ClientID,
		"deviceType":       DeviceType,
		"deviceName":       DeviceName,
		"deviceIdentifier": "device-123",
		"grant_type":       "password",
		"username":         "ada@example.com",
		"password":         "hash",
	}
	for key, want := range wantForm {
		if got := gotForm[key]; got != want {
			t.Errorf("form %s = %q, want %q", key, got, want)
		}
	}
	if _, ok := gotForm["twoFactorToken"]; ok {
		t.Errorf("twoFactorToken sent without a 2FA submission")
	}
	if resp.AccessToken != "access-1" || resp.RefreshToken != "refresh-1" || resp.ExpiresIn != 3600 || resp.TokenType != "Bearer" {
		t.Errorf("token fields = %+v", resp)
	}
	if resp.Key.Type != EncTypeAesCbc256HmacSha256 {
		t.Errorf("Key.Type = %d, want 2", resp.Key.Type)
	}
}

func TestLoginTwoFactorChallenge(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"TwoFactorProviders":[0,2],
			"TwoFactorProviders2":{
				"2":{"AuthUrl":"https://duo.example/auth"},
				"0":{},
				"1":{"Email":"ada@example.com"}
			},
			"Email":"ada@example.com"
		}`))
	})

	resp, err := client.Login(context.Background(), TokenRequest{Email: "ada@example.com", MasterPasswordHash: "hash"})
	if !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("Login error = %v, want ErrTwoFactorRequired", err)
	}
	if resp.Challenge == nil {
		t.Fatal("Challenge = nil, want a populated challenge")
	}
	if len(resp.Challenge.Providers) != 2 || resp.Challenge.Providers[0] != TwoFactorAuthenticator || resp.Challenge.Providers[1] != TwoFactorDuo {
		t.Errorf("Providers = %v, want [authenticator duo] in server order", resp.Challenge.Providers)
	}
	if resp.Challenge.DuoURL != "https://duo.example/auth" {
		t.Errorf("DuoURL = %q", resp.Challenge.DuoURL)
	}
	if resp.Challenge.Email != "ada@example.com" {
		t.Errorf("Email = %q", resp.Challenge.Email)
	}
}

func TestLoginTwoFactorChallengeFromMapOnly(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"TwoFactorProviders2":{"7":{},"0":{}}}`))
	})
	resp, err := client.Login(context.Background(), TokenRequest{Email: "ada@example.com", MasterPasswordHash: "hash"})
	if !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("Login error = %v, want ErrTwoFactorRequired", err)
	}
	if resp.Challenge == nil || len(resp.Challenge.Providers) != 2 {
		t.Fatalf("Challenge = %+v, want two providers", resp.Challenge)
	}
	if resp.Challenge.Providers[0] != TwoFactorAuthenticator || resp.Challenge.Providers[1] != TwoFactorWebAuthn {
		t.Errorf("Providers = %v, want numeric order [authenticator webauthn]", resp.Challenge.Providers)
	}
}

// The identity server sends TwoFactorProviders as a JSON string array and
// TwoFactorProviders2 as an object keyed by the provider number, both on a 400
// whose error is invalid_grant. These bodies are the official client's own
// fixtures:
//
//	bitwarden-clients libs/auth/src/common/login-strategies/login.strategy.spec.ts:357-363 (TOTP)
//	bitwarden-clients libs/auth/src/common/login-strategies/login.strategy.spec.ts:392-399 (email)
//
// TwoFactorProviders2 is the field the official client keys its 2FA branch on
// (libs/common/src/services/api.service.ts:204-209). Before the fix the string
// array failed to decode into []int, the whole body was rejected, and Login
// returned "two-factor required" with a nil Challenge.
func TestLoginTwoFactorChallengeFromServerShape(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		providers []TwoFactorProvider
		email     string
	}{
		{
			name: "totp string array",
			body: `{
				"error":"invalid_grant",
				"error_description":"Two factor required.",
				"TwoFactorProviders":["0"],
				"TwoFactorProviders2":{"0":null}
			}`,
			providers: []TwoFactorProvider{TwoFactorAuthenticator},
		},
		{
			name: "email string array with masked payload",
			body: `{
				"error":"invalid_grant",
				"error_description":"Two factor required.",
				"TwoFactorProviders":["1"],
				"TwoFactorProviders2":{"1":{"Email":"k***@bitwarden.com"}},
				"Email":"kyle@bitwarden.com"
			}`,
			providers: []TwoFactorProvider{TwoFactorEmail},
			email:     "kyle@bitwarden.com",
		},
		{
			name:      "number array still accepted",
			body:      `{"TwoFactorProviders":[0,2],"TwoFactorProviders2":{"0":{},"2":{"AuthUrl":"https://duo.example/auth"}}}`,
			providers: []TwoFactorProvider{TwoFactorAuthenticator, TwoFactorDuo},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			})
			resp, err := client.Login(context.Background(), TokenRequest{Email: "ada@example.com", MasterPasswordHash: "hash"})
			if !errors.Is(err, ErrTwoFactorRequired) {
				t.Fatalf("Login error = %v, want ErrTwoFactorRequired", err)
			}
			if resp.Challenge == nil {
				t.Fatal("Challenge = nil, want a populated challenge")
			}
			if !reflect.DeepEqual(resp.Challenge.Providers, tc.providers) {
				t.Errorf("Providers = %v, want %v", resp.Challenge.Providers, tc.providers)
			}
			if resp.Challenge.Email != tc.email {
				t.Errorf("Email = %q, want %q", resp.Challenge.Email, tc.email)
			}
		})
	}
}

// A 400 whose message classifies as a 2FA error but whose body has no provider
// list cannot produce a challenge. Login must keep the error so the flow is
// not stranded in NeedTwoFactor with Challenge == nil.
func TestLoginTwoFactorWithoutProvidersKeepsError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"twoFactorRequired"}`))
	})
	resp, err := client.Login(context.Background(), TokenRequest{Email: "ada@example.com", MasterPasswordHash: "hash"})
	if err == nil {
		t.Fatal("Login() = nil error, want the two-factor error kept")
	}
	if !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("Login error = %v, want ErrTwoFactorRequired", err)
	}
	if resp.Challenge != nil {
		t.Errorf("Challenge = %+v, want nil without a provider list", resp.Challenge)
	}
}

func TestLoginSendsTwoFactorRetryFields(t *testing.T) {
	var gotForm map[string]string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = make(map[string]string)
		for k := range r.PostForm {
			gotForm[k] = r.PostForm.Get(k)
		}
		_, _ = w.Write([]byte(`{"access_token":"a","token_type":"Bearer","Key":null}`))
	})
	if _, err := client.Login(context.Background(), TokenRequest{
		Email:              "ada@example.com",
		MasterPasswordHash: "hash",
		TwoFactorProvider:  TwoFactorAuthenticator,
		TwoFactorToken:     "123456",
		Remember:           true,
	}); err != nil {
		t.Fatalf("Login: %v", err)
	}
	want := map[string]string{
		"twoFactorProvider": "0",
		"twoFactorToken":    "123456",
		"twoFactorRemember": "1",
	}
	for key, value := range want {
		if gotForm[key] != value {
			t.Errorf("form %s = %q, want %q", key, gotForm[key], value)
		}
	}
}

func TestLoginMapsCredentialErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"invalid grant", http.StatusBadRequest, `{"error":"invalid_grant"}`, ErrUnauthorized},
		{"unauthorized", http.StatusUnauthorized, `{}`, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, `{}`, ErrForbidden},
		{"server error", http.StatusInternalServerError, `{}`, ErrServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := client.Login(context.Background(), TokenRequest{Email: "ada@example.com", MasterPasswordHash: "hash"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Login error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestLoginMissingKeyIsZero(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"a","token_type":"Bearer"}`))
	})
	resp, err := client.Login(context.Background(), TokenRequest{Email: "ada@example.com", MasterPasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !resp.Key.isZero() {
		t.Errorf("Key = %+v, want zero for a missing response field", resp.Key)
	}
}

func TestRefresh(t *testing.T) {
	var gotForm map[string]string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity/connect/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = r.ParseForm()
		gotForm = make(map[string]string)
		for k := range r.PostForm {
			gotForm[k] = r.PostForm.Get(k)
		}
		_, _ = w.Write([]byte(`{"access_token":"access-2","expires_in":3600,"token_type":"Bearer"}`))
	})
	resp, err := client.Refresh(context.Background(), "refresh-1")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if resp.AccessToken != "access-2" {
		t.Errorf("AccessToken = %q", resp.AccessToken)
	}
	want := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": "refresh-1",
		"client_id":     ClientID,
	}
	for key, value := range want {
		if gotForm[key] != value {
			t.Errorf("form %s = %q, want %q", key, gotForm[key], value)
		}
	}
}

func TestRefreshRejectsEmptyToken(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, err := client.Refresh(context.Background(), ""); err == nil {
		t.Fatal("Refresh(\"\") = nil error, want failure")
	}
}

func TestLogoutIsOfflineAndNil(t *testing.T) {
	var calls int
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	if err := client.Logout(context.Background(), "access-1"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if calls != 0 {
		t.Errorf("Logout issued %d requests, want 0", calls)
	}
}

func TestProfileSendsBearerToken(t *testing.T) {
	var gotAuth string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/accounts/profile" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"Id":"user-1","Email":"ada@example.com","Name":"Ada"}`))
	})
	profile, err := client.Profile(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if gotAuth != "Bearer access-1" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if profile.ID != "user-1" || profile.Email != "ada@example.com" || profile.Name != "Ada" {
		t.Errorf("Profile = %+v", profile)
	}
}

func TestSendEmailLogin(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	var gotAuthEmail string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthEmail = r.Header.Get(authEmailHeader)
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := client.SendEmailLogin(context.Background(), "ada@example.com", "hash"); err != nil {
		t.Fatalf("SendEmailLogin: %v", err)
	}
	if gotPath != "/api/two-factor/send-email-login" {
		t.Errorf("path = %q", gotPath)
	}
	wantAuthEmail := base64.RawURLEncoding.EncodeToString([]byte("ada@example.com"))
	if gotAuthEmail != wantAuthEmail {
		t.Errorf("%s = %q, want %q", authEmailHeader, gotAuthEmail, wantAuthEmail)
	}
	want := map[string]string{
		"email":              "ada@example.com",
		"masterPasswordHash": "hash",
		"deviceIdentifier":   "device-123",
	}
	for key, value := range want {
		if gotBody[key] != value {
			t.Errorf("body %s = %q, want %q", key, gotBody[key], value)
		}
	}
}
