// Package autostart wraps the app's login item, the "start when the user
// logs in" setting a settings page toggles. It stays free of the GUI so the
// host and future pages can use it alike.
package autostart

import (
	"fmt"

	"github.com/egoist/mygo"
)

// SetOpenAtLogin starts biowarden when the user logs in, or stops it. On
// Windows it writes the user's Run key
// (HKCU\Software\Microsoft\Windows\CurrentVersion\Run), which Task Manager
// lists and can disable.
func SetOpenAtLogin(open bool) error {
	if err := mygo.App.SetOpenAtLogin(open); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	return nil
}

// OpenAtLogin reports whether biowarden starts when the user logs in.
func OpenAtLogin() bool {
	return mygo.App.OpenAtLogin()
}
