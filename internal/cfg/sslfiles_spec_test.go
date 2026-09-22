package cfg_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

// A managed database ships its own authority, and a server can ask for a client
// certificate. A profile carries the three paths.
func TestLoadConfigReadsTheCertificateFiles(t *testing.T) {
	path := writeConfig(t, `
[profile.rds]
engine      = "postgres"
host        = "shop.eu-central-1.rds.amazonaws.com"
database    = "shop"
user        = "reader"
sslmode     = "verify-full"
sslrootcert = "~/.certs/rds-ca.pem"
sslcert     = "~/.certs/client.pem"
sslkey      = "~/.certs/client.key"
`)
	profile := findProfile(t, cfg.LoadConfig(path), "rds")

	for _, held := range []struct {
		key  string
		got  string
		want string
	}{
		{"sslrootcert", profile.SSLRootCert, "~/.certs/rds-ca.pem"},
		{"sslcert", profile.SSLCert, "~/.certs/client.pem"},
		{"sslkey", profile.SSLKey, "~/.certs/client.key"},
	} {
		if held.got != held.want {
			t.Errorf("%s reads %q, wanted %q", held.key, held.got, held.want)
		}
	}
}

// A client certificate without its key cannot open a connection. The profile is skipped
// with a report, and the rest of the file still loads.
func TestLoadConfigSkipsAProfileWithACertificateAndNoKey(t *testing.T) {
	path := writeConfig(t, `
[profile.rds]
engine   = "postgres"
host     = "db.internal"
database = "shop"
user     = "reader"
sslcert  = "~/.certs/client.pem"
`)
	loaded := cfg.LoadConfig(path)

	if len(loaded.Profiles) != 0 {
		t.Fatalf("the load holds %d profiles, wanted none", len(loaded.Profiles))
	}
	if len(loaded.Problems) != 1 ||
		!strings.Contains(loaded.Problems[0].Reason, "sslcert and sslkey") {
		t.Errorf("the report reads %+v", loaded.Problems)
	}
}

// A connection URL carries the files. `masume "postgres://…"` then reaches a server with
// its own authority.
func TestBuildProfileFromTargetReadsTheCertificateFilesOfAURL(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"postgres://reader@db.internal/shop?sslmode=verify-full" +
			"&sslrootcert=/certs/ca.pem&sslcert=/certs/client.pem&sslkey=/certs/client.key")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}
	if built.SSLRootCert != "/certs/ca.pem" || built.SSLCert != "/certs/client.pem" ||
		built.SSLKey != "/certs/client.key" {
		t.Errorf("the files read %+v", built.BuildSSLFiles())
	}
}

// A keyword DSN uses the libpq names. A connection string from another client opens here
// unchanged.
func TestBuildProfileFromTargetReadsTheCertificateFilesOfADsn(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"host=db.internal dbname=shop user=reader sslmode=verify-ca " +
			"sslrootcert=/certs/ca.pem sslcert=/certs/client.pem sslkey=/certs/client.key")
	if err != nil {
		t.Fatalf("the connection string does not read: %v", err)
	}
	if built.SSLRootCert != "/certs/ca.pem" || built.SSLCert != "/certs/client.pem" ||
		built.SSLKey != "/certs/client.key" {
		t.Errorf("the files read %+v", built.BuildSSLFiles())
	}
}

func TestBuildProfileFromTargetRefusesACertificateWithoutAKey(t *testing.T) {
	if _, err := cfg.BuildProfileFromTarget(
		"postgres://reader@db.internal/shop?sslcert=/certs/client.pem"); err == nil {
		t.Error("a URL with a certificate and no key was accepted")
	}
}

// buildCertificateProfile returns a profile with the three paths set.
func buildCertificateProfile() cfg.Profile {
	profile := buildFormProfile()
	profile.SSLRootCert = "/certs/ca.pem"
	profile.SSLCert = "/certs/client.pem"
	profile.SSLKey = "/certs/client.key"
	return profile
}

