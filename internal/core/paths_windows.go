package core

import (
	"os"
	"path/filepath"
)

// ResolveConfigHome returns the base directory of the config files of the user. %APPDATA%
// roams with the account.
func ResolveConfigHome() string {
	if home := os.Getenv("APPDATA"); home != "" {
		return home
	}
	return filepath.Join(HomeDirectory(), "AppData", "Roaming")
}

// ResolveStateHome returns the base directory of the files this client writes.
// %LOCALAPPDATA% stays on the machine.
func ResolveStateHome() string {
	if home := os.Getenv("LOCALAPPDATA"); home != "" {
		return home
	}
	return filepath.Join(HomeDirectory(), "AppData", "Local")
}
