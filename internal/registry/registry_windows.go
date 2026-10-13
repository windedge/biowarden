package registry

// New returns the HKCU backed registrar.
func New() Registrar { return stub{} }
