package sysproxy

import "testing"

func TestParseProxyServer(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Config
	}{
		{"empty", "", Config{}},
		{"single host and port", "127.0.0.1:7890", Config{HTTP: "127.0.0.1:7890", HTTPS: "127.0.0.1:7890"}},
		{"all protocols with spaces after semicolons", "http=127.0.0.1:8080; https=127.0.0.1:8443; socks=127.0.0.1:1080",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8443", SOCKS: "127.0.0.1:1080"}},
		{"no spaces", "http=127.0.0.1:8080;https=127.0.0.1:8443;socks=127.0.0.1:1080",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8443", SOCKS: "127.0.0.1:1080"}},
		{"missing https falls back to http", "http=127.0.0.1:8080;socks=127.0.0.1:1080",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8080", SOCKS: "127.0.0.1:1080"}},
		{"empty https falls back to http", "http=127.0.0.1:8080;https=",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8080"}},
		{"socks only", "socks=127.0.0.1:1080", Config{SOCKS: "127.0.0.1:1080"}},
		{"values with scheme", "http=http://127.0.0.1:8080;https=https://127.0.0.1:8443;socks=socks5://127.0.0.1:1080",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8443", SOCKS: "127.0.0.1:1080"}},
		{"single ipv6", "[::1]:7890", Config{HTTP: "[::1]:7890", HTTPS: "[::1]:7890"}},
		{"per protocol ipv6", "http=[::1]:8080;socks=[::1]:1080",
			Config{HTTP: "[::1]:8080", HTTPS: "[::1]:8080", SOCKS: "[::1]:1080"}},
		{"unknown keys ignored", "ftp=ftp.example.com:21;http=127.0.0.1:8080;ignored=value",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8080"}},
		{"only unknown keys", "ftp=ftp.example.com:21", Config{}},
		{"spaces around separators", "http = 127.0.0.1:8080 ; https = 127.0.0.1:8443 ; socks = 127.0.0.1:1080",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8443", SOCKS: "127.0.0.1:1080"}},
		{"uppercase keys", "HTTPS=127.0.0.1:8443;HTTP=127.0.0.1:8080",
			Config{HTTP: "127.0.0.1:8080", HTTPS: "127.0.0.1:8443"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseProxyServer(tc.raw); got != tc.want {
				t.Fatalf("ParseProxyServer(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}
