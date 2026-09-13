package cfg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// buildFormProfile returns a profile the connection form can be built from.
func buildFormProfile() cfg.Profile {
	return cfg.Profile{
		Name: "shop", Engine: core.EnginePostgres, Host: "db.example.com", Port: 6543,
		Database: "shop", User: "reader", Auth: cfg.AuthPassword,
		Environment: cfg.EnvironmentTest, AccessMode: cfg.AccessReadOnly,
		SSLMode: core.SSLVerifyFull, PageSize: 250,
	}
}

// The user edits the profile through the form, so the form must hold every value of the
// profile and convert back into the same profile.
func TestBuildProfileFromFieldsRoundTripsAProfile(t *testing.T) {
	source := buildFormProfile()
	fields := cfg.BuildFormFields(source, true, nil)
	if len(fields) == 0 {
		t.Fatal("the form holds no field")
	}

	held, err := cfg.BuildProfileFromFields(fields, source, true)
	if err != nil {
		t.Fatalf("the form does not read back: %v", err)
	}

	for _, one := range []struct {
		field string
		got   any
		want  any
	}{
		{"name", held.Name, source.Name},
		{"engine", held.Engine, source.Engine},
		{"host", held.Host, source.Host},
		{"port", held.Port, source.Port},
		{"database", held.Database, source.Database},
		{"user", held.User, source.User},
		{"environment", held.Environment, source.Environment},
		{"access", held.AccessMode, source.AccessMode},
		{"ssl mode", held.SSLMode, source.SSLMode},
		{"page size", held.PageSize, source.PageSize},
	} {
		if one.got != one.want {
			t.Errorf("the %s reads %v, wanted %v", one.field, one.got, one.want)
		}
	}
}

// An empty required field must be reported and not saved as a profile that cannot connect,
// because the user would see the problem only at the next connection.
func TestBuildProfileFromFieldsReportsAFieldThatIsNeeded(t *testing.T) {
	source := buildFormProfile()
	fields := cfg.ApplyFieldChange(cfg.BuildFormFields(source, true, nil), "name", "")

	if _, err := cfg.BuildProfileFromFields(fields, source, true); err == nil {
		t.Error("a profile with no name was built")
	}
}

// A MySQL connection needs no database, and the sidebar holds every database of the server,
// so the form takes a MySQL profile without one. An engine that opens one database still
// reports the missing name.
func TestBuildProfileFromFieldsTakesAMysqlProfileWithNoDatabase(t *testing.T) {
	source := buildFormProfile()
	source.Engine = core.EngineMysql
	fields := cfg.ApplyFieldChange(cfg.BuildFormFields(source, true, nil), "database", "")

	built, err := cfg.BuildProfileFromFields(fields, source, true)
	if err != nil {
		t.Fatalf("a MySQL profile with no database was refused: %v", err)
	}
	if built.Database != "" {
		t.Errorf("the profile opens %q, wanted no database", built.Database)
	}

	postgres := buildFormProfile()
	onePostgres := cfg.ApplyFieldChange(
		cfg.BuildFormFields(postgres, true, nil), "database", "")
	if _, err := cfg.BuildProfileFromFields(onePostgres, postgres, true); err == nil {
		t.Error("a PostgreSQL profile with no database was built")
	}
}

// A port that is not a number cannot be used, so the form reports it and does not save it.
func TestBuildProfileFromFieldsReportsAPortThatIsNotANumber(t *testing.T) {
	source := buildFormProfile()
	for _, written := range []string{"nope", "-1", "0"} {
		fields := cfg.ApplyFieldChange(cfg.BuildFormFields(source, true, nil), "port", written)
		if _, err := cfg.BuildProfileFromFields(fields, source, true); err == nil {
			t.Errorf("a port of %q was accepted", written)
		}
	}
}

// A field change modifies one field only, so an edit of the host does not clear the user.
func TestApplyFieldChangeTouchesOneFieldOnly(t *testing.T) {
	fields := cfg.BuildFormFields(buildFormProfile(), true, nil)
	before := cfg.ReadField(fields, "user")

	held := cfg.ApplyFieldChange(fields, "host", "other.example.com")
	if cfg.ReadField(held, "host") != "other.example.com" {
		t.Errorf("the host reads %q", cfg.ReadField(held, "host"))
	}
	if cfg.ReadField(held, "user") != before {
		t.Errorf("the user changed to %q", cfg.ReadField(held, "user"))
	}
}