// The form holds the files of the profile and reads them back unchanged.
func TestBuildProfileFromFieldsRoundTripsTheCertificateFiles(t *testing.T) {
	source := buildCertificateProfile()
	fields := cfg.BuildFormFields(source, true, nil)
	if cfg.ReadField(fields, "tls") != "on" {
		t.Errorf("the tls toggle reads %q, wanted on", cfg.ReadField(fields, "tls"))
	}

	held, err := cfg.BuildProfileFromFields(fields, source, true)
	if err != nil {
		t.Fatalf("the form does not read back: %v", err)
	}
	if held.BuildSSLFiles() != source.BuildSSLFiles() {
		t.Errorf("the files read %+v, wanted %+v",
			held.BuildSSLFiles(), source.BuildSSLFiles())
	}
}

// The toggle hides the fields of a profile without files, and an off toggle clears every
// path the profile held.
func TestFindShownFieldsHidesTheCertificateFields(t *testing.T) {
	fields := cfg.BuildFormFields(buildFormProfile(), true, nil)
	for _, field := range cfg.FindShownFields(fields) {
		if field.Key == "sslRootCert" || field.Key == "sslCert" || field.Key == "sslKey" {
			t.Errorf("the form shows %q with the toggle off", field.Key)
		}
	}

	cleared, err := cfg.BuildProfileFromFields(
		cfg.BuildFormFields(buildCertificateProfile(), true, nil),
		buildCertificateProfile(), true)
	if err != nil {
		t.Fatalf("the form does not read back: %v", err)
	}
	cleared, err = cfg.BuildProfileFromFields(
		cfg.ApplyFieldChange(cfg.BuildFormFields(cleared, true, nil), "tls", "off"),
		cleared, true)
	if err != nil {
		t.Fatalf("the form does not read back: %v", err)
	}
	if cleared.BuildSSLFiles().HasFiles() {
		t.Errorf("an off toggle kept %+v", cleared.BuildSSLFiles())
	}
}

// A form with a certificate and no key cannot open a connection.
func TestBuildProfileFromFieldsRefusesACertificateWithoutAKey(t *testing.T) {
	source := buildCertificateProfile()
	fields := cfg.ApplyFieldChange(cfg.BuildFormFields(source, true, nil), "sslKey", "")
	if _, err := cfg.BuildProfileFromFields(fields, source, true); err == nil {
		t.Error("a form with a certificate and no key was accepted")
	}
}

// A pasted connection string fills the certificate fields of the form.
func TestApplyConnectionURLFillsTheCertificateFields(t *testing.T) {
	held, parsed := cfg.ParseConnectionURL(
		"postgres://reader@db.internal/shop?sslmode=verify-full&sslrootcert=/certs/ca.pem")
	if !parsed {
		t.Fatal("the URL does not read")
	}
	filled := cfg.ApplyConnectionURL(cfg.BuildFormFields(cfg.Profile{}, false, nil), held)
	if cfg.ReadField(filled, "sslRootCert") != "/certs/ca.pem" {
		t.Errorf("the root cert reads %q", cfg.ReadField(filled, "sslRootCert"))
	}
	if cfg.ReadField(filled, "tls") != "on" {
		t.Errorf("the tls toggle reads %q, wanted on", cfg.ReadField(filled, "tls"))
	}
}

// The config file holds the files the form wrote, and clearing them removes the keys.
func TestSaveProfileToFileWritesTheCertificateFiles(t *testing.T) {
	profile := buildStoredProfile()
	profile.SSLMode = core.SSLVerifyFull
	profile.SSLRootCert = "/certs/ca.pem"
	profile.SSLCert = "/certs/client.pem"
	profile.SSLKey = "/certs/client.key"
	written := saveProfile(t, "", profile)

	for _, wanted := range []string{
		`sslrootcert = "/certs/ca.pem"`,
		`sslcert = "/certs/client.pem"`,
		`sslkey = "/certs/client.key"`,
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the file holds no %s:\n%s", wanted, written)
		}
	}

	profile.SSLRootCert, profile.SSLCert, profile.SSLKey = "", "", ""
	cleared := saveProfile(t, written, profile)
	for _, refused := range []string{"sslrootcert", "sslcert", "sslkey"} {
		if strings.Contains(cleared, refused) {
			t.Errorf("the file still holds %s:\n%s", refused, cleared)
		}
	}
}

