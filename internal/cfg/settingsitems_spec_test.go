package cfg_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/cfg"
)

// buildTestSources returns everything the settings screen reads.
func buildTestSources() cfg.SettingsSources {
	config := cfg.DefaultAiConfig()
	config.Providers[cfg.ProviderAnthropic] = cfg.AiProviderSettings{
		Model: "claude-opus-5", APIKeyEnv: "ANTHROPIC_API_KEY", MaxToolSteps: 25,
	}
	config.Providers[cfg.ProviderOpenai] = cfg.AiProviderSettings{
		Model: "gpt-5", MaxToolSteps: 25,
	}
	// The agents are tables of the config file, so the settings hold what a file holds.
	for _, held := range []cfg.AiAgentSettings{
		{Name: "claude", Command: "npx", Args: []string{"-y", "claude-code-acp"}},
		{Name: "codex", Command: "npx", Args: []string{"-y", "codex-acp"}},
		{Name: "opencode", Command: "opencode", Args: []string{"acp"}},
	} {
		config.Agents[held.Name] = held
	}
	return cfg.SettingsSources{
		Ai: config, UI: cfg.DefaultUISettings(), Keys: cfg.DefaultKeySettings(),
		Mcp: cfg.DefaultMcpConfig(), Themes: []string{"ayu-dark", "tokyonight"},
		Actions: []cfg.KeyAction{
			{Scope: cfg.ScopeTree, ID: "open-node", Chords: "return",
				Detail: "open the object under the cursor"},
			{Scope: cfg.ScopeTree, ID: "filter-tree", Chords: "/"},
			{Scope: cfg.ScopeGrid, ID: "open-row", Chords: "return"},
		},
	}
}

// listItemKeys returns the key of every row of one page.
func listItemKeys(sources cfg.SettingsSources, section string, path ...string) []string {
	keys := []string{}
	for _, item := range cfg.BuildSettingItems(sources, section, path) {
		keys = append(keys, item.Key)
	}
	return keys
}

// findItem returns the row of one key.
func findItem(
	t *testing.T, sources cfg.SettingsSources, section, key string, path ...string,
) cfg.SettingItem {
	t.Helper()
	for _, item := range cfg.BuildSettingItems(sources, section, path) {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("the %s page %v holds no %s: %v", section, path, key,
		listItemKeys(sources, section, path...))
	return cfg.SettingItem{}
}

// applyToFile writes one row into a config file of this text, and returns the new text and
// the settings the change left.
func applyToFile(
	t *testing.T, body string, sources cfg.SettingsSources, path []string,
	key, value string,
) (string, cfg.SettingsSources) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	built, updates, err := cfg.ApplySetting(sources, path, key, value)
	if err != nil {
		t.Fatalf("%s was refused: %v", key, err)
	}
	if err := cfg.SaveTables(file, updates); err != nil {
		t.Fatalf("%s was not written: %v", key, err)
	}
	written, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("cannot read the config file: %v", err)
	}
	return string(written), built
}