// A URL pasted into the form fills the fields it contains, which is the fastest way to open
// a server from a URL.
func TestApplyConnectionUrlFillsTheFieldsItNames(t *testing.T) {
	held, is := cfg.ParseConnectionURL("postgres://reader@db.example.com:6543/shop")
	if !is {
		t.Fatal("the URL was not read")
	}

	fields := cfg.ApplyConnectionURL(cfg.BuildFormFields(cfg.Profile{}, false, nil), held)
	for _, one := range []struct{ key, want string }{
		{"host", "db.example.com"},
		{"port", "6543"},
		{"database", "shop"},
		{"user", "reader"},
	} {
		if answered := cfg.ReadField(fields, one.key); answered != one.want {
			t.Errorf("the %s reads %q, wanted %q", one.key, answered, one.want)
		}
	}
}

// A MySQL URL without a database names a server, so the paste fills the form and leaves the
// database field empty.
func TestApplyConnectionUrlTakesAMysqlUrlWithNoDatabase(t *testing.T) {
	held, is := cfg.ParseConnectionURL("mysql://reader@db.example.com:3306")
	if !is {
		t.Fatal("a MySQL URL without a database was not read")
	}

	fields := cfg.ApplyConnectionURL(cfg.BuildFormFields(cfg.Profile{}, false, nil), held)
	for _, one := range []struct{ key, want string }{
		{"host", "db.example.com"},
		{"port", "3306"},
		{"database", ""},
		{"user", "reader"},
	} {
		if answered := cfg.ReadField(fields, one.key); answered != one.want {
			t.Errorf("the %s reads %q, wanted %q", one.key, answered, one.want)
		}
	}
	// The name follows the database name, and an empty one leaves the name of a new
	// connection in place.
	if cfg.ReadField(fields, "name") == "" {
		t.Error("the paste cleared the name of the connection")
	}
}

// A field the form hides for one engine must not be required: a file has no host and no
// port, and the user would fill in a value that nothing uses.
func TestFindShownFieldsLeavesOutWhatAnEngineDoesNotNeed(t *testing.T) {
	file := cfg.BuildFormFields(cfg.Profile{
		Name: "notes", Engine: core.EngineSqlite, Database: "/tmp/notes.db",
	}, true, nil)
	shown := map[string]bool{}
	for _, field := range cfg.FindShownFields(file) {
		shown[field.Key] = true
	}
	if shown["host"] || shown["port"] {
		t.Error("a file is asked for a host or a port")
	}
	if !shown["database"] {
		t.Error("a file is not asked for its path")
	}

	// A server needs both fields.
	server := cfg.BuildFormFields(buildFormProfile(), true, nil)
	shownServer := map[string]bool{}
	for _, field := range cfg.FindShownFields(server) {
		shownServer[field.Key] = true
	}
	if !shownServer["host"] || !shownServer["port"] {
		t.Error("a server is not asked for a host or a port")
	}
}

// A profile written into the config file must read back as the same profile, or the form
// loses the values the user typed at the next start of the client.
func TestSaveProfileToFileWritesAProfileThatReadsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	source := buildFormProfile()

	if err := cfg.SaveProfileToFile(source, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 {
		t.Fatalf("the file that was written reports %v", loaded.Problems)
	}
	held := findProfile(t, loaded, source.Name)

	for _, one := range []struct {
		field string
		got   any
		want  any
	}{
		{"engine", held.Engine, source.Engine},
		{"host", held.Host, source.Host},
		{"port", held.Port, source.Port},
		{"database", held.Database, source.Database},
		{"user", held.User, source.User},
		{"environment", held.Environment, source.Environment},
		{"access", held.AccessMode, source.AccessMode},
		{"ssl mode", held.SSLMode, source.SSLMode},
	} {
		if one.got != one.want {
			t.Errorf("the %s read back as %v, wanted %v", one.field, one.got, one.want)
		}
	}

	if held.PageSize != source.PageSize {
		t.Errorf("the page size read back as %d, wanted %d", held.PageSize, source.PageSize)
	}
}

