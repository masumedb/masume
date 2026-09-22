package cfg

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/masumedb/masume/internal/core"
)

// The settings of the client, as one list of rows per section. A row carries what it is now,
// so a screen draws it without reading the config file again.

// SettingsSources is everything the settings screen reads and writes.
type SettingsSources struct {
	Ai        AiConfig
	UI        UISettings
	Keys      KeySettings
	Mcp       McpConfig
	Notebooks NotebookSettings
	// Themes is the theme names the appearance section offers.
	Themes []string
	// Profiles is the connections the MCP section can open to an outside agent.
	Profiles []Profile
	// Actions is every action of the client a chord can be bound to.
	Actions []KeyAction
}

// KeyAction is one action of the client, as the keys section reads it.
type KeyAction struct {
	Scope KeyScope
	ID    string
	// Chords is what the action is bound to now, in the spelling the config file uses.
	Chords string
	// Owner is the card the action belongs to, for a scope that holds more than one.
	Owner string
	// Detail is what the action does.
	Detail string
}

// SettingKind is how one row is changed.
type SettingKind string

// The kinds of row a section holds.
const (
	// SettingToggle is on or off.
	SettingToggle SettingKind = "toggle"
	// SettingChoice steps through the values it offers.
	SettingChoice SettingKind = "choice"
	// SettingText is typed into.
	SettingText SettingKind = "text"
	// SettingAction runs when it is chosen, such as adding an agent.
	SettingAction SettingKind = "action"
	// SettingGroup opens a page of its own, such as the providers of the AI section.
	SettingGroup SettingKind = "group"
)

// SettingItem is one row of a section.
type SettingItem struct {
	Key   string
	Label string
	// Detail is the line under the label, which says what the row is.
	Detail string
	Kind   SettingKind
	Value  string
	// Choices are the values a choice row steps through.
	Choices []string
	// Page is the page a group row opens, under the page the row is on.
	Page string
	// Faint is true for a row that carries nothing yet.
	Faint bool
}

// SettingSection is one section of the settings.
type SettingSection struct {
	Key    string
	Label  string
	Detail string
}

// The sections of the settings.
const (
	SectionAi         = "ai"
	SectionAppearance = "appearance"
	SectionKeys       = "keys"
	SectionMcp        = "mcp"
	SectionNotebooks  = "notebooks"
)

// ListSettingSections returns the sections, in the order the screen draws them.
func ListSettingSections() []SettingSection {
	return []SettingSection{
		{Key: SectionAi, Label: "AI", Detail: "ai chat, providers and agents"},
		{Key: SectionAppearance, Label: "Appearance",
			Detail: "theme, icons and key hints"},
		{Key: SectionKeys, Label: "Keys", Detail: "key bindings"},
		{Key: SectionMcp, Label: "MCP",
			Detail: "mcp server for outside agents"},
		{Key: SectionNotebooks, Label: "Notebooks", Detail: "notebook directories"},
	}
}

// The keys of the rows every section holds.
const (
	ItemChatEnabled  = "chat.enabled"
	ItemChatSource   = "chat.source"
	ItemChatTimeout  = "chat.timeout"
	ItemModel        = "model"
	ItemAPIKeyEnv    = "apiKeyEnv"
	ItemBaseURL      = "baseUrl"
	ItemToolSteps    = "toolSteps"
	ItemAgentName    = "agent.name"
	ItemAgentCommand = "agent.command"
	ItemAgentArgs    = "agent.args"
	ItemAddAgent     = "agent.add"
	ItemUseSource    = "source.use"
	ItemRemoveAgent  = "agent.remove"
	ItemTheme        = "theme"
	ItemIcons        = "icons"
	ItemKeyHints     = "keyHints"
	ItemTimeZone     = "timeZone"
	ItemHideSystem   = "hideSystemSchemas"
	ItemKeyPreset    = "keyPreset"
	ItemMcpAccess    = "mcp.access"
	ItemMcpRowLimit  = "mcp.rowLimit"
	ItemMcpTimeout   = "mcp.timeout"
	ItemNotebookAdd  = "notebook.add"
	// ItemChordPrefix carries the action of the row after it.
	ItemChordPrefix = "chord:"
	// ItemProfilePrefix and ItemPathPrefix carry the name of the row after them.
	ItemProfilePrefix = "mcp.profile:"
	ItemPathPrefix    = "notebook.path:"
)

