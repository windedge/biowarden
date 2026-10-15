package registry

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// appPathLocations are the places App Paths entries live, in lookup order.
// HKCU wins over HKLM, matching the order ShellExecute resolves. The 32-bit
// node is read as a literal path rather than through KEY_WOW64_32KEY because a
// 64-bit biowarden sees only the 64-bit view of HKLM\SOFTWARE under the normal
// open, and a browser installed by a 32-bit installer registers its entry
// there.
var appPathLocations = []struct {
	root registry.Key
	path string
}{
	{registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\`},
	{registry.LOCAL_MACHINE, `Software\Microsoft\Windows\CurrentVersion\App Paths\`},
	{registry.LOCAL_MACHINE, `Software\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths\`},
}

// readAppPath reports the default value of the App Paths subkey for exe. A
// missing key, a missing value, and a failed read all mean "not found": the
// caller must never be told a browser is installed on a guess.
func readAppPath(exe string) (string, bool) {
	for _, loc := range appPathLocations {
		key, err := registry.OpenKey(loc.root, loc.path+exe, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		path, _, err := key.GetStringValue("")
		key.Close()
		if err == nil && strings.TrimSpace(path) != "" {
			return path, true
		}
	}
	return "", false
}
