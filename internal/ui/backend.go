package ui

// The backend seam is where the pages meet the real implementations. The
// prototype wires fake backends; each page ticket replaces its own file
// (backend_browsers.go, backend_keys.go, backend_settings.go) with calls to
// the registry, secrets, hello and autostart seams. Nothing else changes.

// browsersBackend is what the Browsers page needs from the world.
type browsersBackend interface {
	// refresh reports every supported browser's manifest state.
	refresh() ([]browserRow, error)
	// install registers our manifest for the browser, replacing whatever was
	// there. The page asks for confirmation first when a foreign manifest is
	// registered; the backend only records.
	install(row browserRow) error
	// uninstall removes our registration, leaving a foreign one alone.
	uninstall(row browserRow) error
}

// keysBackend is what the Account page's key card and manual entry need from
// the world.
type keysBackend interface {
	// accounts lists the stored accounts, without their keys.
	accounts() ([]accountRow, error)
	// save stores one account's key after a Hello consent check. It reports
	// whether the consent succeeded.
	save(userID, keyB64 string) error
	// remove deletes one account's key.
	remove(userID string) error
	// helloAvailable reports whether Windows Hello is configured.
	helloAvailable() (bool, error)
	// verify runs one consent check without touching any key.
	verify(reason string) error
}

// settingsBackend is what the Settings and Log pages need from the world.
type settingsBackend interface {
	// autostart reads and writes the login item.
	autostart() (bool, error)
	setAutostart(bool) error
	// logPath is where the host writes its redacted log.
	logPath() string
	// logTail reads the last n bytes of the log, already redacted by the
	// host that wrote it.
	logTail(n int64) (string, error)
}
