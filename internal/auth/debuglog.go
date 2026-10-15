package auth

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/windedge/biowarden/internal/hostlog"
)

// Debug logging for the two-factor handshake. The shape below is the only
// thing recorded: field names, JSON types and provider numbers. No field value
// is ever copied, so a token, key, email or password cannot reach the log.
var (
	debugLogMu sync.Mutex
	debugLog   *hostlog.Logger
)

// openDebugLog opens the shared host log at the level BIOWARDEN_LOG asks for.
// Any failure disables the diagnostic rather than blocking a login.
func openDebugLog() *hostlog.Logger {
	path, err := hostlog.DefaultPath()
	if err != nil {
		return hostlog.Nop()
	}
	logger, err := hostlog.New(path, hostlog.ParseLevel(os.Getenv("BIOWARDEN_LOG")))
	if err != nil {
		return hostlog.Nop()
	}
	return logger
}

// logger returns the shared diagnostic logger, opening it on first use. Tests
// can pre-set debugLog to capture writes.
func logger() *hostlog.Logger {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLog == nil {
		debugLog = openDebugLog()
	}
	return debugLog
}

// logTwoFactorShape records the structure of a 2FA response body. It is off by
// default: with BIOWARDEN_LOG unset (or not "debug") nothing is opened or
// written, so a normal install stays silent.
func logTwoFactorShape(body []byte) {
	if hostlog.ParseLevel(os.Getenv("BIOWARDEN_LOG")) != hostlog.LevelDebug {
		return
	}
	logger().Debug("auth: two-factor response " + twoFactorShape(body))
}

// twoFactorShape summarizes a 2FA response without any of its values: the
// sorted top-level field names, each field's JSON type, and the provider
// numbers the reply offers. It is what a bug report needs to tell a string
// array from a number array, or a missing provider list, without leaking the
// account's data.
func twoFactorShape(body []byte) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return "keys=<not a JSON object>"
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	types := make([]string, 0, len(names))
	for _, name := range names {
		types = append(types, name+"="+jsonShape(fields[name]))
	}
	providers := make([]string, 0)
	if challenge := parseTwoFactorChallenge(body); challenge != nil {
		for _, p := range challenge.Providers {
			providers = append(providers, strconv.Itoa(int(p)))
		}
	}
	return "keys=[" + strings.Join(names, " ") + "] types=[" + strings.Join(types, " ") +
		"] providers=[" + strings.Join(providers, " ") + "]"
}

// jsonShape names the JSON type of one value, with the element type of an
// array (e.g. "array<string>").
func jsonShape(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "" || trimmed == "null":
		return "null"
	case trimmed[0] == '{':
		return "object"
	case trimmed[0] == '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
			return "array"
		}
		return "array<" + scalarShape(items[0]) + ">"
	case trimmed[0] == '"':
		return "string"
	case trimmed[0] == 't' || trimmed[0] == 'f':
		return "bool"
	default:
		return "number"
	}
}

// scalarShape names the JSON type of one scalar/first array element.
func scalarShape(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "" || trimmed == "null":
		return "null"
	case trimmed[0] == '"':
		return "string"
	case trimmed[0] == '{':
		return "object"
	case trimmed[0] == '[':
		return "array"
	case trimmed[0] == 't' || trimmed[0] == 'f':
		return "bool"
	default:
		return "number"
	}
}
