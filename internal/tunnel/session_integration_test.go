//go:build integration

// An integration test: it reads the real PostgreSQL named by MASUME_TEST_POSTGRES, through
// an ssh tunnel. The ssh server runs in this process and forwards to that server, so the
// test needs no ssh server of its own.
//
// The build tag keeps these off `go test ./...` entirely: without `-tags=integration` the
// file is not even compiled.
package tunnel_test

import (
	"context"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/tunnel/tunneltest"
)

// openTimeout is the time the connection through the tunnel has.
const openTimeout = 30 * time.Second

// A profile with an ssh tunnel opens a session, reads rows, and closes the tunnel with the
// connection. This is the whole path: the ssh settings of the profile, the tunnel, the dial
// address of the driver, and the statement.
func TestSessionRunsThroughTheTunnel(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	server := tunneltest.Start(t)

	profile.SSHHost, profile.SSHPort, profile.SSHUser = server.Host, server.Port, tunneltest.User
	profile.SSHKey, profile.SSHKnownHosts = server.KeyPath, server.KnownHosts

	dialed, preConnect, err := cfg.StartPreConnect(profile)
	if err != nil {
		t.Fatalf("the tunnel did not open: %v", err)
	}
	defer preConnect.Stop()

	if dialed.DialHost != "127.0.0.1" || dialed.DialPort == 0 {
		t.Fatalf("the driver dials %s:%d", dialed.DialHost, dialed.DialPort)
	}
	if dialed.Host != profile.Host || dialed.Port != profile.Port {
		t.Errorf("the profile now names %s:%d", dialed.Host, dialed.Port)
	}

	ctx, stop := context.WithTimeout(context.Background(), openTimeout)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, dialed, password)
	if err != nil {
		t.Fatalf("the server is unreachable through the tunnel: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.RunQuery(ctx, "select 1 as one", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the statement failed: %v", err)
	}
	if len(result.Rows) != 1 || len(result.Columns) != 1 {
		t.Fatalf("the result holds %d rows and %d columns",
			len(result.Rows), len(result.Columns))
	}

	// The catalog read is a second statement on the same connection, so the tunnel
	// carries more than the login.
	if _, err := session.ListTables(ctx); err != nil {
		t.Errorf("the catalog read failed: %v", err)
	}
}

// A closed tunnel ends the connection, so nothing keeps reading after the handle stops.
func TestStoppingTheHandleClosesTheSession(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	server := tunneltest.Start(t)

	profile.SSHHost, profile.SSHPort, profile.SSHUser = server.Host, server.Port, tunneltest.User
	profile.SSHKey, profile.SSHKnownHosts = server.KeyPath, server.KnownHosts

	dialed, preConnect, err := cfg.StartPreConnect(profile)
	if err != nil {
		t.Fatalf("the tunnel did not open: %v", err)
	}

	ctx, stop := context.WithTimeout(context.Background(), openTimeout)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, dialed, password)
	if err != nil {
		t.Fatalf("the server is unreachable through the tunnel: %v", err)
	}
	defer func() { _ = session.Close() }()

	preConnect.Stop()

	if _, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil); err == nil {
		t.Error("a statement ran after the tunnel closed")
	}
}
