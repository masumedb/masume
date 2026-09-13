package cfg_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestLoadConfigReadsTheSSHTunnel(t *testing.T) {
	path := writeConfig(t, `
[profile.prod]
engine = "postgres"
host = "db.internal"
port = 5432
database = "shop"
user = "reader"
ssh_host = "bastion.example.com"
ssh_port = 2222
ssh_user = "ada"
ssh_key = "~/.ssh/id_ed25519"
ssh_key_passphrase_env = "MASUME_SSH_PASSPHRASE"
ssh_known_hosts = "~/.ssh/known_hosts"
`)

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 {
		t.Fatalf("the load reported %v", loaded.Problems)
	}

	profile := findProfile(t, loaded, "prod")
	for _, held := range []struct {
		field string
		got   any
		want  any
	}{
		{"ssh host", profile.SSHHost, "bastion.example.com"},
		{"ssh port", profile.SSHPort, 2222},
		{"ssh user", profile.SSHUser, "ada"},
		{"ssh key", profile.SSHKey, "~/.ssh/id_ed25519"},
		{"passphrase env", profile.SSHKeyPassphraseEnv, "MASUME_SSH_PASSPHRASE"},
		{"known hosts", profile.SSHKnownHosts, "~/.ssh/known_hosts"},
		{"opens a tunnel", profile.OpensTunnel(), true},
	} {
		if held.got != held.want {
			t.Errorf("the %s reads %v, wanted %v", held.field, held.got, held.want)
		}
	}
}

// A tunnel without a port uses port 22.
func TestLoadConfigGivesTheTunnelItsDefaultPort(t *testing.T) {
	path := writeConfig(t, `
[profile.prod]
engine = "postgres"
host = "db.internal"
database = "shop"
user = "reader"
ssh_host = "bastion.example.com"
ssh_user = "ada"
`)

	profile := findProfile(t, cfg.LoadConfig(path), "prod")
	if profile.SSHPort != 22 {
		t.Errorf("the ssh port reads %d", profile.SSHPort)
	}
}

// ssh_host without ssh_user skips the profile.
func TestLoadConfigSkipsATunnelWithoutAUser(t *testing.T) {
	path := writeConfig(t, `
[profile.prod]
engine = "postgres"
host = "db.internal"
database = "shop"
user = "reader"
ssh_host = "bastion.example.com"
`)

	loaded := cfg.LoadConfig(path)
	if len(loaded.Profiles) != 0 {
		t.Fatalf("the load kept %d profiles", len(loaded.Profiles))
	}
	if len(loaded.Problems) != 1 || !strings.Contains(loaded.Problems[0].Reason, "ssh_user") {
		t.Errorf("the load reported %v", loaded.Problems)
	}
}

// Without a tunnel the driver endpoint is the server.
func TestDialAddressReadsTheServerWithoutATunnel(t *testing.T) {
	profile := cfg.Profile{Host: "db.internal", Port: 5432}

	host, port := profile.DialAddress()
	if host != "db.internal" || port != 5432 {
		t.Errorf("the driver dials %s:%d", host, port)
	}
	if profile.OpensTunnel() {
		t.Error("a profile without an ssh host opens a tunnel")
	}
}

// With a tunnel the driver endpoint is local, and the target text keeps the server.
func TestDialAddressReadsTheOpenTunnel(t *testing.T) {
	profile := cfg.Profile{
		Host: "db.internal", Port: 5432, DialHost: "127.0.0.1", DialPort: 40000,
	}

	host, port := profile.DialAddress()
	if host != "127.0.0.1" || port != 40000 {
		t.Errorf("the driver dials %s:%d", host, port)
	}
	if cfg.DescribeProfileTarget(profile) != "@db.internal:5432" {
		t.Errorf("the picker shows %q", cfg.DescribeProfileTarget(profile))
	}
}

// StartPreConnect leaves a profile without a tunnel unchanged.
func TestStartPreConnectLeavesAProfileWithoutATunnel(t *testing.T) {
	profile := cfg.Profile{Name: "shop", Host: "db.internal", Port: 5432}

	dialed, handle, err := cfg.StartPreConnect(profile)
	if err != nil {
		t.Fatalf("the profile did not open: %v", err)
	}
	defer handle.Stop()

	if dialed.DialHost != "" || dialed.DialPort != 0 {
		t.Errorf("the profile dials %s:%d", dialed.DialHost, dialed.DialPort)
	}
}

// An SSH endpoint that refuses the connection fails the profile.
func TestStartPreConnectReportsATunnelItCannotOpen(t *testing.T) {
	profile := cfg.Profile{
		Name: "prod", Host: "db.internal", Port: 5432,
		SSHHost: "127.0.0.1", SSHPort: 1, SSHUser: "ada",
		SSHKnownHosts: writeConfig(t, ""),
	}

	if _, _, err := cfg.StartPreConnect(profile); err == nil {
		t.Fatal("a tunnel to a closed port opened")
	}
}
