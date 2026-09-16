// Package clip reads and writes the system clipboard through the tool of the platform.
package clip

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// waitForTool is the time limit for one clipboard tool.
const waitForTool = 2 * time.Second

// tool is one pair of commands for the clipboard of a platform.
type tool struct {
	write   []string
	read    []string
	needEnv string
}

var (
	once   sync.Once
	chosen *tool
)

// listTools returns the tool pairs in the order they are tried.
func listTools() []tool {
	switch runtime.GOOS {
	case "darwin":
		return []tool{{write: []string{"pbcopy"}, read: []string{"pbpaste"}}}
	case "windows":
		return []tool{{
			write: []string{"clip"},
			read:  []string{"powershell", "-NoProfile", "-Command", "Get-Clipboard"},
		}}
	default:
		return []tool{
			{
				write:   []string{"wl-copy"},
				read:    []string{"wl-paste", "--no-newline"},
				needEnv: "WAYLAND_DISPLAY",
			},
			{
				write:   []string{"xclip", "-selection", "clipboard"},
				read:    []string{"xclip", "-selection", "clipboard", "-out"},
				needEnv: "DISPLAY",
			},
			{
				write:   []string{"xsel", "--clipboard", "--input"},
				read:    []string{"xsel", "--clipboard", "--output"},
				needEnv: "DISPLAY",
			},
			{
				write: []string{"termux-clipboard-set"},
				read:  []string{"termux-clipboard-get"},
			},
			{
				write: []string{"clip.exe"},
				read:  []string{"powershell.exe", "-NoProfile", "-Command", "Get-Clipboard"},
			},
		}
	}
}

// findTool returns the first tool pair this machine has, and nil when it has none.
func findTool() *tool {
	once.Do(func() {
		for _, held := range listTools() {
			if held.needEnv != "" && os.Getenv(held.needEnv) == "" {
				continue
			}
			if _, err := exec.LookPath(held.write[0]); err != nil {
				continue
			}
			if _, err := exec.LookPath(held.read[0]); err != nil {
				continue
			}
			found := held
			chosen = &found
			return
		}
	})
	return chosen
}

// Available reports whether this machine has a clipboard tool.
func Available() bool {
	return findTool() != nil
}

// Write puts the text on the system clipboard.
func Write(text string) error {
	found := findTool()
	if found == nil {
		return exec.ErrNotFound
	}
	ctx, stop := context.WithTimeout(context.Background(), waitForTool)
	defer stop()
	command := exec.CommandContext(ctx, found.write[0], found.write[1:]...)
	command.Stdin = strings.NewReader(text)
	return command.Run()
}

// Read returns the text on the system clipboard.
func Read() (string, error) {
	found := findTool()
	if found == nil {
		return "", exec.ErrNotFound
	}
	ctx, stop := context.WithTimeout(context.Background(), waitForTool)
	defer stop()
	command := exec.CommandContext(ctx, found.read[0], found.read[1:]...)
	written, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(written), "\r\n"), nil
}