// The pages a section opens under itself. A page of a source is that source under one of
// these, such as `providers/anthropic`.
const (
	PageProviders   = "providers"
	PageAgents      = "agents"
	PageConnections = "connections"
	PageDirectories = "directories"
)

// AgentSourcePrefix marks a source that is an agent rather than a provider.
const AgentSourcePrefix = "agent:"

// BuildAgentSource returns the source value of one agent.
func BuildAgentSource(name string) string { return AgentSourcePrefix + name }

// ReadAgentSource returns the agent of a source value, and false for a provider.
func ReadAgentSource(written string) (string, bool) {
	return strings.CutPrefix(written, AgentSourcePrefix)
}

// BuildSettingItems returns the rows of one page. The path is the pages that are open
// under the section, such as `providers` and then the provider itself.
func BuildSettingItems(
	sources SettingsSources, section string, path []string,
) []SettingItem {
	switch section {
	case SectionAi:
		return buildAiItems(sources, path)
	case SectionAppearance:
		return buildAppearanceItems(sources)
	case SectionKeys:
		return buildKeyItems(sources, path)
	case SectionMcp:
		return buildMcpItems(sources, path)
	}
	return buildNotebookItems(sources, path)
}

// readPage returns the page at this depth of the path, and an empty name where the path ends
// above it.
func readPage(path []string, depth int) string {
	if depth >= len(path) {
		return ""
	}
	return path[depth]
}

// buildAiItems returns the chat and the sources it can send to.
func buildAiItems(sources SettingsSources, path []string) []SettingItem {
	config := sources.Ai
	switch readPage(path, 0) {
	case PageProviders:
		if id := readPage(path, 1); id != "" {
			return buildProviderItems(config, AiProviderID(id))
		}
		return buildProviderRows(config)
	case PageAgents:
		if name := readPage(path, 1); name != "" {
			return buildAgentItems(config, name)
		}
		return buildAgentRows(config)
	}

	items := []SettingItem{{
		Key: ItemChatEnabled, Label: "ai chat", Kind: SettingToggle,
		Value: describeToggle(config.Enabled),
	}}
	if !config.Enabled {
		return items
	}
	return append(items,
		SettingItem{
			Key: ItemChatSource, Label: "source", Kind: SettingChoice,
			Value: describeAiSource(config), Choices: listAiSources(config),
		},
		SettingItem{
			Key: PageProviders, Page: PageProviders, Label: "providers",
			Kind: SettingGroup, Detail: "api providers",
			Value: describeProviderCount(config),
		},
		SettingItem{
			Key: PageAgents, Page: PageAgents, Label: "agents", Kind: SettingGroup,
			Detail: "acp agents",
			Value:  describeAgentCount(config),
		},
		SettingItem{
			Key: ItemChatTimeout, Label: "statement timeout", Kind: SettingText,
			Detail: "time limit in milliseconds",
			Value:  strconv.Itoa(int(config.StatementTimeout / time.Millisecond)),
		})
}

// buildProviderRows returns one row per provider, each opening its settings.
func buildProviderRows(config AiConfig) []SettingItem {
	items := make([]SettingItem, 0, len(AiProviderIDs))
	for _, id := range AiProviderIDs {
		settings := config.Providers[id]
		detail := ""
		if config.DefaultAgent == "" && config.DefaultProvider == id {
			detail = "in use by ai chat"
		}
		items = append(items, SettingItem{
			Key: string(id), Page: string(id), Label: string(id), Kind: SettingGroup,
			Detail: detail, Value: settings.Model, Faint: settings.Model == "",
		})
	}
	return items
}

