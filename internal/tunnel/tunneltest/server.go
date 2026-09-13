// Package tunneltest runs an SSH server in the test process. It accepts one key and serves
// direct-tcpip channels, which is everything the tunnel asks of a server.
package tunneltest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/turanmahmudov/masume/internal/tunnel"
)

// User is the login the server accepts.
const User = "ada"

// Server is a running SSH server with the files a client needs to reach it.
type Server struct {
	Host string
	Port int
	// KeyPath is the private key of the client, and KnownHosts holds the host key.
	KeyPath    string
	KnownHosts string
}

// Settings returns the tunnel settings that reach this server.
func (server Server) Settings() tunnel.Settings {
	return tunnel.Settings{
		Host: server.Host, Port: server.Port, User: User,
		KeyPath: server.KeyPath, KnownHosts: server.KnownHosts,
	}
}

// Start runs the server until the test ends. It writes the client key and the known hosts
// file into a temporary directory.
func Start(t *testing.T) Server {
	t.Helper()
	clientSigner, clientKey := BuildKeyPair(t)
	hostSigner, _ := BuildKeyPair(t)

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(
			_ ssh.ConnMetadata, offered ssh.PublicKey,
		) (*ssh.Permissions, error) {
			if string(offered.Marshal()) != string(clientSigner.PublicKey().Marshal()) {
				return nil, fmt.Errorf("unknown key")
			}
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go serveConnection(connection, config)
		}
	}()

	host, port := SplitAddress(t, listener.Addr().String())
	return Server{
		Host: host, Port: port,
		KeyPath:    WriteFile(t, "id_ed25519", clientKey),
		KnownHosts: WriteKnownHosts(t, listener.Addr().String(), hostSigner.PublicKey()),
	}
}

// BuildKeyPair returns one ed25519 key as a signer and as a PEM private key.
func BuildKeyPair(t *testing.T) (ssh.Signer, []byte) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("cannot build a key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("cannot sign with the key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatalf("cannot write the key: %v", err)
	}
	return signer, pem.EncodeToMemory(block)
}

// WriteFile writes one file into a temporary directory and returns its path.
func WriteFile(t *testing.T, name string, written []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, written, 0o600); err != nil {
		t.Fatalf("cannot write %s: %v", name, err)
	}
	return path
}

// WriteKnownHosts writes the host key of that address into a known hosts file.
func WriteKnownHosts(t *testing.T, address string, key ssh.PublicKey) string {
	t.Helper()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("cannot read the address: %v", err)
	}
	line := fmt.Sprintf("[%s]:%s %s\n", host, port,
		strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))))
	return WriteFile(t, "known_hosts", []byte(line))
}

// SplitAddress returns the host and port of an address.
func SplitAddress(t *testing.T, address string) (string, int) {
	t.Helper()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("cannot read the address: %v", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("cannot read the port: %v", err)
	}
	return host, number
}

// serveConnection serves one client and its channels.
func serveConnection(connection net.Conn, config *ssh.ServerConfig) {
	server, channels, requests, err := ssh.NewServerConn(connection, config)
	if err != nil {
		_ = connection.Close()
		return
	}
	defer func() { _ = server.Close() }()
	go ssh.DiscardRequests(requests)

	for held := range channels {
		if held.ChannelType() != "direct-tcpip" {
			_ = held.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		go serveForward(held)
	}
}

// serveForward dials the channel target and proxies the bytes both ways.
func serveForward(held ssh.NewChannel) {
	address, err := readForwardAddress(held.ExtraData())
	if err != nil {
		_ = held.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	target, err := net.Dial("tcp", address)
	if err != nil {
		_ = held.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	channel, requests, err := held.Accept()
	if err != nil {
		_ = target.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	go func() {
		defer func() { _ = target.Close() }()
		defer func() { _ = channel.Close() }()
		go func() { _, _ = io.Copy(target, channel) }()
		_, _ = io.Copy(channel, target)
	}()
}

// readForwardAddress returns the target of a direct-tcpip channel.
func readForwardAddress(data []byte) (string, error) {
	if len(data) < 4 {
		return "", fmt.Errorf("short channel request")
	}
	length := binary.BigEndian.Uint32(data[:4])
	if uint32(len(data)) < 4+length+4 {
		return "", fmt.Errorf("short channel request")
	}
	host := string(data[4 : 4+length])
	port := binary.BigEndian.Uint32(data[4+length : 8+length])
	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10)), nil
}
