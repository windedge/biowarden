package auth

import (
	"bufio"
	"context"
	"errors"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// resetProxyState keeps the global transport sources from leaking between
// tests. The production environment resolver is restored as well.
func resetProxyState(t *testing.T) {
	t.Helper()
	original := environmentProxy
	ConfigureProxy(nil, nil)
	t.Cleanup(func() {
		ConfigureProxy(nil, nil)
		environmentProxy = original
	})
}

func getBody(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(body)
}

func TestParseProxyURLTable(t *testing.T) {
	cases := []struct {
		name, raw, user, password, want string
		ok                              bool
	}{
		{"host and port", "127.0.0.1:8080", "", "", "http://127.0.0.1:8080", true},
		{"full URL", "http://proxy.example:8080", "", "", "http://proxy.example:8080", true},
		{"credentials", "proxy.example:8080", "alice", "s3cret", "http://alice:s3cret@proxy.example:8080", true},
		{"socks5 credentials", "socks5://127.0.0.1:1080", "alice", "s3cret", "socks5://alice:s3cret@127.0.0.1:1080", true},
		{"socks5h", "socks5h://127.0.0.1:1080", "", "", "socks5h://127.0.0.1:1080", true},
		{"bracketed IPv6", "[::1]:1080", "", "", "http://[::1]:1080", true},
		{"bad escape", "http://%zz", "", "", "", false},
		{"wrong scheme", "ftp://proxy.example:21", "", "", "", false},
		{"empty", "  ", "", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := parseProxyURL(tc.raw, tc.user, tc.password)
			if !tc.ok {
				if u != nil {
					t.Fatalf("parseProxyURL(%q) = %q, want nil", tc.raw, u)
				}
				return
			}
			if u == nil {
				t.Fatalf("parseProxyURL(%q) = nil, want %q", tc.raw, tc.want)
			}
			if got := u.String(); got != tc.want {
				t.Fatalf("parseProxyURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			if tc.user != "" {
				if got, _ := u.User.Password(); got != tc.password {
					t.Fatalf("password = %q, want %q", got, tc.password)
				}
			}
		})
	}
}

func TestManualProxySendsBasicAuthorization(t *testing.T) {
	resetProxyState(t)

	var direct atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		fmt.Fprint(w, "direct")
	}))
	defer target.Close()

	var proxyHits atomic.Int32
	var authMu sync.Mutex
	var proxyAuth string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		authMu.Lock()
		proxyAuth = r.Header.Get("Proxy-Authorization")
		authMu.Unlock()
		if r.Host != strings.TrimPrefix(target.URL, "http://") {
			t.Errorf("proxy request Host = %q, want %q", r.Host, target.URL)
		}
		fmt.Fprint(w, "via-proxy")
	}))
	defer proxy.Close()

	ConfigureProxy(func() ProxySetting {
		return ProxySetting{Mode: ProxyModeManual, Addr: proxy.URL, User: "alice", Password: "s3cret"}
	}, nil)
	cfg, err := NewServerConfig(target.URL)
	if err != nil {
		t.Fatalf("NewServerConfig: %v", err)
	}
	client := NewClient(cfg, "device-1")
	if client.http.Transport != SharedTransport() {
		t.Fatal("NewClient did not use the shared transport")
	}
	if got := getBody(t, client.http, target.URL); got != "via-proxy" {
		t.Fatalf("body = %q, want via-proxy", got)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	authMu.Lock()
	gotAuth := proxyAuth
	authMu.Unlock()
	if gotAuth != wantAuth {
		t.Fatalf("Proxy-Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if proxyHits.Load() != 1 || direct.Load() != 0 {
		t.Fatalf("proxy hits = %d, direct hits = %d, want 1 and 0", proxyHits.Load(), direct.Load())
	}
}

func TestManualInvalidAddressFailsRequest(t *testing.T) {
	for _, tc := range []struct{ name, addr string }{
		{"empty", ""},
		{"whitespace", "   "},
		{"malformed", "http://%zz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetProxyState(t)

			var direct atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				direct.Add(1)
				fmt.Fprint(w, "direct")
			}))
			defer target.Close()

			ConfigureProxy(func() ProxySetting {
				return ProxySetting{Mode: ProxyModeManual, Addr: tc.addr}
			}, nil)
			_, err := (&http.Client{Transport: SharedTransport()}).Get(target.URL)
			if !errors.Is(err, ErrNoProxyAddress) {
				t.Fatalf("GET error = %v, want ErrNoProxyAddress", err)
			}
			if direct.Load() != 0 {
				t.Fatalf("direct hits = %d, want 0", direct.Load())
			}
		})
	}
}

