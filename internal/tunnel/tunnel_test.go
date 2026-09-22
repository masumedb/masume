package tunnel_test

import (
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/tunnel"
	"github.com/masumedb/masume/internal/tunnel/tunneltest"
)

// echoServer echoes every connection it accepts.
func echoServer(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = connection.Close() }()
				_, _ = io.Copy(connection, connection)
			}()
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

// The local endpoint forwards to the tunnel target.
func TestOpenForwardsToTheServer(t *testing.T) {
	server := tunneltest.Start(t)
	echo := echoServer(t)
	host, port := tunneltest.SplitAddress(t, echo.Addr().String())

	handle, err := tunnel.Open(server.Settings(), host, port)
	if err != nil {
		t.Fatalf("the tunnel did not open: %v", err)
	}
	defer handle.Stop()

	localHost, localPort := handle.Address()
	if localPort == 0 {
		t.Fatal("the tunnel opened no local port")
	}
	connection, err := net.Dial("tcp", net.JoinHostPort(localHost, strconv.Itoa(localPort)))
	if err != nil {
		t.Fatalf("the local port refused a connection: %v", err)
	}
	defer func() { _ = connection.Close() }()

	if _, err := connection.Write([]byte("ping")); err != nil {
		t.Fatalf("the write failed: %v", err)
	}
	held := make([]byte, 4)
	if _, err := io.ReadFull(connection, held); err != nil {
		t.Fatalf("the read failed: %v", err)
	}
	if string(held) != "ping" {
		t.Errorf("the tunnel answered %q", held)
	}
}

// An unknown host key fails the dial.
func TestOpenRefusesAnUnknownHostKey(t *testing.T) {
	server := tunneltest.Start(t)
	other, _ := tunneltest.BuildKeyPair(t)

	settings := server.Settings()
	settings.KnownHosts = tunneltest.WriteKnownHosts(t,
		net.JoinHostPort(server.Host, strconv.Itoa(server.Port)), other.PublicKey())

	handle, err := tunnel.Open(settings, "127.0.0.1", 1)
	if err == nil {
		handle.Stop()
		t.Fatal("the tunnel opened on an unknown host key")
	}
	if !strings.Contains(err.Error(), "ssh tunnel") {
		t.Errorf("the error reads %q", err)
	}
}

// A missing known hosts file fails with the path in the error.
func TestOpenReportsAMissingKnownHostsFile(t *testing.T) {
	server := tunneltest.Start(t)
	missing := filepath.Join(t.TempDir(), "absent")

	settings := server.Settings()
	settings.KnownHosts = missing
	if _, err := tunnel.Open(settings, "127.0.0.1", 1); err == nil {
		t.Fatal("a missing known hosts file opened a tunnel")
	} else if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error reads %q", err)
	}
}

// Without a key, a password and an agent the error names the settings.
func TestOpenReportsAMissingLogin(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	settings := tunnel.Settings{Host: "127.0.0.1", Port: 22, User: tunneltest.User}

	if _, err := tunnel.Open(settings, "127.0.0.1", 5432); err == nil {
		t.Fatal("a tunnel without a login opened")
	} else if !strings.Contains(err.Error(), "ssh_key") {
		t.Errorf("the error reads %q", err)
	}
}

// Stop closes the local listener.
func TestStopClosesTheLocalPort(t *testing.T) {
	server := tunneltest.Start(t)
	echo := echoServer(t)
	host, port := tunneltest.SplitAddress(t, echo.Addr().String())

	handle, err := tunnel.Open(server.Settings(), host, port)
	if err != nil {
		t.Fatalf("the tunnel did not open: %v", err)
	}
	localHost, localPort := handle.Address()

	handle.Stop()
	handle.Stop()

	if connection, err := net.Dial(
		"tcp", net.JoinHostPort(localHost, strconv.Itoa(localPort))); err == nil {
		_ = connection.Close()
		t.Error("the local port still accepts connections")
	}
}