// Every page carries the rows of that page, and nothing of another. The settings of a
// source stand on the page of that source, never beside the chat.
func TestEveryPageCarriesItsOwnRows(t *testing.T) {
	sources := buildTestSources()
	sources.Notebooks = cfg.NotebookSettings{Paths: []string{"~/notes"}}
	for _, held := range []struct {
		name    string
		section string
		path    []string
		wanted  []string
	}{
		{"ai", cfg.SectionAi, nil, []string{
			cfg.ItemChatEnabled, cfg.ItemChatSource,
			cfg.PageProviders, cfg.PageAgents, cfg.ItemChatTimeout,
		}},
		{"providers", cfg.SectionAi, []string{cfg.PageProviders},
			[]string{"anthropic", "openai", "grok", "openai_compatible"}},
		{"one provider", cfg.SectionAi,
			[]string{cfg.PageProviders, "openai"}, []string{
				cfg.ItemModel, cfg.ItemAPIKeyEnv, cfg.ItemBaseURL,
				cfg.ItemToolSteps, cfg.ItemUseSource,
			}},
		{"agents", cfg.SectionAi, []string{cfg.PageAgents},
			[]string{"claude", "codex", "opencode", cfg.ItemAddAgent}},
		{"one agent", cfg.SectionAi,
			[]string{cfg.PageAgents, "claude"}, []string{
				cfg.ItemAgentName, cfg.ItemAgentCommand, cfg.ItemAgentArgs,
				cfg.ItemModel, cfg.ItemUseSource, cfg.ItemRemoveAgent,
			}},
		{"appearance", cfg.SectionAppearance, nil, []string{
			cfg.ItemTheme, cfg.ItemIcons, cfg.ItemKeyHints, cfg.ItemHideSystem,
		}},
		{"keys", cfg.SectionKeys, nil, []string{
			cfg.ItemKeyPreset, string(cfg.ScopeGrid), string(cfg.ScopeTree),
		}},
		{"mcp", cfg.SectionMcp, nil, []string{
			cfg.ItemMcpAccess, cfg.ItemMcpRowLimit, cfg.ItemMcpTimeout,
			cfg.PageConnections,
		}},
		{"notebooks", cfg.SectionNotebooks, nil, []string{cfg.PageDirectories}},
		{"directories", cfg.SectionNotebooks, []string{cfg.PageDirectories},
			[]string{cfg.ItemPathPrefix + "~/notes", cfg.ItemNotebookAdd}},
	} {
		t.Run(held.name, func(t *testing.T) {
			keys := listItemKeys(sources, held.section, held.path...)
			if strings.Join(keys, ",") != strings.Join(held.wanted, ",") {
				t.Errorf("the page holds %v, wanted %v", keys, held.wanted)
			}
		})
	}
}

// A group row opens a page of its own and says what that page holds now.
func TestAGroupRowOpensAPageOfItsOwn(t *testing.T) {
	sources := buildTestSources()
	providers := findItem(t, sources, cfg.SectionAi, cfg.PageProviders)

	if providers.Kind != cfg.SettingGroup || providers.Page != cfg.PageProviders {
		t.Errorf("the providers row reads %+v", providers)
	}
	if providers.Value != string(sources.Ai.DefaultProvider) {
		t.Errorf("the providers row reads %q", providers.Value)
	}
	sources.Ai.DefaultAgent = "opencode"
	if held := findItem(t, sources, cfg.SectionAi, cfg.PageAgents); held.Value !=
		"opencode" {
		t.Errorf("the agents row reads %q", held.Value)
	}
}

// The sections are the ones the screen draws, and each one has a name and a line under it.
func TestTheSectionsAreNamed(t *testing.T) {
	sections := cfg.ListSettingSections()
	if len(sections) != 5 {
		t.Fatalf("the settings hold %d sections", len(sections))
	}
	for _, section := range sections {
		if section.Key == "" || section.Label == "" || section.Detail == "" {
			t.Errorf("a section reads %+v", section)
		}
	}
}

// A chat that is off hides every row under it, because none of them is read.
func TestAChatThatIsOffHidesTheRowsUnderIt(t *testing.T) {
	sources := buildTestSources()
	sources.Ai.Enabled = false

	keys := listItemKeys(sources, cfg.SectionAi)
	if len(keys) != 1 || keys[0] != cfg.ItemChatEnabled {
		t.Errorf("the AI section holds %v", keys)
	}
}

// The source row offers every provider and every agent, and the page of the source in use
// leaves out the row that sends the chat there.
func TestTheSourceRowOffersEverySource(t *testing.T) {
	sources := buildTestSources()
	sources.Ai.Agents["goose"] = cfg.AiAgentSettings{
		Name: "goose", Command: "sh", Args: []string{"acp"},
	}
	sources.Ai.DefaultAgent = "goose"

	source := findItem(t, sources, cfg.SectionAi, cfg.ItemChatSource)
	if source.Value != cfg.BuildAgentSource("goose") {
		t.Errorf("the source reads %q", source.Value)
	}
	if len(source.Choices) != len(cfg.AiProviderIDs)+len(sources.Ai.Agents) {
		t.Errorf("the source offers %v", source.Choices)
	}

	page := []string{cfg.PageAgents, "goose"}
	if findItem(t, sources, cfg.SectionAi,
		cfg.ItemAgentArgs, page...).Value != "acp" {
		t.Error("the arguments of the agent are not carried")
	}
	if held := listItemKeys(sources, cfg.SectionAi, page...); strings.Contains(
		strings.Join(held, ","), cfg.ItemUseSource) {
		t.Errorf("the source in use offers to be used: %v", held)
	}
	if held := listItemKeys(
		sources, cfg.SectionAi, cfg.PageAgents, "claude"); !strings.Contains(
		strings.Join(held, ","), cfg.ItemUseSource) {
		t.Errorf("an agent the chat can send to does not offer it: %v", held)
	}
}