// buildAgentRows returns one row per agent, each opening its settings, and the row that adds
// one.
func buildAgentRows(config AiConfig) []SettingItem {
	items := make([]SettingItem, 0, len(config.Agents)+1)
	for _, name := range listAgentNames(config) {
		settings := config.Agents[name]
		detail := ""
		switch {
		case !FindsAgentCommand(settings):
			detail = "not installed: " + settings.Command + " is not on the PATH"
		case config.DefaultAgent == name:
			detail = "in use by ai chat"
		}
		items = append(items, SettingItem{
			Key: name, Page: name, Label: name, Kind: SettingGroup, Detail: detail,
			Value: settings.Command, Faint: settings.Command == "",
		})
	}
	return append(items, SettingItem{
		Key: ItemAddAgent, Label: "add an agent", Kind: SettingAction,
	})
}

// buildProviderItems returns the settings of one provider.
func buildProviderItems(config AiConfig, id AiProviderID) []SettingItem {
	settings := config.Providers[id]
	items := []SettingItem{
		{Key: ItemModel, Label: "model", Kind: SettingText,
			Value: settings.Model, Faint: settings.Model == ""},
		{Key: ItemAPIKeyEnv, Label: "api key variable", Kind: SettingText,
			Detail: "environment variable name",
			Value:  settings.APIKeyEnv, Faint: settings.APIKeyEnv == ""},
		{Key: ItemBaseURL, Label: "server address", Kind: SettingText,
			Detail: "empty uses the provider default",
			Value:  settings.BaseURL, Faint: settings.BaseURL == ""},
		{Key: ItemToolSteps, Label: "tool steps", Kind: SettingText,
			Detail: "max tool rounds per question",
			Value:  strconv.Itoa(resolveConfiguredSteps(settings))},
	}
	return append(items, buildUseSourceItem(
		config.DefaultAgent == "" && config.DefaultProvider == id)...)
}

// buildAgentItems returns the settings of one agent.
func buildAgentItems(config AiConfig, name string) []SettingItem {
	settings := config.Agents[name]
	command := settings.Command
	detail := "acp executable"
	if command != "" && !FindsAgentCommand(settings) {
		detail = "not installed: " + command + " is not on the PATH"
	}
	items := []SettingItem{
		{Key: ItemAgentName, Label: "name", Kind: SettingText,
			Detail: "agent name in the config file", Value: name},
		{Key: ItemAgentCommand, Label: "command", Kind: SettingText,
			Detail: detail, Value: command, Faint: command == ""},
		{Key: ItemAgentArgs, Label: "arguments", Kind: SettingText,
			Detail: "separated by spaces",
			Value:  strings.Join(settings.Args, " "), Faint: len(settings.Args) == 0},
		{Key: ItemModel, Label: "model", Kind: SettingText,
			Detail: "empty keeps the agent default",
			Value:  settings.Model, Faint: settings.Model == ""},
	}
	items = append(items, buildUseSourceItem(config.DefaultAgent == name)...)
	return append(items, SettingItem{
		Key: ItemRemoveAgent, Label: "remove this agent", Kind: SettingAction,
	})
}

// buildUseSourceItem returns the row that sends the chat to this source, and no row where
// the chat already sends to it.
func buildUseSourceItem(inUse bool) []SettingItem {
	if inUse {
		return nil
	}
	return []SettingItem{{
		Key: ItemUseSource, Label: "set as default", Kind: SettingAction,
	}}
}

// describeProviderCount returns the provider the chat sends to, or how many there are.
func describeProviderCount(config AiConfig) string {
	if config.DefaultAgent == "" {
		return string(config.DefaultProvider)
	}
	return strconv.Itoa(len(AiProviderIDs))
}

// describeAgentCount returns the agent the chat sends to, or how many there are.
func describeAgentCount(config AiConfig) string {
	if config.DefaultAgent != "" {
		return config.DefaultAgent
	}
	return strconv.Itoa(len(config.Agents))
}