// A password source, a TLS mode and a confirmation are names alone, so the form says what
// the one under the caret does.
func TestTheFormDescribesEveryChoiceOfAField(t *testing.T) {
	fields := cfg.BuildFormFields(buildCertificateProfile(), true, nil)
	for _, key := range []string{"auth", "sslMode", "confirmWrites"} {
		for _, field := range fields {
			if field.Key != key {
				continue
			}
			for _, choice := range field.Choices {
				if cfg.DescribeFormField(fields, key, choice) == "" {
					t.Errorf("%s %q says nothing about what it does", key, choice)
				}
			}
		}
	}
}

// The form steps the TLS mode through its values, so no one has to know the name of one.
func TestTheFormStepsThroughTheTLSModes(t *testing.T) {
	fields := cfg.BuildFormFields(buildFormProfile(), true, nil)
	choices := []string{}
	for _, field := range fields {
		if field.Key == "sslMode" {
			choices = field.Choices
		}
	}
	if len(choices) != len(core.SSLModes)+1 {
		t.Fatalf("the sslmode field offers %v", choices)
	}
	for _, mode := range core.SSLModes {
		if !slices.Contains(choices, string(mode)) {
			t.Errorf("the sslmode field does not offer %q", mode)
		}
	}
}

// A profile without a mode keeps the default of its engine, and the form says so rather
// than showing an empty row.
func TestTheFormShowsTheEngineDefaultForAnUnsetTLSMode(t *testing.T) {
	source := buildFormProfile()
	source.SSLMode = core.SSLUnset
	fields := cfg.BuildFormFields(source, true, nil)
	if held := cfg.ReadField(fields, "sslMode"); held != "default" {
		t.Errorf("the sslmode field reads %q, wanted default", held)
	}

	built, err := cfg.BuildProfileFromFields(fields, source, true)
	if err != nil {
		t.Fatalf("the form does not read back: %v", err)
	}
	if built.SSLMode != core.SSLUnset {
		t.Errorf("the mode reads %q, wanted the unset mode", built.SSLMode)
	}
}

// The file of a file engine is a path on this machine, and the picker of the form fills it.
func TestTakesFilePathCoversTheFileOfAFileEngine(t *testing.T) {
	sqlite := cfg.BuildFormFields(
		cfg.Profile{Name: "notes", Engine: core.EngineSqlite, Database: "notes.db"}, true, nil)
	if !cfg.TakesFilePath(sqlite, "database") {
		t.Error("the file of a SQLite connection is not picked")
	}

	server := cfg.BuildFormFields(buildFormProfile(), true, nil)
	if cfg.TakesFilePath(server, "database") {
		t.Error("the database of a server is picked as a file")
	}
	for _, key := range []string{"sslRootCert", "sslCert", "sslKey", "sshKey", "sshKnownHosts"} {
		if !cfg.TakesFilePath(server, key) {
			t.Errorf("%s is not picked as a file", key)
		}
	}
}

// The fields a label does not cover on its own keep a line of their own.
func TestDescribeFormFieldCoversTheFieldsALabelMisses(t *testing.T) {
	fields := cfg.BuildFormFields(buildCertificateProfile(), true, []string{"store"})
	for _, key := range []string{
		"passwordEnv", "passwordCommand", "secret", "secretRef", "environment",
		"tls", "sslRootCert", "sslCert", "sslKey", "ssh", "aiInstructions",
	} {
		if cfg.DescribeFormField(fields, key, cfg.ReadField(fields, key)) == "" {
			t.Errorf("%s says nothing about what it holds", key)
		}
	}
}