func TestOffModeGoesDirect(t *testing.T) {
	resetProxyState(t)

	var proxyHits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
	}))
	defer proxy.Close()

	var direct atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		fmt.Fprint(w, "direct")
	}))
	defer target.Close()

	ConfigureProxy(func() ProxySetting {
		return ProxySetting{Mode: ProxyModeOff, Addr: proxy.URL}
	}, nil)
	if got := getBody(t, &http.Client{Transport: SharedTransport()}, target.URL); got != "direct" {
		t.Fatalf("body = %q, want direct", got)
	}
	if direct.Load() != 1 || proxyHits.Load() != 0 {
		t.Fatalf("direct hits = %d, proxy hits = %d, want 1 and 0", direct.Load(), proxyHits.Load())
	}
}

func TestEnvironmentModeUsesEnvironmentResolver(t *testing.T) {
	resetProxyState(t)
	envProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "via-env")
	}))
	defer envProxy.Close()
	t.Setenv("HTTP_PROXY", envProxy.URL)
	// ProxyFromEnvironment reads the process environment only once. The
	// injection below is a fresh resolver, so the dispatch path is testable;
	// production still uses the cached shared resolver.
	environmentProxy = func(req *http.Request) (*url.URL, error) {
		raw := os.Getenv("HTTP_PROXY")
		if raw == "" {
			return nil, nil
		}
		return url.Parse(raw)
	}

	var direct atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		fmt.Fprint(w, "direct")
	}))
	defer target.Close()

	ConfigureProxy(func() ProxySetting { return ProxySetting{Mode: ProxyModeEnvironment} }, nil)
	if got := getBody(t, &http.Client{Transport: SharedTransport()}, target.URL); got != "via-env" {
		t.Fatalf("body = %q, want via-env", got)
	}
	if direct.Load() != 0 {
		t.Fatalf("direct hits = %d, want 0", direct.Load())
	}
}

func TestUnconfiguredTransportUsesEnvironment(t *testing.T) {
	resetProxyState(t)

	var calls atomic.Int32
	environmentProxy = func(req *http.Request) (*url.URL, error) {
		calls.Add(1)
		return nil, nil
	}
	var direct atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		fmt.Fprint(w, "direct")
	}))
	defer target.Close()

	if got := getBody(t, NewClient(authTestConfig(t, target.URL), "device").http, target.URL); got != "direct" {
		t.Fatalf("body = %q, want direct", got)
	}
	if calls.Load() != 1 || direct.Load() != 1 {
		t.Fatalf("env calls = %d, direct hits = %d, want 1 and 1", calls.Load(), direct.Load())
	}
}

func authTestConfig(t *testing.T, base string) ServerConfig {
	t.Helper()
	cfg, err := NewServerConfig(base)
	if err != nil {
		t.Fatalf("NewServerConfig: %v", err)
	}
	return cfg
}

func TestSystemModeDispatchesPerSchemeAndFallsBack(t *testing.T) {
	resetProxyState(t)

	req := func(scheme string) *http.Request {
		return &http.Request{URL: &url.URL{Scheme: scheme, Host: "vault.example.com"}}
	}
	sys := func(httpAddr, httpsAddr, socksAddr string, ok bool) func() (SystemProxy, bool) {
		return func() (SystemProxy, bool) {
			return SystemProxy{HTTP: httpAddr, HTTPS: httpsAddr, SOCKS: socksAddr}, ok
		}
	}

	ConfigureProxy(func() ProxySetting { return ProxySetting{Mode: ProxyModeSystem} },
		sys("127.0.0.1:8080", "10.0.0.2:8443", "10.0.0.3:1080", true))
	httpURL, err := proxyForRequest(req("http"))
	if err != nil {
		t.Fatalf("proxyForRequest(http): %v", err)
	}
	if got, want := httpURL.String(), "http://127.0.0.1:8080"; got != want {
		t.Fatalf("http proxy = %q, want %q", got, want)
	}
	httpsURL, err := proxyForRequest(req("https"))
	if err != nil {
		t.Fatalf("proxyForRequest(https): %v", err)
	}
	if got, want := httpsURL.String(), "http://10.0.0.2:8443"; got != want {
		t.Fatalf("https proxy = %q, want %q", got, want)
	}

	// WinINET may publish only socks=, so the scheme must become socks5.
	ConfigureProxy(func() ProxySetting { return ProxySetting{Mode: ProxyModeSystem} },
		sys("", "", "10.0.0.3:1080", true))
	socksURL, err := proxyForRequest(req("https"))
	if err != nil {
		t.Fatalf("proxyForRequest(socks): %v", err)
	}
	if got, want := socksURL.String(), "socks5://10.0.0.3:1080"; got != want {
		t.Fatalf("socks proxy = %q, want %q", got, want)
	}

	var envCalls atomic.Int32
	environmentProxy = func(r *http.Request) (*url.URL, error) {
		envCalls.Add(1)
		return url.Parse("http://env.example:3128")
	}
	ConfigureProxy(func() ProxySetting { return ProxySetting{Mode: ProxyModeSystem} },
		sys("", "", "", false))
	fallback, err := proxyForRequest(req("http"))
	if err != nil {
		t.Fatalf("proxyForRequest(fallback): %v", err)
	}
	if got, want := fallback.String(), "http://env.example:3128"; got != want || envCalls.Load() != 1 {
		t.Fatalf("fallback = %q, env calls = %d, want %q and 1", got, envCalls.Load(), want)
	}
}

