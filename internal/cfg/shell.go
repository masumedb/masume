package cfg

import (
	"context"
	"os/exec"
)

// buildShellCommand returns the command that runs one shell line from the config file.
func buildShellCommand(ctx context.Context, line string) *exec.Cmd {
	name, flags := resolveShell()
	return exec.CommandContext(ctx, name, append(flags, line)...)
}
