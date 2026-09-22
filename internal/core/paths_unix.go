//go:build !windows

package core

import (
	"os"
	"path/filepath"
)

// ResolveConfigHome returns the base directory of the config files of the user.
func ResolveConfigHome() string {
	if home := os.Getenv("XDG_CONFIG_HOME"); home != "" {
		return home
	}
	return filepath.Join(HomeDirectory(), ".config")
}

// ResolveStateHome returns the base directory of the files this client writes.
func ResolveStateHome() string {
	if home := os.Getenv("XDG_STATE_HOME"); home != "" {
		return home
	}
	return filepath.Join(HomeDirectory(), ".local", "state")
}
