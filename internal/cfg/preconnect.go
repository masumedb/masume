package cfg

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/proc"
	"github.com/turanmahmudov/masume/internal/tunnel"
)

// A pre-connect command starts a tunnel or proxy before the database connection opens.

// pollInterval is the time between two tests of the port while the command starts.
const pollInterval = 100 * time.Millisecond

// portDialTimeout is the timeout of one test of the port.
const portDialTimeout = time.Second

// stopGrace is the time the process group has to stop before it is killed.
const stopGrace = 2 * time.Second

// PreConnectHandle is the running pre-connect command and the open SSH tunnel.
type PreConnectHandle struct {
	command *exec.Cmd
	// exited closes after Wait. The process ID remains reserved until Wait returns.
	exited chan struct{}
	tunnel *tunnel.Handle
}

// Stop closes the SSH tunnel and stops the command with its process group, including child
// processes such as ssh.
func (handle *PreConnectHandle) Stop() {
	if handle == nil {
		return
	}
	handle.tunnel.Stop()
	handle.tunnel = nil
	if handle.command == nil || handle.command.Process == nil {
		return
	}
	command := handle.command
	handle.command = nil

	select {
	case <-handle.exited:
		return
	default:
	}

	if proc.StopGroup(command) != nil {
		_ = command.Process.Kill()
	}
	select {
	case <-handle.exited:
	case <-time.After(stopGrace):
		_ = proc.KillGroup(command)
		<-handle.exited
	}
}

// isPortOpen is true if a process accepts a connection on the port.
func isPortOpen(host string, port int) bool {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	connection, err := net.DialTimeout("tcp", address, portDialTimeout)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

// waitForPort waits for a TCP listener, command exit, or timeout.
func waitForPort(host string, port int, timeout time.Duration, exited <-chan struct{}) bool {
	deadline := time.Now().Add(timeout)
	for {
		if isPortOpen(host, port) {
			return true
		}
		select {
		case <-exited:
			// A command can exit after starting a background listener.
			return isPortOpen(host, port)
		default:
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}

// StartPreConnectCommand starts the profile command. The caller must stop the handle when the connection closes.
func StartPreConnectCommand(profile Profile) (*PreConnectHandle, error) {
	if profile.Command == "" {
		return &PreConnectHandle{}, nil
	}

	// The command and its children share a separate process group.
	command := buildShellCommand(context.Background(), profile.Command)
	proc.LeadGroup(command)
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf(
			"the pre-connect command for %s failed to start: %w", profile.Name, err)
	}
	handle := &PreConnectHandle{command: command, exited: make(chan struct{})}
	go func() {
		_ = command.Wait()
		close(handle.exited)
	}()

	if profile.WaitForPort == 0 {
		return handle, nil
	}
	if waitForPort(profile.Host, profile.WaitForPort, profile.CommandTimeout, handle.exited) {
		return handle, nil
	}

	handle.Stop()
	return nil, fmt.Errorf(
		"the pre-connect command for %s did not open port %d within %.0fs: %s",
		profile.Name, profile.WaitForPort, profile.CommandTimeout.Seconds(), profile.Command)
}

// StartPreConnect starts the pre-connect command and the SSH tunnel. It returns the profile
// with the tunnel endpoint. The caller stops the handle when the connection closes.
func StartPreConnect(profile Profile) (Profile, *PreConnectHandle, error) {
	handle, err := StartPreConnectCommand(profile)
	if err != nil {
		return profile, nil, err
	}
	if profile.UsesSocket() {
		if profile.OpensTunnel() {
			handle.Stop()
			return profile, nil, fmt.Errorf(
				"%s reaches a unix socket through an ssh tunnel; "+
					"write a host and a port, or drop ssh_host", profile.Name)
		}
		path, err := core.FindSocketPath(profile.Engine, profile.Host, profile.Port)
		if err != nil {
			handle.Stop()
			return profile, nil, err
		}
		profile.DialHost, profile.DialPort = path, profile.Port
		return profile, handle, nil
	}
	if !profile.OpensTunnel() {
		return profile, handle, nil
	}

	open, err := tunnel.Open(buildTunnelSettings(profile), profile.Host, profile.Port)
	if err != nil {
		handle.Stop()
		return profile, nil, err
	}
	handle.tunnel = open
	profile.DialHost, profile.DialPort = open.Address()
	return profile, handle, nil
}

// buildTunnelSettings maps the profile to tunnel settings. The passphrase and the password
// come from the environment.
func buildTunnelSettings(profile Profile) tunnel.Settings {
	passphrase := ""
	if profile.SSHKeyPassphraseEnv != "" {
		passphrase = os.Getenv(profile.SSHKeyPassphraseEnv)
	}
	password := ""
	if profile.SSHPasswordEnv != "" {
		password = os.Getenv(profile.SSHPasswordEnv)
	}
	return tunnel.Settings{
		Host: profile.SSHHost, Port: profile.SSHPort, User: profile.SSHUser,
		KeyPath: profile.SSHKey, KeyPassphrase: passphrase, Password: password,
		KnownHosts: profile.SSHKnownHosts,
	}
}
