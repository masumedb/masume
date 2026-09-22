//go:build !windows

package cfg

// resolveShell returns the shell and the flags that run one command line.
func resolveShell() (string, []string) {
	return "sh", []string{"-c"}
}
