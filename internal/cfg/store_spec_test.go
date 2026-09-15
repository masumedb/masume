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

// buildStoredProfile returns a profile the form could have written.
func buildStoredProfile() cfg.Profile {
	return cfg.Profile{
		Name: "shop", Engine: core.EnginePostgres, Host: "127.0.0.1", Port: 5432,
		Database: "shop", User: "you", Auth: cfg.AuthPassword,
		Environment: cfg.EnvironmentDev, AccessMode: cfg.AccessWrite,
	}
}

// saveProfile writes the profile into a file with that text, and returns the new text.
func saveProfile(t *testing.T, body string, profile cfg.Profile) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}
	if err := cfg.SaveProfileToFile(profile, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the config file was not read back: %v", err)
	}
	return string(written)
}

func TestSaveProfileToFilePreservesNewProfileSettings(t *testing.T) {
	for _, replacing := range []string{"", "old-name"} {
		for _, disabled := range []bool{false, true} {
			t.Run(replacing+"/"+map[bool]string{false: "enabled", true: "disabled"}[disabled], func(t *testing.T) {
				profile, err := cfg.BuildProfileFromTarget("postgres://reader@localhost/shop")
				if err != nil {
					t.Fatal(err)
				}
				profile.McpAccess = cfg.McpOff
				profile.WritePlan = cfg.PlanUndo
				profile.UndoRows = 42
				profile.StatementTimeout = 1250 * time.Millisecond
				profile.Autocommit = false
				profile.PageSize = 71
				profile.Keepalive = 17 * time.Second
				profile.Command = "ssh -N -L 15432:localhost:5432 bastion"
				profile.WaitForPort = 15432
				profile.CommandTimeout = 23 * time.Second
				if disabled {
					profile.Environment = cfg.EnvironmentProd
					profile.WritePlan = cfg.PlanOff
					profile.UndoRows = 0
					profile.Keepalive = 0
					profile.StatementTimeout = 0
					profile.Autocommit = true
				}
				path := writeConfig(t, "# user settings\n[ui]\ntheme = \"dark\"\n")
				if err := cfg.SaveProfileToFile(profile, replacing, path); err != nil {
					t.Fatal(err)
				}
				loaded := cfg.LoadConfig(path)
				if len(loaded.Problems) != 0 {
					t.Fatalf("reload problems: %v", loaded.Problems)
				}
				profile.InConfigFile = true
				if reloaded := findProfile(t, loaded, profile.Name); reloaded != profile {
					t.Errorf("reloaded profile: %+v\nwant: %+v", reloaded, profile)
				}
			})
		}
	}
}

func TestSaveProfileToFilePreservesProjectGuards(t *testing.T) {
	projectPath := writeProjectFile(t, t.TempDir(), `
[profile.shop]
engine = "postgres"
host = "localhost"
database = "shop"
user = "reader"
env = "prod"
mode = "read-only"
confirm_writes = "agent"
mcp = "off"
write_plan = "count"
undo_rows = 37
statement_timeout_ms = 1500
autocommit = false
page_size = 53
keepalive_s = 0
`)
	project := cfg.LoadProjectConfig(projectPath)
	if len(project.Problems) != 0 {
		t.Fatalf("project problems: %v", project.Problems)
	}
	profile := findProjectProfile(t, project, "shop")
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := cfg.SaveProfileToFile(profile, "", path); err != nil {
		t.Fatal(err)
	}
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 {
		t.Fatalf("reload problems: %v", loaded.Problems)
	}
	profile.ProjectFile = ""
	profile.InConfigFile = true
	if reloaded := findProfile(t, loaded, "shop"); reloaded != profile {
		t.Errorf("reloaded profile: %+v\nwant: %+v", reloaded, profile)
	}
}