// An agent whose command is not on the PATH says so on its command row.
func TestTheCommandRowMarksAnAgentThatIsNotInstalled(t *testing.T) {
	sources := buildTestSources()
	sources.Ai.Agents["goose"] = cfg.AiAgentSettings{
		Name: "goose", Command: "masume-no-such-command",
	}
	sources.Ai.DefaultAgent = "goose"

	detail := findItem(t, sources, cfg.SectionAi, cfg.ItemAgentCommand,
		cfg.PageAgents, "goose").Detail
	if !strings.Contains(detail, "not installed") {
		t.Errorf("the command row reads %q", detail)
	}
}

// The theme row steps through the themes the client knows.
func TestTheThemeRowOffersTheThemes(t *testing.T) {
	sources := buildTestSources()
	theme := findItem(t, sources, cfg.SectionAppearance, cfg.ItemTheme)

	if theme.Kind != cfg.SettingChoice {
		t.Errorf("the theme row is a %q", theme.Kind)
	}
	if strings.Join(theme.Choices, ",") != "ayu-dark,tokyonight" {
		t.Errorf("the theme row offers %v", theme.Choices)
	}
	if theme.Value != "ayu-dark" {
		t.Errorf("a file with no theme reads %q", theme.Value)
	}
}

// A limit of zero is the one the server applies, so the row shows that one.
func TestTheMcpRowsShowTheLimitsTheServerApplies(t *testing.T) {
	sources := buildTestSources()
	sources.Mcp = cfg.McpConfig{}

	if held := findItem(t, sources, cfg.SectionMcp, cfg.ItemMcpRowLimit).Value; held !=
		strconv.Itoa(cfg.DefaultMcpRowLimit) {
		t.Errorf("the row limit reads %q", held)
	}
	if held := findItem(t, sources, cfg.SectionMcp, cfg.ItemMcpAccess).Value; held !=
		string(cfg.McpReadOnly) {
		t.Errorf("the access reads %q", held)
	}
}

// The MCP section holds one row per connection, which says what an outside agent reaches.
func TestTheMcpSectionHoldsOneRowPerConnection(t *testing.T) {
	sources := buildTestSources()
	sources.Profiles = []cfg.Profile{{Name: "shop"}, {Name: "warehouse"}}
	sources.Mcp.Profiles = []string{"shop"}

	shop := findItem(t, sources, cfg.SectionMcp, cfg.ItemProfilePrefix+"shop",
		cfg.PageConnections)
	if !cfg.IsSettingOn(shop.Value) {
		t.Errorf("the served connection reads %q", shop.Value)
	}
	warehouse := findItem(t, sources, cfg.SectionMcp, cfg.ItemProfilePrefix+"warehouse",
		cfg.PageConnections)
	if cfg.IsSettingOn(warehouse.Value) {
		t.Errorf("a connection that is not served reads %q", warehouse.Value)
	}
	if warehouse.Detail != "not served" {
		t.Errorf("the row reads %q", warehouse.Detail)
	}
}

// A change writes the table it belongs to, and leaves every other line of the file alone.
func TestAChangeWritesOneTable(t *testing.T) {
	body := "[ui]\ntheme = \"ayu-dark\"\n\n[notebooks]\npaths = [\"~/notes\"]\n"
	written, built := applyToFile(
		t, body, buildTestSources(), nil, cfg.ItemTheme, "tokyonight")

	if built.UI.Theme != "tokyonight" {
		t.Errorf("the settings read %q", built.UI.Theme)
	}
	if !strings.Contains(written, "theme = \"tokyonight\"") {
		t.Errorf("the theme was not written:\n%s", written)
	}
	if !strings.Contains(written, "[notebooks]") {
		t.Errorf("the change dropped another table:\n%s", written)
	}
}

