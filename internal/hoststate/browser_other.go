//go:build !windows

package hoststate

// DetectBrowser has no parent process API off Windows; the GUI is Windows
// only, so an empty id simply records nothing identifiable.
func DetectBrowser() string { return "" }