func TestSaveProfileToFilePreservesExistingSettingsAndComments(t *testing.T) {
	for _, operation := range []string{"edit", "rename", "replace"} {
		t.Run(operation, func(t *testing.T) {
			body := `
[profile.shop]
engine = "postgres"
host = "localhost"
database = "shop"
user = "reader"
mcp = "off" # no MCP access
write_plan = "undo" # read the previous rows
undo_rows = 0 # internal capture ceiling
statement_timeout_ms = 1250 # statement limit
autocommit = false # manual commit
page_size = 71 # rows per page
keepalive_s = 0 # no keepalive
command = "start-tunnel" # preconnect
wait_for_port = 15432 # tunnel port
command_timeout = 23 # tunnel limit
`
			path := writeConfig(t, body)
			profile := buildStoredProfile()
			replacing := ""
			if operation == "rename" {
				replacing = "shop"
				profile.Name = "renamed"
			}
			if operation == "replace" {
				body += "\n[profile.old]\nengine = \"sqlite\"\ndatabase = \"old.db\"\n"
				path = writeConfig(t, body)
				replacing = "old"
			}
			if err := cfg.SaveProfileToFile(profile, replacing, path); err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(body, "\n") {
				if strings.Contains(line, " # ") && !strings.Contains(string(written), line) {
					t.Errorf("missing original line: %s", line)
				}
			}
			loaded := cfg.LoadConfig(path)
			if len(loaded.Problems) != 0 || len(loaded.Profiles) != 1 {
				t.Fatalf("reload: %+v", loaded)
			}
			reloaded := findProfile(t, loaded, profile.Name)
			if reloaded.McpAccess != cfg.McpOff || reloaded.WritePlan != cfg.PlanUndo ||
				reloaded.UndoRows != 0 || reloaded.StatementTimeout != 1250*time.Millisecond ||
				reloaded.Autocommit || reloaded.PageSize != 71 || reloaded.Keepalive != 0 ||
				reloaded.Command != "start-tunnel" || reloaded.WaitForPort != 15432 ||
				reloaded.CommandTimeout != 23*time.Second {
				t.Errorf("changed settings: %+v", reloaded)
			}
		})
	}
}

// A password the user cleared must be deleted from the file. A line left behind keeps the
// connection on the old password and stores it on disk.
// A MySQL connection needs no database. The saved profile carries no database key, and it
// loads again as the profile that was written.
func TestSaveProfileToFileWritesAMysqlProfileWithNoDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	profile := buildStoredProfile()
	profile.Engine = core.EngineMysql
	profile.Port = 3306
	profile.Database = ""

	if err := cfg.SaveProfileToFile(profile, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the config file was not read back: %v", err)
	}
	if strings.Contains(string(written), "database") {
		t.Errorf("the file holds a database key:\n%s", written)
	}

	again := findProfile(t, cfg.LoadConfig(path), "shop")
	if again.Database != "" {
		t.Errorf("the profile reads %q, wanted no database", again.Database)
	}
	if again.Engine != core.EngineMysql {
		t.Errorf("the engine reads %q, wanted mysql", again.Engine)
	}
}

func TestSaveProfileToFileTakesOutAValueTheFormCleared(t *testing.T) {
	written := saveProfile(t, `
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"
password = "old-secret"
`, buildStoredProfile())

	if strings.Contains(written, "old-secret") {
		t.Errorf("the old password is still in the file:\n%s", written)
	}
	if strings.Contains(written, "password =") {
		t.Errorf("the password line is still in the file:\n%s", written)
	}
}

// A setting the form does not show keeps its line, so an edit of a connection never deletes
// the page size or a comment from the file.
func TestSaveProfileToFileKeepsWhatTheFormNeverShowed(t *testing.T) {
	written := saveProfile(t, `
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"
page_size = 500                      # as many rows as this screen draws
statement_timeout_ms = 30000
`, buildStoredProfile())

	for _, wanted := range []string{"page_size = 500", "statement_timeout_ms = 30000",
		"as many rows as this screen draws"} {
		if !strings.Contains(written, wanted) {
			t.Errorf("%q left the file:\n%s", wanted, written)
		}
	}
}

// A rename keeps the settings the form does not show, in the same way as an edit. A rename
// that deleted the block and wrote a new one would lose `mcp`, and the profile would use the
// access level of the whole server.
func TestSaveProfileToFileKeepsWhatTheFormNeverShowedThroughARename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"
# an agent may only read this one
mcp = "read-only"
page_size = 500
`), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	renamed := buildStoredProfile()
	renamed.Name = "shop-prod"
	if err := cfg.SaveProfileToFile(renamed, "shop", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the config file was not read back: %v", err)
	}
	written := string(held)

	if !strings.Contains(written, "[profile.shop-prod]") {
		t.Errorf("the profile was not renamed:\n%s", written)
	}
	if strings.Contains(written, "[profile.shop]") {
		t.Errorf("the old name is still in the file:\n%s", written)
	}
	for _, wanted := range []string{`mcp = "read-only"`, "page_size = 500",
		"an agent may only read this one"} {
		if !strings.Contains(written, wanted) {
			t.Errorf("%q left the file on a rename:\n%s", wanted, written)
		}
	}

	// The file still contains one profile with the new name.
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) > 0 {
		t.Fatalf("the file does not read back: %+v", loaded.Problems)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].Name != "shop-prod" {
		t.Fatalf("the file holds %d profiles: %+v", len(loaded.Profiles), loaded.Profiles)
	}
	if loaded.Profiles[0].McpAccess != cfg.McpReadOnly {
		t.Errorf("the mcp level reads %q after the rename", loaded.Profiles[0].McpAccess)
	}
}

// A rename to a name that is already in the file writes into the block that stays, so the
// file never contains two blocks with one name.
func TestSaveProfileToFileRenamingOntoANameTheFileHolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"

[profile.shop-prod]
engine = "postgres"
host = "old.example.com"
port = 5432
database = "shop"
user = "you"
`), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	renamed := buildStoredProfile()
	renamed.Name = "shop-prod"
	renamed.Host = "new.example.com"
	if err := cfg.SaveProfileToFile(renamed, "shop", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) > 0 {
		t.Fatalf("the file does not read back: %+v", loaded.Problems)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].Name != "shop-prod" {
		t.Fatalf("the file holds %d profiles: %+v", len(loaded.Profiles), loaded.Profiles)
	}
	if loaded.Profiles[0].Host != "new.example.com" {
		t.Errorf("the host reads %q, wanted the one just written", loaded.Profiles[0].Host)
	}
}