// A value the settings cannot use is refused, and nothing is written.
func TestAValueThatIsRefusedChangesNothing(t *testing.T) {
	sources := buildTestSources()
	for _, held := range []struct{ key, value string }{
		{cfg.ItemChatTimeout, "soon"},
		{cfg.ItemToolSteps, "0"},
		{cfg.ItemMcpRowLimit, "-4"},
		{cfg.ItemChatSource, "no-such-provider"},
	} {
		t.Run(held.key+"="+held.value, func(t *testing.T) {
			built, updates, err := cfg.ApplySetting(sources, nil, held.key, held.value)
			if err == nil {
				t.Fatalf("%q was accepted", held.value)
			}
			if len(updates) != 0 {
				t.Errorf("a refused value writes %v", updates)
			}
			if built.Ai.StatementTimeout != sources.Ai.StatementTimeout {
				t.Error("a refused value changed the settings")
			}
		})
	}
}

// The screen never writes a key into the file, so a key already there survives a change.
func TestAProviderChangeKeepsAKeyOfTheFile(t *testing.T) {
	body := "[ai.providers.anthropic]\napi_key = \"sk-kept\"\nmodel = \"old\"\n"
	written, _ := applyToFile(t, body, buildTestSources(),
		[]string{cfg.PageProviders, string(cfg.ProviderAnthropic)},
		cfg.ItemModel, "claude-haiku-4-5")

	if !strings.Contains(written, "api_key = \"sk-kept\"") {
		t.Errorf("the key of the file is gone:\n%s", written)
	}
	if !strings.Contains(written, "model = \"claude-haiku-4-5\"") {
		t.Errorf("the model was not written:\n%s", written)
	}
}

// Choosing a provider writes which one the chat sends to, and clears the agent.
func TestChoosingASourceNamesWhatTheChatSendsTo(t *testing.T) {
	sources := buildTestSources()
	sources.Ai.Agents["goose"] = cfg.AiAgentSettings{Name: "goose", Command: "sh"}
	sources.Ai.DefaultAgent = "goose"

	written, built := applyToFile(
		t, "", sources, nil, cfg.ItemChatSource, string(cfg.ProviderOpenai))
	if built.Ai.DefaultAgent != "" {
		t.Errorf("the chat still sends to %q", built.Ai.DefaultAgent)
	}
	if built.Ai.DefaultProvider != cfg.ProviderOpenai {
		t.Errorf("the chat sends to %q", built.Ai.DefaultProvider)
	}
	if strings.Contains(written, "default_agent") {
		t.Errorf("the agent is still named:\n%s", written)
	}
}

// An agent that is added is written under a name no other agent has, and the chat keeps the
// source it sends to.
func TestAddingAnAgentNamesIt(t *testing.T) {
	sources := buildTestSources()
	written, built := applyToFile(
		t, "", sources, []string{cfg.PageAgents}, cfg.ItemAddAgent, "")

	name := cfg.BuildFreeAgentName(sources.Ai)
	if _, held := built.Ai.Agents[name]; !held {
		t.Fatalf("the agents read %v", built.Ai.Agents)
	}
	if built.Ai.DefaultAgent != sources.Ai.DefaultAgent {
		t.Errorf("the chat was sent to %q", built.Ai.DefaultAgent)
	}
	if !strings.Contains(written, "[ai.agents."+name+"]") {
		t.Errorf("the agent has no table:\n%s", written)
	}

	_, second := applyToFile(
		t, written, built, []string{cfg.PageAgents}, cfg.ItemAddAgent, "")
	if len(second.Ai.Agents) != len(built.Ai.Agents)+1 {
		t.Errorf("the second agent took the name of the first: %v", second.Ai.Agents)
	}
}

