// Command biowarden is one executable in two modes: the GUI and the native
// messaging host the browser launches. How the process started decides which.
package main

import (
	"fmt"
	"os"

	"github.com/windedge/biowarden/internal/host"
	"github.com/windedge/biowarden/internal/ui"
)

func main() {
	if isHostMode() {
		if err := host.Run(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "biowarden host:", err)
			os.Exit(1)
		}
		return
	}
	if err := ui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "biowarden:", err)
		os.Exit(1)
	}
}