// listAgentNames returns the agents of the config file, in order.
func listAgentNames(config AiConfig) []string {
	names := make([]string, 0, len(config.Agents))
	for name := range config.Agents {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// buildKeyItems returns the preset and the scopes a chord is bound in.
func buildKeyItems(sources SettingsSources, path []string) []SettingItem {
	if scope, known := findKeyScope(readPage(path, 0)); known {
		return buildChordItems(sources, scope)
	}

	items := []SettingItem{{
		Key: ItemKeyPreset, Label: "preset", Kind: SettingChoice,
		Detail: "base key set",
		Value:  string(sources.Keys.Preset), Choices: listSettingPresets(),
	}}
	for _, scope := range KeyScopes {
		count := 0
		for _, action := range sources.Actions {
			if action.Scope == scope {
				count++
			}
		}
		if count == 0 {
			continue
		}
		items = append(items, SettingItem{
			Key: string(scope), Page: string(scope), Label: string(scope),
			Kind: SettingGroup, Detail: DescribeScopeFocus(scope),
			Value: strconv.Itoa(count) + " keys",
		})
	}
	return items
}

// buildChordItems returns one row per action of one scope.
func buildChordItems(sources SettingsSources, scope KeyScope) []SettingItem {
	items := []SettingItem{}
	for _, action := range sources.Actions {
		if action.Scope != scope {
			continue
		}
		_, chosen := sources.Keys.Choices[BuildActionKey(scope, action.ID)]
		items = append(items, SettingItem{
			Key: ItemChordPrefix + action.ID, Label: action.ID, Kind: SettingText,
			Detail: describeActionDetail(action), Value: action.Chords,
			Faint: !chosen,
		})
	}
	return items
}

// describeActionDetail returns the line under one action: the card it belongs to, and what
// it does.
func describeActionDetail(action KeyAction) string {
	switch {
	case action.Owner == "":
		return action.Detail
	case action.Detail == "":
		return action.Owner
	}
	return action.Owner + " · " + action.Detail
}

// DescribeScopeFocus returns what has the keyboard while the keys of one scope reach it.
func DescribeScopeFocus(scope KeyScope) string {
	switch scope {
	case ScopeGlobal:
		return "the workspace, outside cards and prompts"
	case ScopeGrid:
		return "the result grid"
	case ScopePlan:
		return "the plan view"
	case ScopeDocument:
		return "the result document tree"
	case ScopeTree:
		return "the object tree"
	case ScopeEditor:
		return "the SQL editor"
	case ScopeNotebook:
		return "the cell list of a notebook tab"
	case ScopeBuilder:
		return "the diagram of a query builder tab"
	case ScopeList:
		return "a list in a card, and a detail view that scrolls"
	}
	return "the card, the connection picker or the form on show"
}

// buildAppearanceItems returns the theme, the icons, the key hints and the time zone.
func buildAppearanceItems(sources SettingsSources) []SettingItem {
	settings := sources.UI
	theme := settings.Theme
	if theme == "" && len(sources.Themes) > 0 {
		theme = sources.Themes[0]
	}
	return []SettingItem{
		{Key: ItemTheme, Label: "theme", Kind: SettingChoice,
			Value: theme, Choices: sources.Themes},
		{Key: ItemIcons, Label: "icons", Kind: SettingChoice,
			Detail:  "glyph set",
			Value:   string(findChoice(IconSetNames, string(settings.IconSet), IconsPlain)),
			Choices: listModeNames(IconSetNames)},
		{Key: ItemKeyHints, Label: "key hints", Kind: SettingChoice,
			Detail: "number of keys shown",
			Value: string(findChoice(
				KeyHintsModes, string(settings.KeyHints), KeyHintsFull)),
			Choices: listModeNames(KeyHintsModes)},
		{Key: ItemTimeZone, Label: "time zone", Kind: SettingChoice,
			Detail: "zone of timestamps with a time zone",
			Value: string(findChoice(
				TimeZoneModes, string(settings.TimeZone), TimeZoneServer)),
			Choices: listModeNames(TimeZoneModes)},
		{Key: ItemHideSystem, Label: "hide system schemas", Kind: SettingToggle,
			Value: describeToggle(settings.HideSystemSchemas)},
	}
}

// buildMcpItems returns the limits of the MCP server and the connections it serves.
func buildMcpItems(sources SettingsSources, path []string) []SettingItem {
	config := sources.Mcp
	if readPage(path, 0) == PageConnections {
		return buildConnectionItems(sources)
	}

	access := config.Access
	if access == McpUnset {
		access = McpReadOnly
	}
	rows := config.RowLimit
	if rows <= 0 {
		rows = DefaultMcpRowLimit
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = DefaultMcpTimeout
	}
	return []SettingItem{
		{Key: ItemMcpAccess, Label: "access", Kind: SettingChoice,
			Value: string(access), Choices: listModeNames(McpAccessLevels)},
		{Key: ItemMcpRowLimit, Label: "row limit", Kind: SettingText,
			Detail: "max rows per read",
			Value:  strconv.Itoa(rows)},
		{Key: ItemMcpTimeout, Label: "statement timeout", Kind: SettingText,
			Detail: "time limit in milliseconds",
			Value:  strconv.Itoa(int(timeout / time.Millisecond))},
		{Key: PageConnections, Page: PageConnections, Label: "connections",
			Kind: SettingGroup, Detail: "connections served over MCP",
			Value: describeServedCount(sources)},
	}
}

// buildConnectionItems returns one row per connection of the config file.
func buildConnectionItems(sources SettingsSources) []SettingItem {
	items := make([]SettingItem, 0, len(sources.Profiles))
	for _, profile := range sources.Profiles {
		served := ServesMcpProfile(sources.Mcp, profile.Name)
		detail := "not served"
		if served {
			detail = "served, " +
				string(resolveServedAccess(sources.Mcp, profile)) + " access"
		}
		items = append(items, SettingItem{
			Key: ItemProfilePrefix + profile.Name, Label: profile.Name,
			Kind: SettingToggle, Detail: detail, Value: describeToggle(served),
		})
	}
	return items
}

// describeServedCount says how many connections the MCP server opens.
func describeServedCount(sources SettingsSources) string {
	served := 0
	for _, profile := range sources.Profiles {
		if ServesMcpProfile(sources.Mcp, profile.Name) {
			served++
		}
	}
	return strconv.Itoa(served) + " of " + strconv.Itoa(len(sources.Profiles))
}

// resolveServedAccess returns the level one connection has through the MCP server.
func resolveServedAccess(config McpConfig, profile Profile) McpAccess {
	if profile.McpAccess == McpUnset {
		return config.Access
	}
	return ResolveLowerAccess(config.Access, profile.McpAccess)
}

// buildNotebookItems returns the extra notebook directories.
func buildNotebookItems(sources SettingsSources, path []string) []SettingItem {
	settings := sources.Notebooks
	if readPage(path, 0) != PageDirectories {
		return []SettingItem{{
			Key: PageDirectories, Page: PageDirectories, Label: "directories",
			Kind: SettingGroup, Value: strconv.Itoa(len(settings.Paths)),
			Detail: "extra notebook directories",
		}}
	}

	items := make([]SettingItem, 0, len(settings.Paths)+1)
	for _, held := range settings.Paths {
		items = append(items, SettingItem{
			Key: ItemPathPrefix + held, Label: core.ShortenHomePath(held),
			Kind: SettingAction, Detail: "remove this directory",
		})
	}
	return append(items, SettingItem{
		Key: ItemNotebookAdd, Label: "add a directory", Kind: SettingText,
		Detail: "extra directory, beside project and state",
	})
}

// describeAiSource returns the source the chat sends to.
func describeAiSource(config AiConfig) string {
	if config.DefaultAgent != "" {
		return BuildAgentSource(config.DefaultAgent)
	}
	return string(config.DefaultProvider)
}

// listAiSources returns every provider and then every agent.
func listAiSources(config AiConfig) []string {
	sources := make([]string, 0, len(AiProviderIDs)+len(config.Agents))
	for _, id := range AiProviderIDs {
		sources = append(sources, string(id))
	}
	names := make([]string, 0, len(config.Agents))
	for name := range config.Agents {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		sources = append(sources, BuildAgentSource(name))
	}
	return sources
}

// listSettingPresets returns the key presets a choice row steps through.
func listSettingPresets() []string {
	names := make([]string, 0, len(PresetIDs))
	for _, id := range PresetIDs {
		names = append(names, string(id))
	}
	return names
}

// IsSettingOn is true where a row that is on or off holds on.
func IsSettingOn(value string) bool { return value == toggleOn }

// FlipSettingValue returns the other value of a row that is on or off.
func FlipSettingValue(value string) string {
	if IsSettingOn(value) {
		return toggleOff
	}
	return toggleOn
}