// One row of the page of a source sends the chat there.
func TestOneRowOfASourceSendsTheChatThere(t *testing.T) {
	sources := buildTestSources()
	written, built := applyToFile(t, "", sources,
		[]string{cfg.PageAgents, "opencode"}, cfg.ItemUseSource, "")
	if built.Ai.DefaultAgent != "opencode" {
		t.Errorf("the chat sends to %q", built.Ai.DefaultAgent)
	}
	if !strings.Contains(written, "default_agent") {
		t.Errorf("the file names no agent:\n%s", written)
	}

	_, back := applyToFile(t, written, built,
		[]string{cfg.PageProviders, string(cfg.ProviderOpenai)}, cfg.ItemUseSource, "")
	if back.Ai.DefaultAgent != "" || back.Ai.DefaultProvider != cfg.ProviderOpenai {
		t.Errorf("the chat sends to %+v", back.Ai.DefaultProvider)
	}
}

// An agent that is renamed is written under the new name, and the table of the old name
// leaves the file.
func TestRenamingAnAgentDropsTheOldTable(t *testing.T) {
	sources := buildTestSources()
	sources.Ai.Agents["goose"] = cfg.AiAgentSettings{Name: "goose", Command: "sh"}
	sources.Ai.DefaultAgent = "goose"
	body := "[ai]\ndefault_agent = \"goose\"\n\n[ai.agents.goose]\ncommand = \"sh\"\n"

	written, built := applyToFile(t, body, sources,
		[]string{cfg.PageAgents, "goose"}, cfg.ItemAgentName, "duck")
	if built.Ai.DefaultAgent != "duck" {
		t.Errorf("the chat sends to %q", built.Ai.DefaultAgent)
	}
	if strings.Contains(written, "[ai.agents.goose]") {
		t.Errorf("the old table is still there:\n%s", written)
	}
	if !strings.Contains(written, "[ai.agents.duck]") {
		t.Errorf("the new table was not written:\n%s", written)
	}
}

// A name an agent cannot have is refused.
func TestAnAgentNameThatCannotBeWrittenIsRefused(t *testing.T) {
	sources := buildTestSources()
	sources.Ai.Agents["goose"] = cfg.AiAgentSettings{Name: "goose", Command: "sh"}
	sources.Ai.DefaultAgent = "goose"

	for _, held := range []string{"", "  ", "two words", "a.b", "a[b]", "opencode"} {
		if _, _, err := cfg.ApplySetting(
			sources, []string{cfg.PageAgents, "goose"}, cfg.ItemAgentName, held); err == nil {
			t.Errorf("%q was accepted as a name", held)
		}
	}
}

// A removed agent loses its table, and the chat sends to a provider again.
func TestRemovingAnAgentDropsItsTable(t *testing.T) {
	sources := buildTestSources()
	sources.Ai.Agents["goose"] = cfg.AiAgentSettings{Name: "goose", Command: "sh"}
	sources.Ai.DefaultAgent = "goose"
	body := "[ai]\ndefault_agent = \"goose\"\n\n[ai.agents.goose]\ncommand = \"sh\"\n" +
		"\n[notebooks]\npaths = []\n"

	written, built := applyToFile(t, body, sources,
		[]string{cfg.PageAgents, "goose"}, cfg.ItemRemoveAgent, "")
	if _, held := built.Ai.Agents["goose"]; held || built.Ai.DefaultAgent != "" {
		t.Errorf("the agent is still held: %+v", built.Ai)
	}
	if strings.Contains(written, "[ai.agents.goose]") ||
		strings.Contains(written, "default_agent") {
		t.Errorf("the agent is still named:\n%s", written)
	}
	if !strings.Contains(written, "[notebooks]") {
		t.Errorf("the change dropped another table:\n%s", written)
	}
}

// One row of the MCP section opens a connection to an outside agent, and closes it again.
func TestAProfileRowOpensAConnectionToAnOutsideAgent(t *testing.T) {
	sources := buildTestSources()
	sources.Profiles = []cfg.Profile{{Name: "shop"}, {Name: "warehouse"}}
	sources.Mcp.Profiles = []string{"warehouse"}

	written, built := applyToFile(
		t, "", sources, []string{cfg.PageConnections}, cfg.ItemProfilePrefix+"shop", "")
	if !cfg.ServesMcpProfile(built.Mcp, "shop") ||
		!cfg.ServesMcpProfile(built.Mcp, "warehouse") {
		t.Fatalf("the server serves %v", built.Mcp.Profiles)
	}
	if !strings.Contains(written, "warehouse") {
		t.Errorf("the change dropped a connection:\n%s", written)
	}

	_, closed := applyToFile(t, written, built,
		[]string{cfg.PageConnections}, cfg.ItemProfilePrefix+"shop", "")
	if cfg.ServesMcpProfile(closed.Mcp, "shop") {
		t.Errorf("the server still serves shop: %v", closed.Mcp.Profiles)
	}
	if !cfg.ServesMcpProfile(closed.Mcp, "warehouse") {
		t.Errorf("the server dropped warehouse: %v", closed.Mcp.Profiles)
	}
}

