package cfg

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/masumedb/masume/internal/core"
)

// One change of one row, applied to the settings and written to the config file. A change
// writes the table it belongs to and leaves every other line of the file alone.

// ApplySetting returns the settings with this row changed, and the tables the change writes.
// The path is the pages that are open under the section, which name the provider or the
// agent a row belongs to. A value the settings cannot use is refused, and the settings are
// returned unchanged.
func ApplySetting(
	sources SettingsSources, path []string, key, value string,
) (SettingsSources, []TableUpdate, error) {
	switch {
	case strings.HasPrefix(key, ItemProfilePrefix):
		return applyServedProfile(sources, strings.TrimPrefix(key, ItemProfilePrefix))
	case strings.HasPrefix(key, ItemPathPrefix):
		return applyRemovedPath(sources, strings.TrimPrefix(key, ItemPathPrefix))
	case strings.HasPrefix(key, ItemChordPrefix):
		return applyChordChoice(
			sources, readPage(path, 0), strings.TrimPrefix(key, ItemChordPrefix), value)
	}

	switch key {
	case ItemChatEnabled, ItemChatSource, ItemChatTimeout:
		return applyChatSetting(sources, key, value)
	case ItemModel, ItemAPIKeyEnv, ItemBaseURL, ItemToolSteps,
		ItemAgentName, ItemAgentCommand, ItemAgentArgs, ItemUseSource:
		return applySourceSetting(sources, path, key, value)
	case ItemAddAgent:
		return applyAddedAgent(sources)
	case ItemRemoveAgent:
		return applyRemovedAgent(sources, readPage(path, 1))
	case ItemTheme, ItemIcons, ItemKeyHints, ItemTimeZone, ItemHideSystem:
		return applyAppearanceSetting(sources, key, value)
	case ItemKeyPreset:
		sources.Keys.Preset = findChoice(PresetIDs, value, PresetDefault)
		return sources, []TableUpdate{{
			Header: []string{"keys"}, Order: []string{"preset"},
			Values: map[string]any{"preset": string(sources.Keys.Preset)},
		}}, nil
	case ItemMcpAccess, ItemMcpRowLimit, ItemMcpTimeout:
		return applyMcpSetting(sources, key, value)
	case ItemNotebookAdd:
		return applyAddedPath(sources, value)
	}
	return sources, nil, FormError{Reason: "no setting named " + key}
}

// applyChatSetting reads the switch of the chat, the source it sends to, or its time limit.
func applyChatSetting(
	sources SettingsSources, key, value string,
) (SettingsSources, []TableUpdate, error) {
	built := copyAiConfig(sources.Ai)
	switch key {
	case ItemChatEnabled:
		built.Enabled = value == toggleOn
	case ItemChatSource:
		if name, isAgent := ReadAgentSource(value); isAgent {
			built.DefaultAgent = name
		} else {
			id, known := core.FindAllowed(AiProviderIDs, value)
			if !known {
				return sources, nil, FormError{Reason: "no source named " + value}
			}
			built.DefaultProvider, built.DefaultAgent = id, ""
		}
	case ItemChatTimeout:
		milliseconds, err := readPositiveValue(value, "statement timeout")
		if err != nil {
			return sources, nil, err
		}
		built.StatementTimeout = time.Duration(milliseconds) * time.Millisecond
	}
	sources.Ai = built
	return sources, []TableUpdate{buildAiUpdate(built)}, nil
}

// applySourceSetting reads one setting of the provider or the agent the path names.
func applySourceSetting(
	sources SettingsSources, path []string, key, value string,
) (SettingsSources, []TableUpdate, error) {
	built := copyAiConfig(sources.Ai)
	name := readPage(path, 1)
	if readPage(path, 0) == PageAgents {
		return applyAgentSetting(sources, built, name, key, value)
	}

	id, known := core.FindAllowed(AiProviderIDs, name)
	if !known {
		return sources, nil, FormError{Reason: "no provider named " + name}
	}
	if key == ItemUseSource {
		built.DefaultProvider, built.DefaultAgent = id, ""
		sources.Ai = built
		return sources, []TableUpdate{buildAiUpdate(built)}, nil
	}

	settings := built.Providers[id]
	switch key {
	case ItemModel:
		settings.Model = strings.TrimSpace(value)
	case ItemAPIKeyEnv:
		settings.APIKeyEnv = strings.TrimSpace(value)
	case ItemBaseURL:
		settings.BaseURL = strings.TrimSpace(value)
	case ItemToolSteps:
		steps, err := readPositiveValue(value, "tool steps")
		if err != nil {
			return sources, nil, err
		}
		settings.MaxToolSteps = steps
	default:
		return sources, nil, FormError{Reason: "a provider has no " + key}
	}

	built.Providers[id] = settings
	sources.Ai = built
	return sources, []TableUpdate{buildProviderUpdate(built, id)}, nil
}

