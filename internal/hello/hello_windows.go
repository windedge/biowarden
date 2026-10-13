package hello

// New returns the Windows Hello verifier. The syscall implementation replaces
// the stub here; see docs/research/windows-hello-go.md.
func New() Verifier { return stub{} }
