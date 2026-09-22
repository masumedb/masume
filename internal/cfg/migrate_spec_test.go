package cfg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
)

// buildOldConfigFile writes a config file of the kind a client with no version wrote.
func buildOldConfigFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "# the config file of the user\n\n[profile.shop]\nengine = \"postgres\"\n\n" +
		"[ui]\ntheme = \"ayu-dark\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}
	return path
}

// readConfigFileText returns what a config file holds.
func readConfigFileText(t *testing.T, path string) string {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the config file: %v", err)
	}
	return string(written)
}

// A file written before the client had a version gains the providers and the agents, and
// keeps every line it had.
func TestMigrateConfigFileAddsTheAiTablesToAnOldFile(t *testing.T) {
	path := buildOldConfigFile(t)

	version, err := cfg.MigrateConfigFile(path)
	if err != nil {
		t.Fatalf("the migration failed: %v", err)
	}
	if version != cfg.ConfigVersion {
		t.Errorf("the file reads version %d", version)
	}

	written := readConfigFileText(t, path)
	for _, wanted := range []string{
		"# the config file of the user", "[profile.shop]", "theme = \"ayu-dark\"",
		"[ai.providers.anthropic]", "[ai.providers.openai]",
		"[ai.agents.claude]", "[ai.agents.codex]", "[ai.agents.opencode]",
		"[masume]", "version = 1",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the file holds no %q:\n%s", wanted, written)
		}
	}

	document, err := cfg.DecodeDocument(written)
	if err != nil {
		t.Fatalf("the migrated file does not read: %v", err)
	}
	config := cfg.ParseAiConfig(document)
	if len(config.Problems) != 0 {
		t.Errorf("the migrated file reports %v", config.Problems)
	}
	for _, name := range []string{"claude", "codex", "gemini", "opencode"} {
		if _, held := config.Agents[name]; !held {
			t.Errorf("the migrated file names no %s agent", name)
		}
	}
	if held := config.Agents["claude"]; len(held.Env) != 1 {
		t.Errorf("the claude agent reads %+v", held)
	}
	if held := config.Providers[cfg.ProviderAnthropic]; held.Model == "" ||
		held.APIKeyEnv == "" {
		t.Errorf("the anthropic provider reads %+v", held)
	}
}

// The copy of the file is taken before it is changed, under the version it was written at.
func TestMigrateConfigFileKeepsACopyOfTheOldFile(t *testing.T) {
	path := buildOldConfigFile(t)
	body := readConfigFileText(t, path)

	if _, err := cfg.MigrateConfigFile(path); err != nil {
		t.Fatalf("the migration failed: %v", err)
	}
	if held := readConfigFileText(t, path+".bak.0"); held != body {
		t.Errorf("the copy reads %q", held)
	}
}

// A step runs once. A second start changes nothing, and what the user took out afterwards
// stays out.
func TestMigrateConfigFileRunsAStepOnce(t *testing.T) {
	path := buildOldConfigFile(t)
	if _, err := cfg.MigrateConfigFile(path); err != nil {
		t.Fatalf("the migration failed: %v", err)
	}

	before := readConfigFileText(t, path)
	if _, err := cfg.MigrateConfigFile(path); err != nil {
		t.Fatalf("the second migration failed: %v", err)
	}
	if held := readConfigFileText(t, path); held != before {
		t.Errorf("the second run changed the file:\n%s", held)
	}

	// An agent the user takes out is not written again.
	if err := cfg.SaveTables(path, []cfg.TableUpdate{
		{Header: []string{"ai", "agents", "codex"}, Remove: true},
	}); err != nil {
		t.Fatalf("the agent was not removed: %v", err)
	}
	if _, err := cfg.MigrateConfigFile(path); err != nil {
		t.Fatalf("the third migration failed: %v", err)
	}
	if strings.Contains(readConfigFileText(t, path), "[ai.agents.codex]") {
		t.Error("the migration wrote an agent the user took out")
	}
}

// A table the file holds is left as the user wrote it, and every other table of the client
// is added beside it. A user who set one provider up still gains the others.
func TestMigrateConfigFileAddsTheTablesTheFileHasNot(t *testing.T) {
	path := buildOldConfigFile(t)
	if err := cfg.SaveTables(path, []cfg.TableUpdate{{
		Header: []string{"ai", "providers", "openai"}, Order: []string{"model"},
		Values: map[string]any{"model": "gpt-4o"},
	}, {
		Header: []string{"ai", "agents", "mine"}, Order: []string{"command"},
		Values: map[string]any{"command": "my-acp"},
	}}); err != nil {
		t.Fatalf("the tables were not written: %v", err)
	}

	if _, err := cfg.MigrateConfigFile(path); err != nil {
		t.Fatalf("the migration failed: %v", err)
	}
	config := readAiConfigOf(t, path)

	if held := config.Providers[cfg.ProviderOpenai]; held.Model != "gpt-4o" {
		t.Errorf("the provider of the file reads %+v", held)
	}
	if held := config.Providers[cfg.ProviderAnthropic]; held.Model == "" {
		t.Errorf("the migration wrote no anthropic provider: %+v", held)
	}
	if held := config.Agents["mine"]; held.Command != "my-acp" {
		t.Errorf("the agent of the file reads %+v", held)
	}
	for _, name := range []string{"claude", "codex", "opencode"} {
		if _, held := config.Agents[name]; !held {
			t.Errorf("the migration wrote no %s agent", name)
		}
	}
}