// applyAgentSetting reads one setting of one agent. A changed name writes the agent under
// the new name and removes the table of the old one.
func applyAgentSetting(
	sources SettingsSources, built AiConfig, was, key, value string,
) (SettingsSources, []TableUpdate, error) {
	settings, held := built.Agents[was]
	if !held {
		return sources, nil, FormError{Reason: "no agent named " + was}
	}
	written := strings.TrimSpace(value)

	switch key {
	case ItemUseSource:
		built.DefaultAgent = was
		sources.Ai = built
		return sources, []TableUpdate{buildAiUpdate(built)}, nil
	case ItemAgentName:
		if problem := findAgentNameProblem(built, was, written); problem != "" {
			return sources, nil, FormError{Reason: problem}
		}
		if written == was {
			return sources, nil, nil
		}
		settings.Name = written
		delete(built.Agents, was)
		built.Agents[written] = settings
		updates := []TableUpdate{
			buildAgentUpdate(built, written), TableUpdate{Header: []string{"ai", "agents", was}, Remove: true},
		}
		if built.DefaultAgent == was {
			built.DefaultAgent = written
			updates = append([]TableUpdate{buildAiUpdate(built)}, updates...)
		}
		sources.Ai = built
		return sources, updates, nil
	case ItemAgentCommand:
		settings.Command = written
	case ItemAgentArgs:
		settings.Args = strings.Fields(value)
	case ItemModel:
		settings.Model = written
	default:
		return sources, nil, FormError{Reason: "an agent has no " + key}
	}

	built.Agents[was] = settings
	sources.Ai = built
	return sources, []TableUpdate{buildAgentUpdate(built, was)}, nil
}

// findAgentNameProblem returns why this name cannot be the name of an agent.
func findAgentNameProblem(config AiConfig, was, name string) string {
	if name == "" {
		return "the name is empty, and an agent needs one"
	}
	if strings.ContainsAny(name, " \t.\"[]") {
		return "the name holds a space, a dot, a quote or a bracket"
	}
	if _, held := config.Agents[name]; held && name != was {
		return "another agent is named " + name
	}
	return ""
}

// applyAddedAgent adds an agent the config file has not got. The chat keeps the source it
// sends to, which the page of the new agent then offers to change.
func applyAddedAgent(sources SettingsSources) (SettingsSources, []TableUpdate, error) {
	built := copyAiConfig(sources.Ai)
	name := BuildFreeAgentName(built)
	built.Agents[name] = AiAgentSettings{Name: name, Command: name}
	sources.Ai = built
	return sources, []TableUpdate{buildAgentUpdate(built, name)}, nil
}

// BuildFreeAgentName returns a name no agent has yet.
func BuildFreeAgentName(config AiConfig) string {
	name := "agent"
	for at := 2; ; at++ {
		if _, held := config.Agents[name]; !held {
			return name
		}
		name = "agent" + strconv.Itoa(at)
	}
}

// applyRemovedAgent removes one agent. A chat that sent to it sends to the provider again.
func applyRemovedAgent(
	sources SettingsSources, name string,
) (SettingsSources, []TableUpdate, error) {
	built := copyAiConfig(sources.Ai)
	if _, held := built.Agents[name]; !held {
		return sources, nil, FormError{Reason: "no agent named " + name}
	}
	delete(built.Agents, name)
	updates := []TableUpdate{{Header: []string{"ai", "agents", name}, Remove: true}}
	if built.DefaultAgent == name {
		built.DefaultAgent = ""
		updates = append([]TableUpdate{buildAiUpdate(built)}, updates...)
	}
	sources.Ai = built
	return sources, updates, nil
}