// A second profile is added to the file and does not replace the first.
func TestSaveProfileToFileKeepsTheProfilesAlreadyThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	first := buildFormProfile()
	if err := cfg.SaveProfileToFile(first, "", path); err != nil {
		t.Fatalf("the first profile was not written: %v", err)
	}
	second := buildFormProfile()
	second.Name = "warehouse"
	if err := cfg.SaveProfileToFile(second, "", path); err != nil {
		t.Fatalf("the second profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	findProfile(t, loaded, "shop")
	findProfile(t, loaded, "warehouse")
}

// A profile saved under a new name replaces the old one, so the file never contains both.
func TestSaveProfileToFileReplacesTheProfileItRenames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	source := buildFormProfile()
	if err := cfg.SaveProfileToFile(source, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	renamed := source
	renamed.Name = "shop_renamed"
	if err := cfg.SaveProfileToFile(renamed, source.Name, path); err != nil {
		t.Fatalf("the renamed profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	findProfile(t, loaded, "shop_renamed")
	for _, held := range loaded.Profiles {
		if held.Name == "shop" {
			t.Error("the profile it was renamed from was kept")
		}
	}
}

// The settings that are not profiles stay unchanged, because a save of one connection must
// not rewrite the theme.
func TestSaveProfileToFileKeepsTheRestOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[ui]\ntheme = \"nord\"\n"), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	if err := cfg.SaveProfileToFile(buildFormProfile(), "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the config file back: %v", err)
	}
	if !strings.Contains(string(written), "nord") {
		t.Errorf("the theme was lost:\n%s", written)
	}
	if cfg.LoadConfig(path).Settings.Theme != "nord" {
		t.Error("the theme does not read back")
	}
}

// The connection form shows a part of a profile only. A save from the form must keep the
// settings it does not show, or an edit of the host in the client would delete the page
// size, the keepalive and the pre-connect command from the file.
func TestSaveProfileToFileKeepsTheSettingsTheFormNeverShows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`[profile.shop]
engine = "postgres"
host = "db.example.com"
port = 6543
database = "shop"
user = "reader"
page_size = 250
keepalive_s = 30
autocommit = true
`), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	// The user edits the host in the form and saves. Everything else stays unchanged.
	loaded := cfg.LoadConfig(path)
	held := findProfile(t, loaded, "shop")
	held.Host = "other.example.com"

	if err := cfg.SaveProfileToFile(held, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	again := findProfile(t, cfg.LoadConfig(path), "shop")
	if again.Host != "other.example.com" {
		t.Errorf("the host reads %q, wanted the one edited", again.Host)
	}
	if again.PageSize != 250 {
		t.Errorf("the page size reads %d, wanted the 250 in the file", again.PageSize)
	}
	if again.Keepalive != 30*time.Second {
		t.Errorf("the keepalive reads %v, wanted the 30 seconds in the file", again.Keepalive)
	}
	if !again.Autocommit {
		t.Error("autocommit was lost")
	}
}

// The five password sources read alike in the form, so each one has a line of its own that
// says where the password comes from. A name that is not a source has none.
func TestDescribeAuthModeNamesEverySource(t *testing.T) {
	for _, mode := range cfg.AuthModes {
		if cfg.DescribeAuthMode(string(mode)) == "" {
			t.Errorf("the %q source has no line", mode)
		}
	}
	if held := cfg.DescribeAuthMode("nope"); held != "" {
		t.Errorf("a name that is no source reads %q", held)
	}
}

