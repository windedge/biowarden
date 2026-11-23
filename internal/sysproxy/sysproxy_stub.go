//go:build !windows

package sysproxy

type stubReader struct{}

// NewReader returns the no-op reader used on platforms without a system proxy seam.
func NewReader() Reader { return stubReader{} }

// Supported reports that this platform has no system proxy reader; see ErrUnsupported.
func Supported() bool { return false }

func (stubReader) System() (Config, bool) { return Config{}, false }
