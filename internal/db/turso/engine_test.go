package turso_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/turso"
)

func TestBuildSourceWritesTheWebsocketURL(t *testing.T) {
	cases := []struct {
		name     string
		profile  cfg.Profile
		password string
		wanted   string
	}{
		{
			name:     "a hosted database",
			profile:  cfg.Profile{Host: "shop-acme.turso.io", Port: 443, SSLMode: core.SSLRequire},
			password: "a-token",
			wanted:   "wss://shop-acme.turso.io:443?authToken=a-token",
		},
		{
			name:    "no port and no token",
			profile: cfg.Profile{Host: "shop-acme.turso.io", SSLMode: core.SSLRequire},
			wanted:  "wss://shop-acme.turso.io",
		},
		{
			name:    "a server without TLS",
			profile: cfg.Profile{Host: "127.0.0.1", Port: 8080, SSLMode: core.SSLDisable},
			wanted:  "ws://127.0.0.1:8080",
		},
		{
			name:    "an IPv6 address",
			profile: cfg.Profile{Host: "::1", Port: 8080, SSLMode: core.SSLDisable},
			wanted:  "ws://[::1]:8080",
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			written, err := turso.BuildSource(held.profile, held.password)
			if err != nil {
				t.Fatalf("the source answered %v", err)
			}
			if written != held.wanted {
				t.Errorf("the source is %q, wanted %q", written, held.wanted)
			}
		})
	}
}

func TestBuildSourceRefusesAProfileWithoutAHost(t *testing.T) {
	if _, err := turso.BuildSource(cfg.Profile{}, "a-token"); err == nil {
		t.Error("a profile without a host answered no error")
	}
}
