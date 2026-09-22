package cfg

import "os"

// resolveShell returns the shell and the flags that run one command line. COMSPEC is the
// command interpreter of the machine, and cmd.exe is the one every Windows install has.
func resolveShell() (string, []string) {
	shell := os.Getenv("COMSPEC")
	if shell == "" {
		shell = "cmd.exe"
	}
	return shell, []string{"/d", "/s", "/c"}
}