type socksTrace struct {
	mu     sync.Mutex
	user   string
	pass   string
	target string
}

func (s *socksTrace) set(user, pass, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user, s.pass, s.target = user, pass, target
}

func (s *socksTrace) values() (string, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.user, s.pass, s.target
}

// startFakeSOCKS5 is just enough of RFC 1928 to prove that net/http dials a
// socks5:// Proxy URL itself, including username/password authentication.
func startFakeSOCKS5(t *testing.T, trace *socksTrace) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeSOCKS5(conn, trace)
		}
	}()
	return ln.Addr().String()
}

func serveFakeSOCKS5(conn net.Conn, trace *socksTrace) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	r := bufio.NewReader(conn)

	greeting := make([]byte, 2)
	if _, err := io.ReadFull(r, greeting); err != nil || greeting[0] != 5 {
		return
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(r, methods); err != nil {
		return
	}
	method := byte(0xff)
	for _, m := range methods {
		if m == 2 {
			method = 2
			break
		}
		if m == 0 {
			method = 0
		}
	}
	if _, err := conn.Write([]byte{5, method}); err != nil || method == 0xff {
		return
	}

	user, pass := "", ""
	if method == 2 {
		header := make([]byte, 2)
		if _, err := io.ReadFull(r, header); err != nil || header[0] != 1 {
			return
		}
		u := make([]byte, int(header[1]))
		if _, err := io.ReadFull(r, u); err != nil {
			return
		}
		passwordLength := make([]byte, 1)
		if _, err := io.ReadFull(r, passwordLength); err != nil {
			return
		}
		p := make([]byte, int(passwordLength[0]))
		if _, err := io.ReadFull(r, p); err != nil {
			return
		}
		user, pass = string(u), string(p)
		if _, err := conn.Write([]byte{1, 0}); err != nil {
			return
		}
	}

	connect := make([]byte, 4)
	if _, err := io.ReadFull(r, connect); err != nil || connect[0] != 5 || connect[1] != 1 {
		return
	}
	var host string
	switch connect[3] {
	case 1:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		host = net.IP(buf).String()
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(r, length); err != nil {
			return
		}
		buf := make([]byte, int(length[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		host = string(buf)
	case 4:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		host = net.IP(buf).String()
	default:
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(r, portBytes); err != nil {
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(portBytes[0])<<8|int(portBytes[1])))
	trace.set(user, pass, target)
	if _, err := conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 9\r\nConnection: close\r\n\r\nvia-socks")
}

func TestSOCKS5UsesNativeHTTPTransport(t *testing.T) {
	resetProxyState(t)

	trace := &socksTrace{}
	addr := startFakeSOCKS5(t, trace)
	ConfigureProxy(func() ProxySetting {
		return ProxySetting{Mode: ProxyModeManual, Addr: "socks5://" + addr, User: "alice", Password: "s3cret"}
	}, nil)

	u, err := proxyForRequest(&http.Request{URL: &url.URL{Scheme: "http", Host: "socks-target.invalid"}})
	if err != nil {
		t.Fatalf("proxyForRequest: %v", err)
	}
	if u.Scheme != "socks5" || u.Host != addr {
		t.Fatalf("proxy URL = %q, want socks5://%s", u, addr)
	}
	if got, _ := u.User.Password(); u.User.Username() != "alice" || got != "s3cret" {
		t.Fatalf("proxy userinfo = %q, want alice:s3cret", u.User)
	}

	client := &http.Client{Transport: SharedTransport(), Timeout: 3 * time.Second}
	if got := getBody(t, client, "http://socks-target.invalid/api/config"); got != "via-socks" {
		t.Fatalf("body = %q, want via-socks", got)
	}
	user, pass, target := trace.values()
	if user != "alice" || pass != "s3cret" || target != "socks-target.invalid:80" {
		t.Fatalf("SOCKS5 trace = %q/%q/%q, want alice/s3cret/socks-target.invalid:80", user, pass, target)
	}
}

func TestConfigureProxyConcurrentWithTransportUse(t *testing.T) {
	resetProxyState(t)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mode := ProxyModeOff
			if i == 1 {
				mode = ProxyModeEnvironment
			}
			for j := 0; j < 50; j++ {
				ConfigureProxy(func() ProxySetting { return ProxySetting{Mode: mode} }, nil)
				transport := SharedTransport()
				req := &http.Request{URL: &url.URL{Scheme: "http", Host: "example.com"}}
				if _, err := transport.Proxy(req); err != nil {
					t.Errorf("Proxy: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
