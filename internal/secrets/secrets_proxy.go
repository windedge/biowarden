package secrets

// The proxy password is not an account key: it has no user id, is read on every
// connection attempt, and is not gated by Windows Hello. It gets its own
// credential namespace so it can never be listed or removed as an account.

// ProxySubtree is the namespace below Prefix that holds the proxy password.
const ProxySubtree = "Proxy"

// ProxySecrets stores the Manual proxy password in the OS credential store.
// Get reports ErrNotFound when no password has been saved.
type ProxySecrets interface {
	GetProxyPassword() (string, error)
	PutProxyPassword(password string) error
	RemoveProxyPassword() error // removing a missing password is not an error
}
