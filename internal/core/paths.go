package core

import (
	"os"
	"path/filepath"
	"strings"
)

// stateDirectory is the directory for the files this client writes: the history file
// and the logs.
const stateDirectory = "masume"

// ResolveStatePath returns the full path of a file this client writes.
func ResolveStatePath(fileName string) string {
	return filepath.Join(ResolveStateHome(), stateDirectory, fileName)
}

// HomeDirectory returns the home directory of the user, or an empty string.
func HomeDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// ShortenHomePath replaces the home directory prefix with `~`.
func ShortenHomePath(path string) string {
	home := HomeDirectory()
	if home == "" || !strings.HasPrefix(path, home) {
		return path
	}
	return "~" + path[len(home):]
}

// ExpandHomePath expands a leading `~`. A config file and a path the user types both
// accept it.
func ExpandHomePath(path string) string {
	if path == "~" {
		return HomeDirectory()
	}
	if strings.HasPrefix(path, "~/") ||
		(os.PathSeparator == '\\' && strings.HasPrefix(path, `~\`)) {
		return filepath.Join(HomeDirectory(), path[2:])
	}
	return path
}

// WriteFileWholly writes the file through a temporary file in the same directory, and
// renames it over the path. A client that stops in the middle of a write leaves the file it
// had, not half of the new one.
func WriteFileWholly(path string, text []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	drop := func(reason error) error {
		_ = file.Close()
		_ = os.Remove(temporaryPath)
		return reason
	}
	if _, err := file.Write(text); err != nil {
		return drop(err)
	}
	if err := file.Chmod(mode); err != nil {
		return drop(err)
	}
	// The write reaches the disk before the rename.
	if err := file.Sync(); err != nil {
		return drop(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}