// The file is written to a temporary file in the same directory and then moved over the
// target, so a write that fails part way leaves the old file complete. No temporary file can
// stay behind, and only the owner can read the file.
func TestSaveProfileToFileLeavesNoHalfWrittenFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(path, []byte("[profile.other]\nengine = \"sqlite\"\n"), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}
	if err := cfg.SaveProfileToFile(buildStoredProfile(), "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	left, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("cannot read the directory: %v", err)
	}
	if len(left) != 1 || left[0].Name() != "config.toml" {
		names := []string{}
		for _, entry := range left {
			names = append(names, entry.Name())
		}
		t.Errorf("the directory holds %v, wanted the config file alone", names)
	}

	found, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("cannot read the config file back: %v", statErr)
	}
	if held := found.Mode().Perm(); held != 0o600 {
		t.Errorf("the config file is written %o, wanted 600", held)
	}
	written, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("cannot read the config file back: %v", readErr)
	}
	for _, wanted := range []string{"[profile.other]", "[profile.shop]"} {
		if !strings.Contains(string(written), wanted) {
			t.Errorf("%q left the file:\n%s", wanted, written)
		}
	}
}

// The form writes the tunnel of a profile. An off toggle takes every ssh key out of the
// file again.
func TestSaveProfileToFileWritesAndClearsTheTunnel(t *testing.T) {
	profile := buildStoredProfile()
	profile.SSHHost, profile.SSHPort, profile.SSHUser = "ssh.example.com", 2222, "ada"
	profile.SSHKey = "~/.ssh/id_ed25519"

	written := saveProfile(t, "", profile)
	for _, wanted := range []string{
		`ssh_host = "ssh.example.com"`, "ssh_port = 2222",
		`ssh_user = "ada"`, `ssh_key = "~/.ssh/id_ed25519"`,
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the file holds no %s:\n%s", wanted, written)
		}
	}

	cleared := saveProfile(t, written, buildStoredProfile())
	if strings.Contains(cleared, "ssh_") {
		t.Errorf("the file keeps an ssh key:\n%s", cleared)
	}
}

// saveTables writes the tables into a file with that text, and returns the new text.
func saveTables(t *testing.T, body string, updates []cfg.TableUpdate) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}
	if err := cfg.SaveTables(path, updates); err != nil {
		t.Fatalf("the tables were not written: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the config file: %v", err)
	}
	return string(written)
}

// A table the file has not got is added at the end, under the lines it already holds.
func TestSaveTablesAddsATableTheFileHasNot(t *testing.T) {
	written := saveTables(t, "[ui]\ntheme = \"ayu-dark\"\n", []cfg.TableUpdate{{
		Header: []string{"ai"},
		Order:  []string{"enabled", "default_provider"},
		Values: map[string]any{"enabled": true, "default_provider": "openai"},
	}})

	wanted := "[ui]\ntheme = \"ayu-dark\"\n\n[ai]\nenabled = true\ndefault_provider = \"openai\"\n"
	if written != wanted {
		t.Errorf("the file reads:\n%s\nwanted:\n%s", written, wanted)
	}
}

// A key the file already has keeps its line, so the layout of the file survives the save. A
// key the file has not got is added after them, in the order of the update.
func TestSaveTablesKeepsTheLineOfAKeyTheFileHas(t *testing.T) {
	written := saveTables(t, "[ai]\ndefault_provider = \"anthropic\"\nenabled = false\n",
		[]cfg.TableUpdate{{
			Header: []string{"ai"},
			Order:  []string{"enabled", "default_provider", "statement_timeout_ms"},
			Values: map[string]any{
				"enabled": true, "default_provider": "openai",
				"statement_timeout_ms": 45000,
			},
		}})

	wanted := "[ai]\ndefault_provider = \"openai\"\nenabled = true\n" +
		"statement_timeout_ms = 45000\n"
	if written != wanted {
		t.Errorf("the file reads:\n%s\nwanted:\n%s", written, wanted)
	}
}

