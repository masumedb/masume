// Package tunnel opens an SSH local forward to a database server.
package tunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/turanmahmudov/masume/internal/core"
)

// dialTimeout is the SSH dial timeout.
const dialTimeout = 15 * time.Second

// DefaultPort is the default SSH port.
const DefaultPort = 22

// Settings is the SSH endpoint and its credentials.
type Settings struct {
	Host string
	Port int
	User string
	// Private key path. Empty uses the SSH agent.
	KeyPath       string
	KeyPassphrase string
	Password      string
	// Known hosts path. Empty reads `~/.ssh/known_hosts`.
	KnownHosts string
}

// Handle is an open tunnel: the SSH client and the local listener.
type Handle struct {
	client   *ssh.Client
	listener net.Listener
	// Forward target on the SSH server.
	target string
	closed sync.Once
}

// Address returns the local host and port of the tunnel.
func (handle *Handle) Address() (string, int) {
	if handle == nil || handle.listener == nil {
		return "", 0
	}
	host, port, err := net.SplitHostPort(handle.listener.Addr().String())
	if err != nil {
		return "", 0
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return "", 0
	}
	return host, number
}

// Stop closes the listener and the SSH client.
func (handle *Handle) Stop() {
	if handle == nil {
		return
	}
	handle.closed.Do(func() {
		if handle.listener != nil {
			_ = handle.listener.Close()
		}
		if handle.client != nil {
			_ = handle.client.Close()
		}
	})
}

// Open dials the SSH server and starts a local forward to host:port. The SSH server
// resolves that address.
func Open(settings Settings, host string, port int) (*Handle, error) {
	config, err := buildClientConfig(settings)
	if err != nil {
		return nil, err
	}

	serverPort := settings.Port
	if serverPort == 0 {
		serverPort = DefaultPort
	}
	server := net.JoinHostPort(settings.Host, strconv.Itoa(serverPort))
	client, err := ssh.Dial("tcp", server, config)
	if err != nil {
		return nil, fmt.Errorf("the ssh tunnel to %s failed: %w", server, err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("the ssh tunnel found no local port: %w", err)
	}

	handle := &Handle{
		client: client, listener: listener,
		target: net.JoinHostPort(host, strconv.Itoa(port)),
	}
	go handle.acceptConnections()
	return handle, nil
}

// acceptConnections forwards each accepted connection.
func (handle *Handle) acceptConnections() {
	for {
		local, err := handle.listener.Accept()
		if err != nil {
			return
		}
		go handle.forwardConnection(local)
	}
}

// forwardConnection proxies one connection over the SSH client.
func (handle *Handle) forwardConnection(local net.Conn) {
	defer func() { _ = local.Close() }()

	remote, err := handle.client.Dial("tcp", handle.target)
	if err != nil {
		return
	}
	defer func() { _ = remote.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(remote, local)
		close(done)
	}()
	_, _ = io.Copy(local, remote)
	<-done
}

// buildClientConfig returns the SSH client config: auth methods and host key check.
func buildClientConfig(settings Settings) (*ssh.ClientConfig, error) {
	user := settings.User
	if user == "" {
		return nil, errors.New("the ssh tunnel needs ssh_user")
	}
	methods, err := buildAuthMethods(settings)
	if err != nil {
		return nil, err
	}
	if len(methods) == 0 {
		return nil, errors.New(
			"the ssh tunnel found no login: set ssh_key or ssh_password_env, " +
				"or start an ssh agent")
	}
	check, err := buildHostKeyCheck(settings.KnownHosts)
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User: user, Auth: methods, HostKeyCallback: check, Timeout: dialTimeout,
	}, nil
}

// buildAuthMethods returns the auth methods in order: key, password, agent.
func buildAuthMethods(settings Settings) ([]ssh.AuthMethod, error) {
	methods := []ssh.AuthMethod{}
	if settings.KeyPath != "" {
		signer, err := readPrivateKey(settings.KeyPath, settings.KeyPassphrase)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if settings.Password != "" {
		methods = append(methods, ssh.Password(settings.Password))
	}
	if signers, found := readAgentSigners(); found {
		methods = append(methods, ssh.PublicKeysCallback(signers))
	}
	return methods, nil
}

// readPrivateKey returns the signer of the private key path.
func readPrivateKey(path, passphrase string) (ssh.Signer, error) {
	written, err := os.ReadFile(core.ExpandHomePath(path))
	if err != nil {
		return nil, fmt.Errorf("the ssh key %s is unreadable: %w", path, err)
	}
	if passphrase != "" {
		signer, keyErr := ssh.ParsePrivateKeyWithPassphrase(written, []byte(passphrase))
		if keyErr != nil {
			return nil, fmt.Errorf("the ssh key %s is unusable: %w", path, keyErr)
		}
		return signer, nil
	}

	signer, err := ssh.ParsePrivateKey(written)
	if err == nil {
		return signer, nil
	}
	passphraseErr := &ssh.PassphraseMissingError{}
	if errors.As(err, &passphraseErr) {
		return nil, fmt.Errorf(
			"the ssh key %s has a passphrase: set ssh_key_passphrase_env "+
				"or add the key to an ssh agent", path)
	}
	return nil, fmt.Errorf("the ssh key %s is unusable: %w", path, err)
}

// readAgentSigners returns the signers of the ssh agent.
func readAgentSigners() (func() ([]ssh.Signer, error), bool) {
	address, found := resolveAgentAddress()
	if !found {
		return nil, false
	}
	return func() ([]ssh.Signer, error) {
		connection, err := dialAgent(address)
		if err != nil {
			return nil, err
		}
		return agent.NewClient(connection).Signers()
	}, true
}

// buildHostKeyCheck returns the known hosts callback.
func buildHostKeyCheck(path string) (ssh.HostKeyCallback, error) {
	if path == "" {
		path = filepath.Join(core.HomeDirectory(), ".ssh", "known_hosts")
	}
	path = core.ExpandHomePath(path)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf(
			"the known hosts file %s is unreadable: add the server with ssh-keyscan, "+
				"or set ssh_known_hosts", path)
	}
	check, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("the known hosts file %s is unusable: %w", path, err)
	}
	return check, nil
}
