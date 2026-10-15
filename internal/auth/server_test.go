package auth

import "testing"

func TestNewServerConfigCloud(t *testing.T) {
	cases := []struct {
		name string
		base string
		want ServiceURLs
	}{
		{
			name: "official us cloud",
			base: "https://vault.bitwarden.com",
			want: ServiceURLs{
				Base:     "https://vault.bitwarden.com",
				API:      "https://api.bitwarden.com",
				Identity: "https://identity.bitwarden.com",
				Events:   "https://events.bitwarden.com",
			},
		},
		{
			name: "official eu cloud",
			base: "https://vault.bitwarden.eu",
			want: ServiceURLs{
				Base:     "https://vault.bitwarden.eu",
				API:      "https://api.bitwarden.eu",
				Identity: "https://identity.bitwarden.eu",
				Events:   "https://events.bitwarden.eu",
			},
		},
		{
			name: "cloud with trailing slash",
			base: "https://vault.bitwarden.com/",
			want: ServiceURLs{
				Base:     "https://vault.bitwarden.com",
				API:      "https://api.bitwarden.com",
				Identity: "https://identity.bitwarden.com",
				Events:   "https://events.bitwarden.com",
			},
		},
		{
			name: "cloud uppercased host",
			base: "HTTPS://Vault.Bitwarden.COM",
			want: ServiceURLs{
				Base:     "https://vault.bitwarden.com",
				API:      "https://api.bitwarden.com",
				Identity: "https://identity.bitwarden.com",
				Events:   "https://events.bitwarden.com",
			},
		},
		{
			name: "cloud with missing scheme",
			base: "vault.bitwarden.com",
			want: ServiceURLs{
				Base:     "https://vault.bitwarden.com",
				API:      "https://api.bitwarden.com",
				Identity: "https://identity.bitwarden.com",
				Events:   "https://events.bitwarden.com",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := NewServerConfig(tc.base)
			if err != nil {
				t.Fatalf("NewServerConfig(%q): %v", tc.base, err)
			}
			if got := cfg.URLs(); got != tc.want {
				t.Errorf("URLs() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNewServerConfigSelfHosted(t *testing.T) {
	cases := []struct {
		name string
		base string
		want ServiceURLs
	}{
		{
			name: "plain host",
			base: "https://vault.example.com",
			want: ServiceURLs{
				Base:     "https://vault.example.com",
				API:      "https://vault.example.com/api",
				Identity: "https://vault.example.com/identity",
				Events:   "https://vault.example.com/events",
			},
		},
		{
			name: "trailing slash and whitespace",
			base: "  https://vault.example.com/  ",
			want: ServiceURLs{
				Base:     "https://vault.example.com",
				API:      "https://vault.example.com/api",
				Identity: "https://vault.example.com/identity",
				Events:   "https://vault.example.com/events",
			},
		},
		{
			name: "path prefix is preserved",
			base: "https://host.example/bitwarden/",
			want: ServiceURLs{
				Base:     "https://host.example/bitwarden",
				API:      "https://host.example/bitwarden/api",
				Identity: "https://host.example/bitwarden/identity",
				Events:   "https://host.example/bitwarden/events",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := NewServerConfig(tc.base)
			if err != nil {
				t.Fatalf("NewServerConfig(%q): %v", tc.base, err)
			}
			if got := cfg.URLs(); got != tc.want {
				t.Errorf("URLs() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNewServerConfigRejectsEmpty(t *testing.T) {
	for _, base := range []string{"", "   ", "/"} {
		if _, err := NewServerConfig(base); err == nil {
			t.Errorf("NewServerConfig(%q) = nil error, want failure", base)
		}
	}
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	if got := cfg.Identity(); got != "https://identity.bitwarden.com" {
		t.Errorf("Identity() = %q", got)
	}
	if got := cfg.API(); got != "https://api.bitwarden.com" {
		t.Errorf("API() = %q", got)
	}
}

func TestWithEnvironmentOverrides(t *testing.T) {
	cfg, err := NewServerConfig("https://vault.example.com")
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.WithEnvironment(EnvironmentURLs{
		API:      "https://api.example.com/",
		Identity: "IDENTITY.example.com",
	})
	if got.API() != "https://api.example.com" {
		t.Errorf("API() = %q, want https://api.example.com", got.API())
	}
	if got.Identity() != "https://identity.example.com" {
		t.Errorf("Identity() = %q, want https://identity.example.com", got.Identity())
	}
	if got.Events() != "https://vault.example.com/events" {
		t.Errorf("Events() = %q, want the derived default", got.Events())
	}
}

func TestNormalizeBaseURLLowercasesSchemeAndHost(t *testing.T) {
	got, err := NormalizeBaseURL("HTTP://Vault.Example.COM/Path/")
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://vault.example.com/Path"; got != want {
		t.Errorf("NormalizeBaseURL() = %q, want %q", got, want)
	}
}

func TestJoinURL(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"https://a.example", "/connect/token", "https://a.example/connect/token"},
		{"https://a.example/", "connect/token", "https://a.example/connect/token"},
		{"https://a.example/", "/connect/token", "https://a.example/connect/token"},
	}
	for _, tc := range cases {
		if got := joinURL(tc.base, tc.path); got != tc.want {
			t.Errorf("joinURL(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}