// applyChordChoice binds the chords of one action. An empty row drops the choice, and the
// chord of the preset comes back.
func applyChordChoice(
	sources SettingsSources, scope, id, value string,
) (SettingsSources, []TableUpdate, error) {
	held, known := findKeyScope(scope)
	if !known {
		return sources, nil, FormError{Reason: "no key scope named " + scope}
	}

	choices := ChordChoices{}
	for actionKey, sequences := range sources.Keys.Choices {
		choices[actionKey] = sequences
	}
	actionKey := BuildActionKey(held, id)

	if strings.TrimSpace(value) == "" {
		delete(choices, actionKey)
	} else {
		sequences, parsed := ParseChordChoice(value)
		if !parsed {
			return sources, nil, FormError{
				Reason: strings.TrimSpace(value) + " is not a chord",
			}
		}
		for _, sequence := range sequences {
			for _, chord := range sequence {
				if reason := FindUndeliverableChord(chord); reason != "" {
					return sources, nil, FormError{Reason: reason}
				}
			}
		}
		choices[actionKey] = sequences
	}

	sources.Keys.Choices = choices
	return sources, []TableUpdate{buildScopeUpdate(choices, held)}, nil
}

// buildScopeUpdate returns the table of one key scope, with every action a chord was chosen
// for. An action of no choice is left out, so it keeps the chord of the preset.
func buildScopeUpdate(choices ChordChoices, scope KeyScope) TableUpdate {
	update := TableUpdate{
		Header: []string{"keys", string(scope)}, Values: map[string]any{},
	}
	for actionKey, sequences := range choices {
		held, id := SplitActionKey(actionKey)
		if held != scope {
			continue
		}
		update.Order = append(update.Order, id)
		update.Values[id] = describeChordValue(sequences)
	}
	slices.Sort(update.Order)
	if len(update.Order) == 0 {
		update.Remove = true
	}
	return update
}

// describeChordValue returns the chords of one action as the config file writes them: one
// chord as a word, two or more as a list, and none as an empty word.
func describeChordValue(sequences []ChordSequence) any {
	written := make([]string, 0, len(sequences))
	for _, sequence := range sequences {
		written = append(written, DescribeSequence(sequence))
	}
	if len(written) == 1 {
		return written[0]
	}
	if len(written) == 0 {
		return ""
	}
	return written
}

// applyAppearanceSetting reads the theme, the icons, the key hints, the time zone or the system
// schemas.
func applyAppearanceSetting(
	sources SettingsSources, key, value string,
) (SettingsSources, []TableUpdate, error) {
	settings := sources.UI
	switch key {
	case ItemTheme:
		settings.Theme = strings.TrimSpace(value)
	case ItemIcons:
		settings.IconSet = findChoice(IconSetNames, value, IconsPlain)
	case ItemKeyHints:
		settings.KeyHints = findChoice(KeyHintsModes, value, KeyHintsFull)
	case ItemTimeZone:
		settings.TimeZone = findChoice(TimeZoneModes, value, TimeZoneServer)
	case ItemHideSystem:
		settings.HideSystemSchemas = value == toggleOn
	}
	sources.UI = settings

	return sources, []TableUpdate{{
		Header: []string{"ui"},
		Order:  []string{"theme", "icons", "key_hints", "timezone", "hide_system_schemas"},
		Values: map[string]any{
			"theme": settings.Theme, "icons": string(settings.IconSet),
			"key_hints":           string(settings.KeyHints),
			"timezone":            string(settings.TimeZone),
			"hide_system_schemas": settings.HideSystemSchemas,
		},
	}}, nil
}

// applyMcpSetting reads the access level or a limit of the MCP server.
func applyMcpSetting(
	sources SettingsSources, key, value string,
) (SettingsSources, []TableUpdate, error) {
	config := sources.Mcp
	switch key {
	case ItemMcpAccess:
		config.Access = findChoice(McpAccessLevels, value, McpReadOnly)
	case ItemMcpRowLimit:
		rows, err := readPositiveValue(value, "row limit")
		if err != nil {
			return sources, nil, err
		}
		config.RowLimit = rows
	case ItemMcpTimeout:
		milliseconds, err := readPositiveValue(value, "statement timeout")
		if err != nil {
			return sources, nil, err
		}
		config.Timeout = time.Duration(milliseconds) * time.Millisecond
	}
	sources.Mcp = config
	return sources, []TableUpdate{buildMcpUpdate(config)}, nil
}

// applyServedProfile opens one connection to an agent outside masume, or closes it.
func applyServedProfile(
	sources SettingsSources, name string,
) (SettingsSources, []TableUpdate, error) {
	config := sources.Mcp
	kept := make([]string, 0, len(config.Profiles)+1)
	found := false
	for _, held := range config.Profiles {
		if held == name {
			found = true
			continue
		}
		kept = append(kept, held)
	}
	if !found {
		kept = append(kept, name)
	}
	config.Profiles = kept
	sources.Mcp = config
	return sources, []TableUpdate{buildMcpUpdate(config)}, nil
}

