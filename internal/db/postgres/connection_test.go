package postgres

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

func TestBuildPostgresTls(t *testing.T) {
	cases := []struct {
		mode        core.SSLMode
		encrypts    bool
		mayFallBack bool
		checks      bool
	}{
		// A profile that names no mode tries TLS and connects in the clear where the
		// server refuses it, which is what libpq calls `prefer`.
		{core.SSLUnset, true, true, false},
		{core.SSLPrefer, true, true, false},
		{core.SSLAllow, true, true, false},
		{core.SSLDisable, false, false, false},
		{core.SSLRequire, true, false, false},
		{core.SSLVerifyCa, true, false, true},
		{core.SSLVerifyFull, true, false, true},
	}
	for _, held := range cases {
		settings, mayFallBack := buildPostgresTLS(cfg.Profile{
			Host: "held.example", SSLMode: held.mode,
		})
		if (settings != nil) != held.encrypts {
			t.Errorf("%s encrypts %v, wanted %v", held.mode, settings != nil, held.encrypts)
		}
		if mayFallBack != held.mayFallBack {
			t.Errorf("%s falls back %v, wanted %v", held.mode, mayFallBack, held.mayFallBack)
		}
		if settings == nil {
			continue
		}
		checks := !settings.InsecureSkipVerify || settings.VerifyPeerCertificate != nil
		if checks != held.checks {
			t.Errorf("%s checks the certificate %v, wanted %v",
				held.mode, checks, held.checks)
		}
	}
}

func TestBuildPostgresConfigFallsBackToTheClear(t *testing.T) {
	config := buildPostgresConfig(cfg.Profile{
		Host: "held.example", Port: 5432, Database: "held", User: "held",
	}, "secret")
	if config.TLSConfig == nil {
		t.Fatal("a profile that names no mode did not try TLS")
	}
	if len(config.Fallbacks) != 1 || config.Fallbacks[0].TLSConfig != nil {
		t.Errorf("the fallbacks are %+v, wanted one without TLS", config.Fallbacks)
	}

	held := buildPostgresConfig(cfg.Profile{
		Host: "held.example", Port: 5432, SSLMode: core.SSLRequire,
	}, "secret")
	if len(held.Fallbacks) != 0 {
		t.Errorf("`require` has %d fallbacks, wanted none", len(held.Fallbacks))
	}
}

// A tunneled profile dials the local endpoint and verifies the certificate against the
// database host.
func TestBuildPostgresConfigDialsTheTunnel(t *testing.T) {
	profile := cfg.Profile{
		Host: "db.internal", Port: 5432, SSLMode: core.SSLVerifyFull,
		DialHost: "127.0.0.1", DialPort: 40000,
	}

	config := buildPostgresConfig(profile, "")
	if config.Host != "127.0.0.1" || config.Port != 40000 {
		t.Errorf("the driver dials %s:%d", config.Host, config.Port)
	}
	if config.TLSConfig == nil || config.TLSConfig.ServerName != "db.internal" {
		t.Errorf("the certificate is checked against %+v", config.TLSConfig)
	}
}

// A socket host dials the directory without TLS, because the server refuses TLS on a socket.
func TestBuildPostgresConfigDialsTheSocket(t *testing.T) {
	profile := cfg.Profile{
		Host: "/var/run/postgresql", Port: 5432, SSLMode: core.SSLVerifyFull,
	}

	config := buildPostgresConfig(profile, "")
	if config.Host != "/var/run/postgresql" || config.Port != 5432 {
		t.Errorf("the driver dials %s:%d", config.Host, config.Port)
	}
	if config.TLSConfig != nil || len(config.Fallbacks) != 0 {
		t.Error("the socket connection carries TLS")
	}
}
