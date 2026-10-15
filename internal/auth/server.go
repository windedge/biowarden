package auth

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// DefaultBaseURL is the official Bitwarden US cloud web vault. This is the one
// address a user has to know; everything else is derived from it.
const DefaultBaseURL = "https://vault.bitwarden.com"

// ServiceURLs are the roots the client talks to, all derived from one base
// URL. Bitwarden's official clients take a single web vault address, read
// {base}/api/config and use the Environment block it returns; the same three
// URLs are kept here. The browser extension only needs API and Identity;
// Events exists for parity with the official client.
type ServiceURLs struct {
	// Base is the normalized web vault address the user entered.
	Base string
	// API serves account and vault data, for example {API}/accounts/profile.
	API string
	// Identity serves tokens and KDF parameters, for example
	// {Identity}/connect/token and {Identity}/accounts/prelogin.
	Identity string
	// Events is the telemetry endpoint; unused today, carried for parity.
	Events string
}

// ServerConfig resolves a user-entered base URL into the Bitwarden service
// URLs. The zero value is not usable; build one with NewServerConfig or
// DefaultServerConfig.
type ServerConfig struct {
	urls ServiceURLs
}

// EnvironmentURLs is the subset of the server's /api/config response that
// tells a client where its services live. Self-hosted servers may put the
// services on different hosts than the web vault, so these override the
// derived defaults.
type EnvironmentURLs struct {
	// API is the "api" field of Environment.
	API string
	// Identity is the "identity" field of Environment.
	Identity string
	// Events is the "events" field of Environment. Optional.
	Events string
}

// DefaultServerConfig returns the official US cloud configuration.
func DefaultServerConfig() ServerConfig {
	cfg, err := NewServerConfig(DefaultBaseURL)
	if err != nil {
		// The constant above is a valid URL; a failure here is a build bug.
		panic("auth: invalid default base URL: " + err.Error())
	}
	return cfg
}

// NewServerConfig normalizes base and derives the service URLs from it.
//
// The official cloud hosts map to their fixed api/identity/events hosts. Any
// other address is treated as self-hosted and the services are assumed to
// live under {base}/api, {base}/identity and {base}/events; a later
// /api/config lookup may override those with ServerConfig.WithEnvironment.
func NewServerConfig(base string) (ServerConfig, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return ServerConfig{}, err
	}
	if cloud, ok := cloudURLs[normalized]; ok {
		return ServerConfig{urls: cloud}, nil
	}
	return ServerConfig{urls: ServiceURLs{
		Base:     normalized,
		API:      joinURL(normalized, "/api"),
		Identity: joinURL(normalized, "/identity"),
		Events:   joinURL(normalized, "/events"),
	}}, nil
}

// cloudURLs maps the known official web vault hosts to their service URLs.
var cloudURLs = map[string]ServiceURLs{
	"https://vault.bitwarden.com": {
		Base:     "https://vault.bitwarden.com",
		API:      "https://api.bitwarden.com",
		Identity: "https://identity.bitwarden.com",
		Events:   "https://events.bitwarden.com",
	},
	"https://vault.bitwarden.eu": {
		Base:     "https://vault.bitwarden.eu",
		API:      "https://api.bitwarden.eu",
		Identity: "https://identity.bitwarden.eu",
		Events:   "https://events.bitwarden.eu",
	},
}

// URLs returns the resolved service URLs.
func (c ServerConfig) URLs() ServiceURLs { return c.urls }

// API returns the API service root.
func (c ServerConfig) API() string { return c.urls.API }

// Identity returns the identity service root.
func (c ServerConfig) Identity() string { return c.urls.Identity }

// Events returns the events service root.
func (c ServerConfig) Events() string { return c.urls.Events }

// WithEnvironment overrides the derived URLs with values read from the
// server's /api/config Environment block. Empty fields keep the derived value,
// so a partial response cannot drop a working default.
func (c ServerConfig) WithEnvironment(env EnvironmentURLs) ServerConfig {
	out := c
	if v := normalizeServiceURL(env.API); v != "" {
		out.urls.API = v
	}
	if v := normalizeServiceURL(env.Identity); v != "" {
		out.urls.Identity = v
	}
	if v := normalizeServiceURL(env.Events); v != "" {
		out.urls.Events = v
	}
	return out
}

// NormalizeBaseURL cleans a user-entered base URL the way the official
// clients do: trim surrounding space, drop trailing slashes, lowercase the
// scheme and host, and assume https:// when no scheme is given. A path prefix
// is preserved because self-hosted installs may live under one.
func NormalizeBaseURL(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("auth: base URL is empty")
	}
	base = strings.TrimRight(base, "/")
	lower := strings.ToLower(base)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		base = "https://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("auth: parse base URL %q: %w", base, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("auth: base URL %q has no host", base)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// normalizeServiceURL cleans a URL that came from the server. It is more
// permissive than NormalizeBaseURL: it accepts a bare host and returns "" for
// an empty value.
func normalizeServiceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	normalized, err := NormalizeBaseURL(raw)
	if err != nil {
		return ""
	}
	return normalized
}

// joinURL appends path to base, honouring exactly one separator.
func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}