// The notebook section adds one directory, and refuses an empty or a repeated one.
func TestTheNotebookSectionAddsOneDirectory(t *testing.T) {
	sources := buildTestSources()
	sources.Notebooks = cfg.NotebookSettings{Paths: []string{"~/notes"}}

	_, built := applyToFile(t, "", sources,
		[]string{cfg.PageDirectories}, cfg.ItemNotebookAdd, "~/team/sql")
	if len(built.Notebooks.Paths) != 2 || built.Notebooks.Paths[1] != "~/team/sql" {
		t.Errorf("the directories read %v", built.Notebooks.Paths)
	}

	for _, held := range []string{"", "  ", "~/notes"} {
		if _, _, err := cfg.ApplySetting(
			sources, []string{cfg.PageDirectories}, cfg.ItemNotebookAdd, held); err == nil {
			t.Errorf("%q was accepted", held)
		}
	}
}

// A directory of the notebook section is removed by its own row, and every other one stays.
func TestRemovingANotebookDirectoryKeepsTheOthers(t *testing.T) {
	sources := buildTestSources()
	sources.Notebooks = cfg.NotebookSettings{Paths: []string{"~/notes", "~/team/sql"}}

	_, built := applyToFile(t, "", sources,
		[]string{cfg.PageDirectories}, cfg.ItemPathPrefix+"~/notes", "")
	if len(built.Notebooks.Paths) != 1 || built.Notebooks.Paths[0] != "~/team/sql" {
		t.Errorf("the directories read %v", built.Notebooks.Paths)
	}
	if held := listItemKeys(
		built, cfg.SectionNotebooks, cfg.PageDirectories); len(held) != 2 {
		t.Errorf("the page holds %v", held)
	}
}

// The written file reads back as the settings that were written.
func TestTheWrittenFileReadsBack(t *testing.T) {
	sources := buildTestSources()
	written, _ := applyToFile(t, "", sources, nil, cfg.ItemChatTimeout, "45000")

	document, err := cfg.DecodeDocument(written)
	if err != nil {
		t.Fatalf("the written file does not read: %v", err)
	}
	read := cfg.ParseAiConfig(document)
	if len(read.Problems) != 0 {
		t.Errorf("the written file reports %v", read.Problems)
	}
	if read.StatementTimeout != 45*time.Second {
		t.Errorf("the timeout reads %v", read.StatementTimeout)
	}
}

// A row of no section is refused, so a key that is misspelt writes nothing.
func TestARowOfNoSectionIsRefused(t *testing.T) {
	if _, _, err := cfg.ApplySetting(
		buildTestSources(), nil, "no.such.row", "on"); err == nil {
		t.Error("a row of no section was accepted")
	}
}

// The keys section holds the preset and one page per scope, and a page of a scope holds one
// row per action of that scope.
func TestTheKeysSectionHoldsOnePagePerScope(t *testing.T) {
	sources := buildTestSources()

	keys := listItemKeys(sources, cfg.SectionKeys)
	if strings.Join(keys, ",") != strings.Join([]string{
		cfg.ItemKeyPreset, string(cfg.ScopeGrid), string(cfg.ScopeTree),
	}, ",") {
		t.Errorf("the section holds %v", keys)
	}
	if held := findItem(t, sources, cfg.SectionKeys, string(cfg.ScopeTree)); held.Value !=
		"2 keys" {
		t.Errorf("the tree row reads %q", held.Value)
	}

	page := listItemKeys(sources, cfg.SectionKeys, string(cfg.ScopeTree))
	if strings.Join(page, ",") != cfg.ItemChordPrefix+"open-node,"+
		cfg.ItemChordPrefix+"filter-tree" {
		t.Errorf("the page of the tree holds %v", page)
	}
	row := findItem(t, sources, cfg.SectionKeys,
		cfg.ItemChordPrefix+"open-node", string(cfg.ScopeTree))
	if row.Value != "return" || row.Detail != "open the object under the cursor" {
		t.Errorf("the row reads %+v", row)
	}
	if !row.Faint {
		t.Error("an action of the preset alone is not marked as such")
	}
}

