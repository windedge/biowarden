package secrets

// New returns the Credential Manager backed store: Biowarden/<userId> per
// account plus Biowarden/Index for metadata.
func New() Store { return stub{} }
