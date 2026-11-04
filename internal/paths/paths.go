// Package paths names where biowarden keeps its per-user files.
//
// Everything sits under one root: %LOCALAPPDATA%\biowarden, or the user cache
// directory when LOCALAPPDATA is empty so the layout still resolves outside
// Windows. The lookup lives here once so the host's log, the GUI's log, the
// connection state and the browser manifests cannot drift apart, and a port
// reuses the same names instead of copying the environment dance again.
package paths

import (
	"os"
	"path/filepath"
)

const dirName = "biowarden"

// Names directly under the root.
const (
	// HostLogName and GUILogName are one log per process: Windows lets
	// neither process rename a file the other holds open, so a shared file
	// would stop rotating for both.
	HostLogName = "host.log"
	GUILogName  = "gui.log"
	// ConnectionsDirName holds the per-browser "connected" records.
	ConnectionsDirName = "connections"
	// ManifestsDirName holds the native messaging manifest files.
	ManifestsDirName = "manifests"
)

// Root is the per-user biowarden directory: %LOCALAPPDATA%\biowarden, or
// <user cache dir>\biowarden when LOCALAPPDATA is empty.
func Root() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = cache
	}
	return filepath.Join(base, dirName), nil
}

// RootOrTemp is Root for a call site that cannot report an error: the temp
// directory keeps a displayed path from going blank when the profile directory
// is unresolvable.
func RootOrTemp() string {
	root, err := Root()
	if err != nil {
		return filepath.Join(os.TempDir(), dirName)
	}
	return root
}

// HostLog is the host process's log file.
func HostLog() (string, error) { return under(HostLogName) }

// GUILog is the GUI process's own log file.
func GUILog() (string, error) { return under(GUILogName) }

// Connections is the directory holding the per-browser connection records.
func Connections() (string, error) { return under(ConnectionsDirName) }

// Manifests is the directory holding the native messaging manifests.
func Manifests() (string, error) { return under(ManifestsDirName) }

func under(name string) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}