// A key of the update with no value is removed, and a key outside the update stays.
func TestSaveTablesRemovesAClearedKeyAndKeepsTheOthers(t *testing.T) {
	written := saveTables(t, strings.Join([]string{
		"[ai.providers.anthropic]",
		"# the key of the team",
		"api_key = \"sk-written\"",
		"api_key_env = \"ANTHROPIC_API_KEY\"",
		"model = \"claude-opus-5\"",
		"",
	}, "\n"), []cfg.TableUpdate{{
		Header: []string{"ai", "providers", "anthropic"},
		Order:  []string{"model", "api_key_env"},
		Values: map[string]any{"model": "claude-sonnet-5"},
	}})

	for _, wanted := range []string{
		"# the key of the team", "api_key = \"sk-written\"",
		"model = \"claude-sonnet-5\"",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the file does not hold %q:\n%s", wanted, written)
		}
	}
	if strings.Contains(written, "api_key_env") {
		t.Errorf("the cleared key is still there:\n%s", written)
	}
}

// Two tables are written in one pass, and the lines between them stay unchanged.
func TestSaveTablesWritesEveryTableInOnePass(t *testing.T) {
	written := saveTables(t, strings.Join([]string{
		"[ai]",
		"enabled = false",
		"",
		"[profile.shop]",
		"engine = \"postgres\"",
		"",
		"[ai.providers.openai]",
		"model = \"gpt-5\"",
		"",
	}, "\n"), []cfg.TableUpdate{
		{
			Header: []string{"ai"}, Order: []string{"enabled"},
			Values: map[string]any{"enabled": true},
		},
		{
			Header: []string{"ai", "providers", "openai"}, Order: []string{"model"},
			Values: map[string]any{"model": "gpt-5-mini"},
		},
	})

	for _, wanted := range []string{
		"enabled = true", "[profile.shop]", "engine = \"postgres\"", "model = \"gpt-5-mini\"",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the file does not hold %q:\n%s", wanted, written)
		}
	}
}

// A file that is not valid TOML is never written over, because the write would drop what
// the user meant to keep.
func TestSaveTablesRefusesAFileItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[ai\nenabled = true\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	err := cfg.SaveTables(path, []cfg.TableUpdate{{
		Header: []string{"ai"}, Order: []string{"enabled"},
		Values: map[string]any{"enabled": false},
	}})
	if err == nil {
		t.Fatal("a file that does not read was written over")
	}
	written, _ := os.ReadFile(path)
	if string(written) != body {
		t.Errorf("the file changed:\n%s", written)
	}
}

// A table that is removed takes the blank row over it, so the file reads as though the
// table was never written: one blank row between the blocks that remain, and no blank row
// left at either end.
func TestRemovingATableLeavesTheFileTidy(t *testing.T) {
	body := "[a]\nx = 1\n\n[b]\ny = 2\n\n[c]\nz = 3\n"
	for _, held := range []struct {
		name    string
		updates []cfg.TableUpdate
		wanted  string
	}{
		{"the middle table", []cfg.TableUpdate{{Header: []string{"b"}, Remove: true}},
			"[a]\nx = 1\n\n[c]\nz = 3\n"},
		{"the last table", []cfg.TableUpdate{{Header: []string{"c"}, Remove: true}},
			"[a]\nx = 1\n\n[b]\ny = 2\n"},
		{"the first table", []cfg.TableUpdate{{Header: []string{"a"}, Remove: true}},
			"[b]\ny = 2\n\n[c]\nz = 3\n"},
		{"one table written and another removed", []cfg.TableUpdate{
			{Header: []string{"d"}, Order: []string{"w"},
				Values: map[string]any{"w": 4}},
			{Header: []string{"c"}, Remove: true},
		}, "[a]\nx = 1\n\n[b]\ny = 2\n\n[d]\nw = 4\n"},
	} {
		t.Run(held.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("cannot write the config file: %v", err)
			}
			if err := cfg.SaveTables(path, held.updates); err != nil {
				t.Fatalf("the tables were not written: %v", err)
			}
			written, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("cannot read the config file: %v", err)
			}
			if string(written) != held.wanted {
				t.Errorf("the file reads %q, wanted %q", written, held.wanted)
			}
			if _, err := cfg.DecodeDocument(string(written)); err != nil {
				t.Errorf("the file does not read: %v", err)
			}
		})
	}
}
