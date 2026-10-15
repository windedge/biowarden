//go:build windows

package hello

import (
	"os"
	"sync"

	"github.com/windedge/biowarden/internal/hostlog"
)

// Debug logging for the consent prompt's placement. Only window handles,
// thread ids, booleans and Win32 error codes are written: no credential, key
// or user input can reach these lines. With BIOWARDEN_LOG unset the logger is
// never opened and nothing is written.
var (
	debugMu  sync.Mutex
	debugLog *hostlog.Logger
)

func debugEnabled() bool {
	return hostlog.ParseLevel(os.Getenv("BIOWARDEN_LOG")) == hostlog.LevelDebug
}

func logDebug(message string) {
	if !debugEnabled() {
		return
	}
	debugMu.Lock()
	if debugLog == nil {
		path, err := hostlog.DefaultPath()
		if err != nil {
			debugLog = hostlog.Nop()
		} else if l, err := hostlog.New(path, hostlog.LevelDebug); err != nil {
			debugLog = hostlog.Nop()
		} else {
			debugLog = l
		}
	}
	l := debugLog
	debugMu.Unlock()
	l.Debug(message)
}
