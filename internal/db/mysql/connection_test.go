package mysql

import (
	"strings"
	"testing"

	driver "github.com/go-sql-driver/mysql"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// readMysqlTLS returns the TLS settings the driver opens this profile with.
func readMysqlTLS(t *testing.T, profile cfg.Profile) *driver.Config {
	t.Helper()
	written, err := buildMysqlDsn(profile, "secret")
	if err != nil {
		t.Fatalf("the dsn answered %v", err)
	}
	config, parseErr := driver.ParseDSN(written)
	if parseErr != nil {
		t.Fatalf("the driver refused the dsn %q: %v", written, parseErr)
	}
	return config
}

// The mode of a profile decides what the connection asks of the server. A profile that
// names no mode must still encrypt where the server offers it, because a connection in
// the clear carries the password and every row over the network.
func TestResolveMysqlTLSFollowsTheModeOfTheProfile(t *testing.T) {
	for _, held := range []struct {
		mode         core.SSLMode
		encrypts     bool
		maysFallBack bool
		checks       bool
	}{
		{core.SSLUnset, true, true, false},
		{core.SSLPrefer, true, true, false},
		{core.SSLAllow, true, true, false},
		{core.SSLDisable, false, false, false},
		{core.SSLRequire, true, false, false},
		{core.SSLVerifyCa, true, false, true},
		{core.SSLVerifyFull, true, false, true},
	} {
		config := readMysqlTLS(t, cfg.Profile{Host: "held.example", SSLMode: held.mode})
		if (config.TLS != nil) != held.encrypts {
			t.Errorf("%q encrypts %v, wanted %v", held.mode, config.TLS != nil, held.encrypts)
		}
		if config.AllowFallbackToPlaintext != held.maysFallBack {
			t.Errorf("%q falls back %v, wanted %v",
				held.mode, config.AllowFallbackToPlaintext, held.maysFallBack)
		}
		if config.TLS == nil {
			continue
		}
		checks := !config.TLS.InsecureSkipVerify || config.TLS.VerifyPeerCertificate != nil
		if checks != held.checks {
			t.Errorf("%q checks the certificate %v, wanted %v",
				held.mode, checks, held.checks)
		}
	}
}

// A certificate file the client cannot read stops the connection, and the message names
// the path.
func TestBuildMysqlDsnReportsAnUnreadableCertificate(t *testing.T) {
	_, err := buildMysqlDsn(cfg.Profile{
		Host: "held.example", SSLMode: core.SSLVerifyFull,
		SSLRootCert: "/held/absent.pem",
	}, "secret")
	if err == nil || !strings.Contains(err.Error(), "/held/absent.pem") {
		t.Errorf("the dsn answered %v, wanted the path of the missing file", err)
	}
}

// `require` encrypts and never falls back to the clear, so it takes settings of its own
// and not the name the driver reads as a fallback.
func TestResolveMysqlTLSNeverFallsBackWhereTheProfileRequiresTLS(t *testing.T) {
	for _, mode := range []core.SSLMode{core.SSLRequire, core.SSLVerifyCa} {
		config := readMysqlTLS(t, cfg.Profile{Host: "held.example", SSLMode: mode})
		if config.TLS == nil {
			t.Errorf("%q connects without TLS", mode)
		}
		if config.AllowFallbackToPlaintext {
			t.Errorf("%q may connect in the clear", mode)
		}
	}
}

// A socket host dials the file over the unix network, without TLS.
func TestBuildMysqlDsnDialsTheSocket(t *testing.T) {
	written, err := buildMysqlDsn(cfg.Profile{
		Host: "/var/run/mysqld/mysqld.sock", Port: 3306, User: "root",
		SSLMode: core.SSLVerifyFull,
	}, "secret")
	if err != nil {
		t.Fatalf("the dsn answered %v", err)
	}

	if !strings.Contains(written, "unix(/var/run/mysqld/mysqld.sock)") {
		t.Errorf("the dsn reads %q", written)
	}
	if !strings.Contains(written, "tls=false") {
		t.Errorf("the socket connection carries TLS: %q", written)
	}
}
