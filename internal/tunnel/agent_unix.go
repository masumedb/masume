//go:build !windows

package tunnel

import (
	"io"
	"net"
	"os"
)

// resolveAgentAddress returns the address of the ssh agent, and false where this machine
// has none.
func resolveAgentAddress() (string, bool) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	return socket, socket != ""
}

// dialAgent opens the connection to the ssh agent.
func dialAgent(address string) (io.ReadWriteCloser, error) {
	return net.Dial("unix", address)
}