// A chord that is chosen is written under the scope of its action, and the row is no longer
// marked as the chord of the preset.
func TestChoosingAChordWritesItUnderItsScope(t *testing.T) {
	sources := buildTestSources()
	written, built := applyToFile(t, "[ui]\ntheme = \"ayu-dark\"\n", sources,
		[]string{string(cfg.ScopeTree)}, cfg.ItemChordPrefix+"filter-tree", "ctrl+alt+f")

	if !strings.Contains(written, "[keys.tree]") ||
		!strings.Contains(written, "filter-tree = \"ctrl+alt+f\"") {
		t.Errorf("the chord was not written:\n%s", written)
	}
	if !strings.Contains(written, "[ui]") {
		t.Errorf("the change dropped another table:\n%s", written)
	}

	built.Actions[1].Chords = "ctrl+alt+f"
	row := findItem(t, built, cfg.SectionKeys,
		cfg.ItemChordPrefix+"filter-tree", string(cfg.ScopeTree))
	if row.Faint || row.Value != "ctrl+alt+f" {
		t.Errorf("the row reads %+v", row)
	}
}

// The written chords read back as the chords that were written: one chord as a word, two as
// a list, and none as an empty word.
func TestTheWrittenChordsReadBack(t *testing.T) {
	sources := buildTestSources()
	body := ""
	for _, held := range [][2]string{
		{"open-node", "return, space"}, {"filter-tree", "none"},
	} {
		body, sources = applyToFile(t, body, sources, []string{string(cfg.ScopeTree)},
			cfg.ItemChordPrefix+held[0], held[1])
	}

	document, err := cfg.DecodeDocument(body)
	if err != nil {
		t.Fatalf("the written file does not read: %v", err)
	}
	read := cfg.ParseKeySettings(document)
	if len(read.Problems) != 0 {
		t.Errorf("the written file reports %v", read.Problems)
	}
	if held := cfg.DescribeChordChoice(
		read.Choices["tree:open-node"]); held != "return, space" {
		t.Errorf("the chords of open-node read %q", held)
	}
	if held := cfg.DescribeChordChoice(
		read.Choices["tree:filter-tree"]); held != cfg.NoChord {
		t.Errorf("the chords of filter-tree read %q", held)
	}
}

// An empty row drops the chord that was chosen, and the table goes with the last one.
func TestAnEmptyChordRowTakesBackTheChordOfThePreset(t *testing.T) {
	sources := buildTestSources()
	body, sources := applyToFile(t, "", sources, []string{string(cfg.ScopeTree)},
		cfg.ItemChordPrefix+"filter-tree", "ctrl+alt+f")

	written, built := applyToFile(t, body, sources, []string{string(cfg.ScopeTree)},
		cfg.ItemChordPrefix+"filter-tree", "")
	if len(built.Keys.Choices) != 0 {
		t.Errorf("the choices read %v", built.Keys.Choices)
	}
	if strings.Contains(written, "[keys.tree]") {
		t.Errorf("the table of the scope is still there:\n%s", written)
	}
}

// A chord the client cannot read is refused, and nothing is written.
func TestAChordThatCannotBeReadIsRefused(t *testing.T) {
	sources := buildTestSources()
	for _, held := range []string{"ctrl+", "no-such-key", "ctrl+i"} {
		_, updates, err := cfg.ApplySetting(sources, []string{string(cfg.ScopeTree)},
			cfg.ItemChordPrefix+"filter-tree", held)
		if err == nil {
			t.Errorf("%q was accepted", held)
		}
		if len(updates) != 0 {
			t.Errorf("a refused chord writes %v", updates)
		}
	}
	if _, _, err := cfg.ApplySetting(sources, []string{"no-such-scope"},
		cfg.ItemChordPrefix+"filter-tree", "f"); err == nil {
		t.Error("a scope the client has not got was accepted")
	}
}