// The ssh toggle shows the tunnel fields. A profile without a tunnel hides them.
func TestFindShownFieldsHidesTheTunnelBehindTheToggle(t *testing.T) {
	fields := cfg.BuildFormFields(buildFormProfile(), true, nil)
	shown := map[string]bool{}
	for _, field := range cfg.FindShownFields(fields) {
		shown[field.Key] = true
	}
	if !shown["ssh"] {
		t.Fatal("the form has no ssh toggle")
	}
	if shown["sshHost"] || shown["sshUser"] {
		t.Error("a profile without a tunnel is asked for an ssh host")
	}

	opened := map[string]bool{}
	for _, field := range cfg.FindShownFields(cfg.ApplyFieldChange(fields, "ssh", "on")) {
		opened[field.Key] = true
	}
	for _, key := range []string{
		"sshHost", "sshPort", "sshUser", "sshKey",
		"sshKeyPassphraseEnv", "sshPasswordEnv", "sshKnownHosts",
	} {
		if !opened[key] {
			t.Errorf("the open toggle hides %s", key)
		}
	}
}

// A file engine opens no tunnel, so the form hides the toggle and the fields.
func TestFindShownFieldsHidesTheTunnelForAFileEngine(t *testing.T) {
	fields := cfg.BuildFormFields(cfg.Profile{
		Name: "notes", Engine: core.EngineSqlite, Database: "/tmp/notes.db",
		SSHHost: "ssh.example.com", SSHUser: "ada",
	}, true, nil)

	for _, field := range cfg.FindShownFields(fields) {
		if strings.HasPrefix(field.Key, "ssh") {
			t.Errorf("a file engine is asked for %s", field.Key)
		}
	}
}

// The tunnel fields build a profile. An empty port reads 22.
func TestBuildProfileFromFieldsTakesTheTunnel(t *testing.T) {
	fields := cfg.BuildFormFields(buildFormProfile(), true, nil)
	for key, value := range map[string]string{
		"ssh": "on", "sshHost": "ssh.example.com", "sshPort": "",
		"sshUser": "ada", "sshKey": "~/.ssh/id_ed25519",
		"sshKeyPassphraseEnv": "MASUME_SSH_PASSPHRASE",
		"sshKnownHosts":       "~/.ssh/known_hosts",
	} {
		fields = cfg.ApplyFieldChange(fields, key, value)
	}

	built, err := cfg.BuildProfileFromFields(fields, buildFormProfile(), true)
	if err != nil {
		t.Fatalf("the form reported %v", err)
	}
	for _, held := range []struct {
		field string
		got   any
		want  any
	}{
		{"ssh host", built.SSHHost, "ssh.example.com"},
		{"ssh port", built.SSHPort, 22},
		{"ssh user", built.SSHUser, "ada"},
		{"ssh key", built.SSHKey, "~/.ssh/id_ed25519"},
		{"passphrase env", built.SSHKeyPassphraseEnv, "MASUME_SSH_PASSPHRASE"},
		{"known hosts", built.SSHKnownHosts, "~/.ssh/known_hosts"},
	} {
		if held.got != held.want {
			t.Errorf("the %s reads %v, wanted %v", held.field, held.got, held.want)
		}
	}
}

// An off toggle clears the tunnel of the profile it edits.
func TestBuildProfileFromFieldsClearsTheTunnelTheToggleCloses(t *testing.T) {
	source := buildFormProfile()
	source.SSHHost, source.SSHPort, source.SSHUser = "ssh.example.com", 22, "ada"
	fields := cfg.ApplyFieldChange(
		cfg.BuildFormFields(source, true, nil), "ssh", "off")

	built, err := cfg.BuildProfileFromFields(fields, source, true)
	if err != nil {
		t.Fatalf("the form reported %v", err)
	}
	if built.OpensTunnel() || built.SSHUser != "" || built.SSHPort != 0 {
		t.Errorf("the profile keeps %s@%s:%d", built.SSHUser, built.SSHHost, built.SSHPort)
	}
}

// An on toggle needs a host and a user.
func TestBuildProfileFromFieldsReportsATunnelWithoutAUser(t *testing.T) {
	fields := cfg.ApplyFieldChange(cfg.ApplyFieldChange(
		cfg.BuildFormFields(buildFormProfile(), true, nil),
		"ssh", "on"), "sshHost", "ssh.example.com")

	if _, err := cfg.BuildProfileFromFields(fields, buildFormProfile(), true); err == nil {
		t.Fatal("a tunnel without a user was taken")
	} else if !strings.Contains(err.Error(), "ssh user") {
		t.Errorf("the form reported %q", err)
	}
}