// A table the file holds gains the keys the client held for it before the file did, and
// keeps every key it names. A model and a command were in the client at the version before
// this one, so a table that names neither takes the one it had.
func TestMigrateConfigFileFillsTheKeysTheClientHeld(t *testing.T) {
	for _, held := range []struct {
		name    string
		written cfg.TableUpdate
		read    func(cfg.AiConfig) string
		wanted  string
	}{
		{"a provider with a key and no model", cfg.TableUpdate{
			Header: []string{"ai", "providers", "anthropic"},
			Order:  []string{"api_key_env"},
			Values: map[string]any{"api_key_env": "MY_KEY"},
		}, func(config cfg.AiConfig) string {
			held := config.Providers[cfg.ProviderAnthropic]
			return held.Model + " " + held.APIKeyEnv
		}, "claude-opus-5 MY_KEY"},
		{"an agent with a model and no command", cfg.TableUpdate{
			Header: []string{"ai", "agents", "opencode"}, Order: []string{"model"},
			Values: map[string]any{"model": "grok-code"},
		}, func(config cfg.AiConfig) string {
			held := config.Agents["opencode"]
			return held.Command + " " + strings.Join(held.Args, " ") + " " + held.Model
		}, "opencode acp grok-code"},
		{"an agent with a command of its own", cfg.TableUpdate{
			Header: []string{"ai", "agents", "claude"}, Order: []string{"command"},
			Values: map[string]any{"command": "my-claude"},
		}, func(config cfg.AiConfig) string {
			held := config.Agents["claude"]
			return held.Command + " " + strings.Join(held.Args, " ")
		}, "my-claude "},
	} {
		t.Run(held.name, func(t *testing.T) {
			path := buildOldConfigFile(t)
			if err := cfg.SaveTables(
				path, []cfg.TableUpdate{held.written}); err != nil {
				t.Fatalf("the table was not written: %v", err)
			}
			if _, err := cfg.MigrateConfigFile(path); err != nil {
				t.Fatalf("the migration failed: %v", err)
			}
			if wrote := held.read(readAiConfigOf(t, path)); wrote != held.wanted {
				t.Errorf("the table reads %q, wanted %q", wrote, held.wanted)
			}
		})
	}
}

// readAiConfigOf returns the AI settings a config file holds.
func readAiConfigOf(t *testing.T, path string) cfg.AiConfig {
	t.Helper()
	document, err := cfg.DecodeDocument(readConfigFileText(t, path))
	if err != nil {
		t.Fatalf("the file does not read: %v", err)
	}
	config := cfg.ParseAiConfig(document)
	if len(config.Problems) != 0 {
		t.Fatalf("the file reports %v", config.Problems)
	}
	return config
}

// A file written by a later client is left as it is. A client that does not know a version
// must not write over the file that carries it.
func TestMigrateConfigFileLeavesALaterFileAlone(t *testing.T) {
	path := buildOldConfigFile(t)
	if err := cfg.SaveTables(path, []cfg.TableUpdate{{
		Header: []string{"masume"}, Order: []string{"version"},
		Values: map[string]any{"version": cfg.ConfigVersion + 5},
	}}); err != nil {
		t.Fatalf("the version was not written: %v", err)
	}
	before := readConfigFileText(t, path)

	version, err := cfg.MigrateConfigFile(path)
	if err != nil {
		t.Fatalf("the migration failed: %v", err)
	}
	if version != cfg.ConfigVersion+5 {
		t.Errorf("the migration reports version %d", version)
	}
	if held := readConfigFileText(t, path); held != before {
		t.Errorf("the migration changed a file of a later version:\n%s", held)
	}
}

// The file the first run writes is at the version of this client, so it is never migrated.
func TestTheStarterConfigIsAtTheVersionOfTheClient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "masume", "config.toml")
	if _, err := cfg.EnsureConfigFile(path); err != nil {
		t.Fatalf("cannot write the starter config: %v", err)
	}
	before := readConfigFileText(t, path)

	document, err := cfg.DecodeDocument(before)
	if err != nil {
		t.Fatalf("the starter config does not read: %v", err)
	}
	if held := cfg.ReadConfigVersion(document); held != cfg.ConfigVersion {
		t.Errorf("the starter config reads version %d", held)
	}
	if _, err := cfg.MigrateConfigFile(path); err != nil {
		t.Fatalf("the migration failed: %v", err)
	}
	if held := readConfigFileText(t, path); held != before {
		t.Error("the migration changed the file the first run writes")
	}
}

// A file that does not read is left alone: a migration must never write over a file it
// could not read.
func TestMigrateConfigFileLeavesAFileThatDoesNotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[ui\ntheme = \n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	if _, err := cfg.MigrateConfigFile(path); err == nil {
		t.Error("a file that does not read was accepted")
	}
	if held := readConfigFileText(t, path); held != body {
		t.Errorf("the file reads %q", held)
	}
}
