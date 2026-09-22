package tunnel

import (
	"io"
	"net"
	"os"
	"strings"
)

// openSSHAgentPipe is the named pipe the OpenSSH agent of Windows listens on.
const openSSHAgentPipe = `\\.\pipe\openssh-ssh-agent`

// resolveAgentAddress returns the address of the ssh agent, and false where this machine
// has none.
func resolveAgentAddress() (string, bool) {
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		return socket, true
	}
	return openSSHAgentPipe, true
}

// dialAgent opens the connection to the ssh agent. A named pipe opens as a file.
func dialAgent(address string) (io.ReadWriteCloser, error) {
	if strings.HasPrefix(address, `\\`) {
		return os.OpenFile(address, os.O_RDWR, 0)
	}
	return net.Dial("unix", address)
}
