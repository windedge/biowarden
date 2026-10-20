package main

import (
	"github.com/egoist/mygo"

	"github.com/windedge/biowarden/internal/ui"
)

// runGUI starts the tray app under a single-instance lock. Host mode never
// calls it: the browser runs a host process per connection, so those must
// start concurrently rather than hand their arguments to a running one.
func runGUI() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	// A second launch means "show me the window": biowarden may be sitting
	// in the tray with its window hidden.
	mygo.App.OnSecondInstance(func([]string, string) {
		for _, w := range mygo.Windows() {
			ui.Reveal(w)
		}
	})
	return ui.Run()
}
