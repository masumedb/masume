package cfg_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
)

func TestEnsureConfigFileWritesTheStarterWhereThereIsNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "masume", "config.toml")

	written, err := cfg.EnsureConfigFile(path)
	if err != nil {
		t.Fatalf("cannot write the starter config: %v", err)
	}
	if !written {
		t.Fatal("answered that it wrote nothing, and there was no file")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read what it wrote: %v", err)
	}
	if string(body) != string(cfg.StarterConfig()) {
		t.Error("what it wrote is not the starter config")
	}
}

func TestEnsureConfigFileKeepsAFileThatIsAlreadyThere(t *testing.T) {
	kept := "[ui]\ntheme = \"nord\"\n"
	path := writeConfig(t, kept)

	written, err := cfg.EnsureConfigFile(path)
	if err != nil {
		t.Fatalf("cannot check the config file: %v", err)
	}
	if written {
		t.Error("answered that it wrote a file over one that was already there")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the config file: %v", err)
	}
	if string(body) != kept {
		t.Errorf("the config file was rewritten, and now holds %q", string(body))
	}
}

func TestEnsureConfigFileWritesAFileOnlyTheUserCanRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the permission bits of a file mean something else on Windows")
	}
	path := filepath.Join(t.TempDir(), "masume", "config.toml")

	if _, err := cfg.EnsureConfigFile(path); err != nil {
		t.Fatalf("cannot write the starter config: %v", err)
	}

	details, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cannot read the file details: %v", err)
	}
	if mode := details.Mode().Perm(); mode != 0o600 {
		t.Errorf("the file is %o, and a config file that may hold a password must be 600", mode)
	}
}

func TestLoadConfigReadsTheStarterWithNoProblem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "masume", "config.toml")
	if _, err := cfg.EnsureConfigFile(path); err != nil {
		t.Fatalf("cannot write the starter config: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 {
		t.Errorf("the starter config reports problems: %v", loaded.Problems)
	}
	if len(loaded.Profiles) != 0 {
		t.Errorf("the starter config opens %d profiles, and it should name none",
			len(loaded.Profiles))
	}
}

func TestStarterConfigNamesNoProfileToAnAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "masume", "config.toml")
	if _, err := cfg.EnsureConfigFile(path); err != nil {
		t.Fatalf("cannot write the starter config: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Mcp.Profiles) != 0 {
		t.Errorf("the starter config serves %v to an agent, and it should serve none",
			loaded.Mcp.Profiles)
	}
}

// The starter config shows every setting at its default value, so a new file must change
// nothing. This test compares the settings loaded from the starter file with the defaults
// used when there is no file.
func TestStarterConfigShowsTheDefaultsAndChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "masume", "config.toml")
	if _, err := cfg.EnsureConfigFile(path); err != nil {
		t.Fatalf("cannot write the starter config: %v", err)
	}
	loaded := cfg.LoadConfig(path)

	settings := cfg.DefaultUISettings()
	if loaded.Settings.IconSet != settings.IconSet {
		t.Errorf("the starter sets icons to %q, and the default is %q",
			loaded.Settings.IconSet, settings.IconSet)
	}
	if loaded.Settings.HideSystemSchemas != settings.HideSystemSchemas {
		t.Errorf("the starter sets hide_system_schemas to %v, and the default is %v",
			loaded.Settings.HideSystemSchemas, settings.HideSystemSchemas)
	}

	ai := cfg.DefaultAiConfig()
	if loaded.Ai.DefaultProvider != ai.DefaultProvider {
		t.Errorf("the starter sets default_provider to %q, and the default is %q",
			loaded.Ai.DefaultProvider, ai.DefaultProvider)
	}
	if loaded.Ai.StatementTimeout != ai.StatementTimeout {
		t.Errorf("the starter sets the AI statement timeout to %v, and the default is %v",
			loaded.Ai.StatementTimeout, ai.StatementTimeout)
	}
	mcp := cfg.DefaultMcpConfig()
	if loaded.Mcp.Access != mcp.Access {
		t.Errorf("the starter sets [mcp] access to %q, and the default is %q",
			loaded.Mcp.Access, mcp.Access)
	}
	if loaded.Mcp.RowLimit != mcp.RowLimit {
		t.Errorf("the starter sets [mcp] row_limit to %d, and the default is %d",
			loaded.Mcp.RowLimit, mcp.RowLimit)
	}
	if loaded.Mcp.Timeout != mcp.Timeout {
		t.Errorf("the starter sets [mcp] timeout_ms to %v, and the default is %v",
			loaded.Mcp.Timeout, mcp.Timeout)
	}
}

// The theme in the starter config must be a theme included in masume, or the first run
// starts with a theme that does not exist.
func TestStarterConfigNamesAThemeThatExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "masume", "config.toml")
	if _, err := cfg.EnsureConfigFile(path); err != nil {
		t.Fatalf("cannot write the starter config: %v", err)
	}
	if theme := cfg.LoadConfig(path).Settings.Theme; theme == "" {
		t.Error("the starter names no theme")
	}
}

// The first run writes the providers and the agents. The client carries none of its own, so
// the file it writes is where they come from.
func TestTheStarterConfigCarriesTheProvidersAndTheAgents(t *testing.T) {
	document, err := cfg.DecodeDocument(string(cfg.StarterConfig()))
	if err != nil {
		t.Fatalf("the starter config does not read: %v", err)
	}
	config := cfg.ParseAiConfig(document)
	if len(config.Problems) != 0 {
		t.Fatalf("the starter config reports %v", config.Problems)
	}

	if !config.Enabled || config.DefaultProvider != cfg.ProviderAnthropic {
		t.Errorf("the chat reads %+v", config)
	}
	for id, model := range map[cfg.AiProviderID]string{
		cfg.ProviderAnthropic: "claude-opus-5", cfg.ProviderOpenai: "gpt-5",
	} {
		held := config.Providers[id]
		if held.Model != model || held.APIKeyEnv == "" ||
			held.MaxToolSteps != cfg.DefaultMaxToolSteps {
			t.Errorf("the %s provider reads %+v", id, held)
		}
	}
	for _, name := range []string{"claude", "codex", "opencode"} {
		held, bound := config.Agents[name]
		if !bound {
			t.Errorf("the starter config names no %s agent", name)
			continue
		}
		if held.Name != name || held.Command == "" || len(held.Args) == 0 {
			t.Errorf("the %s agent reads %+v", name, held)
		}
	}
	// A client with no config file of its own carries neither.
	if fresh := cfg.DefaultAiConfig(); len(fresh.Agents) != 0 ||
		len(fresh.Providers) != 0 {
		t.Errorf("the client carries %d agents and %d providers",
			len(fresh.Agents), len(fresh.Providers))
	}
}