// applyAddedPath adds one notebook directory.
func applyAddedPath(
	sources SettingsSources, path string,
) (SettingsSources, []TableUpdate, error) {
	written := strings.TrimSpace(path)
	if written == "" {
		return sources, nil, FormError{Reason: "the directory is empty"}
	}
	for _, held := range sources.Notebooks.Paths {
		if held == written {
			return sources, nil, FormError{
				Reason: "the notebook directories already hold " + written,
			}
		}
	}
	sources.Notebooks.Paths = append(
		append([]string{}, sources.Notebooks.Paths...), written)
	return sources, []TableUpdate{buildNotebookUpdate(sources.Notebooks)}, nil
}

// applyRemovedPath removes one notebook directory.
func applyRemovedPath(
	sources SettingsSources, path string,
) (SettingsSources, []TableUpdate, error) {
	kept := make([]string, 0, len(sources.Notebooks.Paths))
	for _, held := range sources.Notebooks.Paths {
		if held != path {
			kept = append(kept, held)
		}
	}
	sources.Notebooks.Paths = NotebookSettings{Paths: kept}.Paths
	return sources, []TableUpdate{buildNotebookUpdate(sources.Notebooks)}, nil
}

// readPositiveValue returns a whole number above zero.
func readPositiveValue(value, label string) (int, error) {
	held, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || held <= 0 {
		return 0, FormError{
			Reason: label + " reads " + strings.TrimSpace(value) +
				", wanted a number above zero",
		}
	}
	return held, nil
}

// buildAiUpdate returns the `[ai]` table, which names the source the chat sends to.
func buildAiUpdate(config AiConfig) TableUpdate {
	update := TableUpdate{
		Header: []string{"ai"},
		Order: []string{
			"enabled", "default_provider", "default_agent", "statement_timeout_ms",
		},
		Values: map[string]any{
			"enabled":              config.Enabled,
			"default_provider":     string(config.DefaultProvider),
			"statement_timeout_ms": int(config.StatementTimeout / time.Millisecond),
		},
	}
	// An empty name removes the key, so the chat sends to the provider again.
	if config.DefaultAgent != "" {
		update.Values["default_agent"] = config.DefaultAgent
	}
	return update
}

// buildProviderUpdate returns the table of one provider. api_key is left out, so a key in the
// file stays there and the screen never writes one.
func buildProviderUpdate(config AiConfig, id AiProviderID) TableUpdate {
	settings := config.Providers[id]
	update := TableUpdate{
		Header: []string{"ai", "providers", string(id)},
		Order:  []string{"model", "api_key_env", "base_url", "max_tool_steps"},
		Values: map[string]any{"max_tool_steps": resolveConfiguredSteps(settings)},
	}
	for key, value := range map[string]string{
		"model": settings.Model, "api_key_env": settings.APIKeyEnv,
		"base_url": settings.BaseURL,
	} {
		if value != "" {
			update.Values[key] = value
		}
	}
	return update
}

// buildAgentUpdate returns the table of one agent.
func buildAgentUpdate(config AiConfig, name string) TableUpdate {
	settings := config.Agents[name]
	update := TableUpdate{
		Header: []string{"ai", "agents", name},
		Order:  []string{"command", "args", "model"},
		Values: map[string]any{"command": settings.Command},
	}
	if len(settings.Args) > 0 {
		update.Values["args"] = settings.Args
	}
	if settings.Model != "" {
		update.Values["model"] = settings.Model
	}
	return update
}

// buildMcpUpdate returns the `[mcp]` table.
func buildMcpUpdate(config McpConfig) TableUpdate {
	rows := config.RowLimit
	if rows <= 0 {
		rows = DefaultMcpRowLimit
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = DefaultMcpTimeout
	}
	access := config.Access
	if access == McpUnset {
		access = McpReadOnly
	}
	return TableUpdate{
		Header: []string{"mcp"},
		Order:  []string{"profiles", "access", "row_limit", "timeout_ms"},
		Values: map[string]any{
			"profiles": config.Profiles, "access": string(access),
			"row_limit": rows, "timeout_ms": int(timeout / time.Millisecond),
		},
	}
}

// buildNotebookUpdate returns the `[notebooks]` table.
func buildNotebookUpdate(settings NotebookSettings) TableUpdate {
	return TableUpdate{
		Header: []string{"notebooks"}, Order: []string{"paths"},
		Values: map[string]any{"paths": settings.Paths},
	}
}
