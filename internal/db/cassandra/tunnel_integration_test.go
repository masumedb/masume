//go:build integration

package cassandra_test

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

// tunnelForwarder copies every connection it accepts to the server, and closes them all on
// stop.
type tunnelForwarder struct {
	listener    net.Listener
	guard       sync.Mutex
	connections []net.Conn
}

func startTunnelForwarder(t *testing.T, server string) *tunnelForwarder {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	forwarder := &tunnelForwarder{listener: listener}
	go func() {
		for {
			local, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			remote, dialErr := net.Dial("tcp", server)
			if dialErr != nil {
				_ = local.Close()
				continue
			}
			forwarder.guard.Lock()
			forwarder.connections = append(forwarder.connections, local, remote)
			forwarder.guard.Unlock()
			go func() { _, _ = io.Copy(remote, local); _ = remote.Close() }()
			go func() { _, _ = io.Copy(local, remote); _ = local.Close() }()
		}
	}()
	t.Cleanup(forwarder.stop)
	return forwarder
}

func (forwarder *tunnelForwarder) stop() {
	_ = forwarder.listener.Close()
	forwarder.guard.Lock()
	defer forwarder.guard.Unlock()
	for _, connection := range forwarder.connections {
		_ = connection.Close()
	}
}

func TestATunnelledSessionReachesTheClusterOnlyThroughTheTunnel(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Cassandra)
	forwarder := startTunnelForwarder(t,
		net.JoinHostPort(profile.Host, strconv.Itoa(profile.Port)))
	tunnelAddress := forwarder.listener.Addr().(*net.TCPAddr)
	profile.Host = "tunnel-only.invalid"
	profile.DialHost, profile.DialPort = "127.0.0.1", tunnelAddress.Port

	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatalf("the tunnelled session did not open: %v", err)
	}
	defer func() { _ = session.Close() }()
	if _, err := session.RunQuery(ctx, "select release_version from system.local", 1, nil); err != nil {
		t.Fatalf("the read through the tunnel answered %v", err)
	}

	forwarder.stop()
	if _, err := session.RunQuery(ctx, "select release_version from system.local", 1, nil); err == nil {
		t.Error("a read went to the cluster after the tunnel closed")
	}
}
