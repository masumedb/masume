package ui

import (
	"context"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/acp"
	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// The settings screen: the sections on the left, the rows of the section under the caret on
// the right. A change is written to the config file as it is made, and the rows are built
// again from what the file now holds.

// settingsPane is the side of the screen that has the keyboard.
type settingsPane int

// The two panes of the screen.
const (
	paneSections settingsPane = iota
	paneItems
)

// SettingsState is the settings screen: the settings it reads and writes, the section on
// show, and the row being typed into.
type SettingsState struct {
	Sources  cfg.SettingsSources
	Sections []cfg.SettingSection
	// Section is the section under the caret of the left pane.
	Section int
	// Path is the pages that are open under the section, the innermost last, and Trail is
	// what each one is called.
	Path  []string
	Trail []string
	Items []cfg.SettingItem
	// Item is the row under the caret of the right pane.
	Item int
	Pane settingsPane
	// Offset is the first row the right pane draws.
	Offset int
	// Draft is the text typed into the row under the caret, and nil where no row is open.
	Draft *app.EditorBuffer
	// Models is the models the agent last answered with, which the model row steps through.
	Models []string
	// Reading is true while an agent is being asked what models it offers.
	Reading bool
	// Saved is true after a change that wrote the file, and Message says what happened.
	Saved   bool
	Message string
}

// NewSettingsState opens the settings screen on the first section.
func NewSettingsState(sources cfg.SettingsSources) *SettingsState {
	state := &SettingsState{Sources: sources, Sections: cfg.ListSettingSections()}
	state.readItems()
	return state
}

// SectionKey returns the section on show.
func (state *SettingsState) SectionKey() string {
	if state.Section < 0 || state.Section >= len(state.Sections) {
		return ""
	}
	return state.Sections[state.Section].Key
}

// readItems builds the rows of the page on show again, so a change shows at once.
func (state *SettingsState) readItems() {
	state.Items = state.offerAgentModels(
		cfg.BuildSettingItems(state.Sources, state.SectionKey(), state.Path))
	state.Item = clamp(state.Item, len(state.Items))
}

// openPage opens the page one row leads to.
func (state *SettingsState) openPage(item cfg.SettingItem) {
	state.closeDraft()
	state.Path = append(state.Path, item.Page)
	state.Trail = append(state.Trail, item.Label)
	state.Item, state.Offset = 0, 0
	state.readItems()
}

// renamePage puts the page on show under another name, which is what a row that renames
// what the page holds needs.
func (state *SettingsState) renamePage(name string) {
	name = strings.TrimSpace(name)
	if len(state.Path) == 0 || name == "" {
		return
	}
	state.Path[len(state.Path)-1] = name
	state.Trail[len(state.Trail)-1] = name
	state.readItems()
}

// closePage closes the page on show, and reports whether a page was closed.
func (state *SettingsState) closePage() bool {
	if len(state.Path) == 0 {
		return false
	}
	name := state.Path[len(state.Path)-1]
	state.closeDraft()
	state.Path = state.Path[:len(state.Path)-1]
	state.Trail = state.Trail[:len(state.Trail)-1]
	state.Offset = 0
	state.readItems()
	state.focusItem(name)
	return true
}

// EditedAgent returns the agent whose page is on show.
func (state *SettingsState) EditedAgent() string {
	if len(state.Path) < 2 || state.Path[0] != cfg.PageAgents {
		return ""
	}
	return state.Path[1]
}

// offerAgentModels puts the models an agent answered with on its model row, so the row steps
// through them.
func (state *SettingsState) offerAgentModels(items []cfg.SettingItem) []cfg.SettingItem {
	if len(state.Models) == 0 || state.EditedAgent() == "" {
		return items
	}
	for at, item := range items {
		if item.Key != cfg.ItemModel {
			continue
		}
		items[at].Kind = cfg.SettingChoice
		items[at].Choices = append([]string{""}, state.Models...)
	}
	return items
}

// FocusedItem returns the row under the caret of the right pane.
func (state *SettingsState) FocusedItem() (cfg.SettingItem, bool) {
	if state.Item < 0 || state.Item >= len(state.Items) {
		return cfg.SettingItem{}, false
	}
	return state.Items[state.Item], true
}

// TypesIntoRow is true while a row is open for typing.
func (state *SettingsState) TypesIntoRow() bool { return state.Draft != nil }

// openDraft opens the row under the caret for typing.
func (state *SettingsState) openDraft(written string) {
	state.Draft = app.NewEditorBuffer(written, len(written))
	state.Saved, state.Message = false, ""
}

// closeDraft drops the text being typed.
func (state *SettingsState) closeDraft() { state.Draft = nil }

// stepSection moves the caret of the left pane and builds the rows of the section it lands on.
func (state *SettingsState) stepSection(step int) {
	state.closeDraft()
	state.Section = wrap(state.Section+step, len(state.Sections))
	state.Path, state.Trail = nil, nil
	state.Item, state.Offset = 0, 0
	state.readItems()
}

// focusItem puts the caret of the right pane on the row of this key.
func (state *SettingsState) focusItem(key string) {
	for at, item := range state.Items {
		if item.Key == key {
			state.Pane, state.Item = paneItems, at
			return
		}
	}
}

// stepItem moves the caret of the right pane.
func (state *SettingsState) stepItem(step int) {
	state.closeDraft()
	state.Item = wrap(state.Item+step, len(state.Items))
}

// showSettings opens the settings screen.
func (model *Model) showSettings() (tea.Model, tea.Cmd) {
	model.settingsForm = NewSettingsState(model.buildSettingsSources())
	model.screen = ScreenSettings
	return model, nil
}

// buildSettingsSources returns everything the settings screen reads.
func (model *Model) buildSettingsSources() cfg.SettingsSources {
	themes := []string{}
	for _, choice := range model.styles.registry.ListThemeChoices() {
		themes = append(themes, choice.Name)
	}
	return cfg.SettingsSources{
		Ai: model.ai, UI: model.settings, Keys: model.keys, Mcp: model.mcp,
		Notebooks: model.notebooks, Themes: themes,
		Profiles: append([]cfg.Profile{}, model.profiles...),
		Actions:  model.buildSettingsActions(),
	}
}

// buildSettingsActions returns every action a chord can be bound to, with the chords it has
// now and what it does. The dialog scope is ordered by the card each action belongs to, so
// the keys of one card stand together.
func (model *Model) buildSettingsActions() []cfg.KeyAction {
	actions := make([]cfg.KeyAction, 0, len(ListActionKeys()))
	for _, actionKey := range ListActionKeys() {
		scope, id := cfg.SplitActionKey(actionKey)
		if !model.offersAi() && IsAiAction(ActionID(id)) {
			continue
		}
		actions = append(actions, cfg.KeyAction{
			Scope: scope, ID: id, Owner: findDialogOwner(scope, id),
			Detail: describeActionHelp(scope, ActionID(id)),
			Chords: cfg.DescribeChordChoice(
				model.registry.FindActionChords(scope, ActionID(id))),
		})
	}
	slices.SortStableFunc(actions, func(one, other cfg.KeyAction) int {
		if one.Scope != other.Scope {
			return 0
		}
		return strings.Compare(one.Owner+" "+one.ID, other.Owner+" "+other.ID)
	})
	return actions
}

// sharedDialogOwner stands for a key several cards read, such as the one that closes them.
const sharedDialogOwner = "any card"

// findDialogOwner returns the card whose keys hold this action alone. A key of no scope but
// the dialog belongs to whichever card is open, and every other scope is one pane, which
// the scope itself already names.
func findDialogOwner(scope cfg.KeyScope, id string) string {
	if scope != cfg.ScopeDialog {
		return ""
	}
	// The AI features are one set, and every key of that set reaches the panel alone.
	if IsAiAction(ActionID(id)) {
		return aiHelpSection
	}
	found := ""
	for _, name := range ListDialogNames() {
		if !namesDialogAction(name, id) {
			continue
		}
		if found != "" {
			return sharedDialogOwner
		}
		found = name
	}
	if found == "" {
		return sharedDialogOwner
	}
	return strings.ReplaceAll(found, "-", " ")
}

// namesDialogAction is true where the keys of this card name the action themselves. The
// actions of a list reach every card, so they are not read here.
func namesDialogAction(name, id string) bool {
	for _, spec := range keyGroups[name] {
		if spec.scope != cfg.ScopeDialog {
			continue
		}
		for _, action := range spec.listActions() {
			if string(action) == id {
				return true
			}
		}
	}
	return false
}

// describeActionHelp returns what one action does, as the help screen says it.
func describeActionHelp(scope cfg.KeyScope, id ActionID) string {
	if action, known := FindAction(scope, id); known && action.Label != "" {
		return action.Label
	}
	for _, section := range HelpSections {
		for _, entry := range section.Entries {
			if entry.Scope != scope {
				continue
			}
			for _, held := range entry.Actions {
				if held == id {
					return readHelpText(entry)
				}
			}
		}
	}
	return ""
}

// leaveSettings closes the screen and goes back to the screen it was opened from.
func (model *Model) leaveSettings() (tea.Model, tea.Cmd) {
	model.settingsForm = nil
	model.screen = model.settingsCameFrom
	return model, nil
}

// readSettingsKey returns what one press does on the settings screen.
func (model *Model) readSettingsKey(key tea.Key) (tea.Model, tea.Cmd) {
	state := model.settingsForm
	if state == nil {
		return model.leaveSettings()
	}
	if state.TypesIntoRow() {
		return model.readSettingsDraftKey(key, state)
	}
	// Escape belongs to an action of the registry as well, and here it goes back one step.
	if key.Code == tea.KeyEscape {
		return model.goSettingsBack()
	}

	// The list scope is read first, so an arrow moves the caret rather than stepping a value.
	match, matched := model.keymap.MatchOnly(key,
		FindDialogActions(settingsGroup), cfg.ScopeList, cfg.ScopeDialog)
	if matched {
		if held, command, ran := model.runSettingsAction(match); ran {
			return held, command
		}
		return model, nil
	}
	// A printable character opens the row under the caret and types into it.
	if item, found := state.FocusedItem(); found && state.Pane == paneItems &&
		item.Kind == cfg.SettingText && key.Text != "" &&
		!key.Mod.Contains(uv.ModCtrl) && !key.Mod.Contains(uv.ModAlt) {
		state.openDraft(describeOpenedValue(item))
		state.Draft.Insert(key.Text)
	}
	return model, nil
}

// readSettingsDraftKey returns what one press does while a row is open for typing.
func (model *Model) readSettingsDraftKey(
	key tea.Key, state *SettingsState,
) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		state.closeDraft()
		return model, nil
	case tea.KeyEnter:
		return model.keepDraft(state), nil
	case tea.KeyTab:
		return model.keepDraft(state), nil
	case tea.KeyLeft:
		state.Draft.MoveCaret(-1, false)
		return model, nil
	case tea.KeyRight:
		state.Draft.MoveCaret(1, false)
		return model, nil
	case tea.KeyHome:
		state.Draft.MoveToLineStart(false)
		return model, nil
	case tea.KeyEnd:
		state.Draft.MoveToLineEnd(false)
		return model, nil
	case tea.KeyBackspace:
		state.Draft.DeleteBackward()
		return model, nil
	case tea.KeyDelete:
		state.Draft.DeleteForward()
		return model, nil
	}
	if key.Text != "" && !key.Mod.Contains(uv.ModCtrl) && !key.Mod.Contains(uv.ModAlt) {
		state.Draft.Insert(key.Text)
	}
	return model, nil
}

// keepDraft writes the text that was typed into the row it was typed in.
func (model *Model) keepDraft(state *SettingsState) tea.Model {
	item, found := state.FocusedItem()
	if !found {
		state.closeDraft()
		return model
	}
	written := state.Draft.Text
	state.closeDraft()
	return model.applySetting(item.Key, written)
}

// pasteIntoSettings inserts pasted text into the row open for typing.
func (model *Model) pasteIntoSettings(written string) (tea.Model, tea.Cmd) {
	state := model.settingsForm
	if state == nil || !state.TypesIntoRow() {
		return model, nil
	}
	state.Draft.Insert(flattenPaste(written))
	return model, nil
}

// runSettingsAction runs one action of the settings screen, whether a key or a press asked
// for it. It reports whether the action belonged to the screen.
func (model *Model) runSettingsAction(match Match) (tea.Model, tea.Cmd, bool) {
	state := model.settingsForm
	if state == nil {
		return model, nil, false
	}
	onItems := state.Pane == paneItems

	switch match.Action {
	case ActionClose:
		held, command := model.goSettingsBack()
		return held, command, true
	case ActionCursorUp:
		model.stepSettingsCaret(-1)
		return model, nil, true
	case ActionCursorDown:
		model.stepSettingsCaret(1)
		return model, nil, true
	case ActionCursorFirstRow:
		model.holdSettingsCaret(0)
		return model, nil, true
	case ActionCursorLastRow:
		model.holdSettingsCaret(model.countSettingsRows() - 1)
		return model, nil, true
	case ActionNextField:
		model.focusSettingsPane(paneItems)
		return model, nil, true
	case ActionPreviousField:
		model.focusSettingsPane(paneSections)
		return model, nil, true
	case ActionNextValue:
		if !onItems {
			model.focusSettingsPane(paneItems)
			return model, nil, true
		}
		if item, found := state.FocusedItem(); found && item.Kind == cfg.SettingGroup {
			state.openPage(item)
			return model, nil, true
		}
		return model.stepSettingsValue(1), nil, true
	case ActionPreviousValue:
		if !onItems || !model.stepsSettingsChoice() {
			held, command := model.goSettingsBack()
			return held, command, true
		}
		return model.stepSettingsValue(-1), nil, true
	case ActionChooseRow, ActionToggleValue:
		if !onItems {
			model.focusSettingsPane(paneItems)
			return model, nil, true
		}
		held, command := model.openSettingsRow()
		return held, command, true
	case ActionListModels:
		if state.EditedAgent() != "" {
			held, command := model.listAgentModels()
			return held, command, true
		}
	}
	return model, nil, false
}

// goSettingsBack goes back one step: the page on show closes, then the sections take the
// keyboard, and then the screen closes.
func (model *Model) goSettingsBack() (tea.Model, tea.Cmd) {
	state := model.settingsForm
	switch {
	case state.TypesIntoRow():
		state.closeDraft()
	case state.closePage():
	case state.Pane == paneItems:
		state.Pane = paneSections
	default:
		return model.leaveSettings()
	}
	return model, nil
}

// focusSettingsPane gives the keyboard to one pane.
func (model *Model) focusSettingsPane(pane settingsPane) {
	state := model.settingsForm
	state.closeDraft()
	state.Pane = pane
}

// countSettingsRows returns how many rows the pane with the keyboard holds.
func (model *Model) countSettingsRows() int {
	state := model.settingsForm
	if state.Pane == paneSections {
		return len(state.Sections)
	}
	return len(state.Items)
}

// stepSettingsCaret moves the caret of the pane with the keyboard.
func (model *Model) stepSettingsCaret(step int) {
	state := model.settingsForm
	if state.Pane == paneSections {
		state.stepSection(step)
		return
	}
	state.stepItem(step)
}

// holdSettingsCaret puts the caret of the pane with the keyboard on one row.
func (model *Model) holdSettingsCaret(row int) {
	state := model.settingsForm
	if state.Pane == paneSections {
		state.stepSection(clamp(row, len(state.Sections)) - state.Section)
		return
	}
	state.closeDraft()
	state.Item = clamp(row, len(state.Items))
}

// stepsSettingsChoice is true where the row under the caret steps through a list of values.
// A row that is on or off is not one: the arrow that steps a value back always goes back,
// so no page can hold the reader.
func (model *Model) stepsSettingsChoice() bool {
	item, found := model.settingsForm.FocusedItem()
	return found && item.Kind == cfg.SettingChoice
}

// stepSettingsValue steps the row under the caret to the value before or after the one it
// holds, and writes it.
func (model *Model) stepSettingsValue(step int) tea.Model {
	state := model.settingsForm
	item, found := state.FocusedItem()
	if !found {
		return model
	}
	switch item.Kind {
	case cfg.SettingToggle:
		return model.applySetting(item.Key, cfg.FlipSettingValue(item.Value))
	case cfg.SettingChoice:
		if len(item.Choices) == 0 {
			return model
		}
		at := 0
		for index, choice := range item.Choices {
			if choice == item.Value {
				at = index
				break
			}
		}
		return model.applySetting(item.Key, item.Choices[wrap(at+step, len(item.Choices))])
	}
	return model
}

// openSettingsRow runs the row under the caret: a toggle flips, a choice steps on, a row of
// text opens for typing, and an action runs.
func (model *Model) openSettingsRow() (tea.Model, tea.Cmd) {
	state := model.settingsForm
	item, found := state.FocusedItem()
	if !found {
		return model, nil
	}
	switch item.Kind {
	case cfg.SettingGroup:
		state.openPage(item)
		return model, nil
	case cfg.SettingToggle, cfg.SettingChoice:
		return model.stepSettingsValue(1), nil
	case cfg.SettingText:
		state.openDraft(describeOpenedValue(item))
		return model, nil
	}
	return model.runSettingsRowAction(item)
}

// describeOpenedValue returns what a row of text holds when it opens for typing. A row of a
// chord opens empty: the chord typed takes the place of the one the row has, and a row left
// empty takes back the chord of the preset.
func describeOpenedValue(item cfg.SettingItem) string {
	if strings.HasPrefix(item.Key, cfg.ItemChordPrefix) {
		return ""
	}
	return item.Value
}

// runSettingsRowAction runs an action row. A row that removes something asks first.
func (model *Model) runSettingsRowAction(item cfg.SettingItem) (tea.Model, tea.Cmd) {
	switch {
	case item.Key == cfg.ItemRemoveAgent:
		return model.askRemoveSetting(item, " remove agent ",
			"Remove "+model.settingsForm.EditedAgent()+" from the config file?")
	case strings.HasPrefix(item.Key, cfg.ItemPathPrefix):
		return model.askRemoveSetting(item, " remove directory ",
			"Remove "+item.Label+" from the notebook directories?")
	}
	return model.applySetting(item.Key, ""), nil
}

// askRemoveSetting asks before a row that removes something runs.
func (model *Model) askRemoveSetting(
	item cfg.SettingItem, title, body string,
) (tea.Model, tea.Cmd) {
	model.confirm = &confirmState{
		Title: title, Body: body, Destructive: true,
		Answer: func(yes bool) tea.Cmd {
			if yes {
				model.applySetting(item.Key, "")
			}
			model.confirm = nil
			return nil
		},
	}
	return model, nil
}

// applySetting writes one row into the settings and into the config file, and applies it to
// the running client.
func (model *Model) applySetting(key, value string) tea.Model {
	state := model.settingsForm
	built, updates, err := cfg.ApplySetting(state.Sources, state.Path, key, value)
	if err != nil {
		state.Saved, state.Message = false, err.Error()
		return model
	}
	if len(updates) == 0 {
		return model
	}
	if err := cfg.SaveTables(cfg.ResolveConfigPath(), updates); err != nil {
		state.Saved, state.Message = false, db.DescribeError(err)
		return model
	}

	was := state.Sources
	state.Sources = built
	// The bindings are built again before the rows are, so a row of a key shows the chord
	// the client now answers.
	problems := model.applySettingsSources(built)
	state.Sources.Actions = model.buildSettingsActions()
	state.readItems()

	// The page of an agent that was just added opens, because it is renamed first. A
	// removed agent leaves no page to stand on, so its list comes back.
	switch key {
	case cfg.ItemAgentName:
		// The page of the agent is the page of its name, so a rename carries the page
		// with it.
		state.renamePage(value)
	case cfg.ItemAddAgent:
		state.openPage(cfg.SettingItem{
			Page: findAddedAgent(was.Ai, built.Ai), Label: "agent",
		})
		state.Trail[len(state.Trail)-1] = state.Path[len(state.Path)-1]
		state.readItems()
	case cfg.ItemRemoveAgent:
		state.closePage()
	}

	if len(problems) > 0 {
		state.Saved, state.Message = false, problems[0]
		return model
	}
	state.Saved, state.Message = true, "Saved"
	return model
}

// findAddedAgent returns the agent the settings gained.
func findAddedAgent(was, built cfg.AiConfig) string {
	for name := range built.Agents {
		if _, held := was.Agents[name]; !held {
			return name
		}
	}
	return ""
}

// applySettingsSources applies saved settings to the running client, and returns what the
// keys of those settings clash with.
func (model *Model) applySettingsSources(sources cfg.SettingsSources) []string {
	model.applyAiSettings(sources.Ai)
	model.mcp = sources.Mcp
	model.notebooks = sources.Notebooks
	model.applyInterfaceSettings(sources.UI)
	if model.keys.Preset == sources.Keys.Preset &&
		sameChordChoices(model.keys.Choices, sources.Keys.Choices) {
		return nil
	}
	model.keys = sources.Keys
	return model.applyKeySettings()
}

// sameChordChoices is true where two sets of chosen chords bind the same actions the same
// way.
func sameChordChoices(held, other cfg.ChordChoices) bool {
	if len(held) != len(other) {
		return false
	}
	for actionKey, sequences := range held {
		taken, bound := other[actionKey]
		if !bound || cfg.DescribeChordChoice(sequences) !=
			cfg.DescribeChordChoice(taken) {
			return false
		}
	}
	return true
}

// applyInterfaceSettings applies the theme, the icons and the key hints at once.
func (model *Model) applyInterfaceSettings(settings cfg.UISettings) {
	changesTheme := model.settings.Theme != settings.Theme
	model.settings = settings
	model.icons = BuildIconSet(settings.IconSet, settings.IconGlyphs)
	if !changesTheme || settings.Theme == "" {
		return
	}
	if _, applied := model.styles.ApplyThemeByName(settings.Theme); !applied {
		model.problems = append(model.problems,
			"unknown theme: \""+settings.Theme+"\"")
	}
}

// applyKeySettings builds the bindings again from the preset and the chords of the file,
// and returns what those chords clash with.
func (model *Model) applyKeySettings() []string {
	problems := model.registry.ApplyKeySettings(
		FindKeyPreset(model.keys.Preset), model.keys.Choices, model.offersAi())
	for _, problem := range problems {
		model.problems = append(model.problems, "keys: "+problem)
	}
	return problems
}

// applyAiSettings applies saved AI settings to the running client. A chat that was switched
// off takes its chords with it, so the bindings are built again.
func (model *Model) applyAiSettings(config cfg.AiConfig) {
	rebuilds := model.ai.Enabled != config.Enabled
	model.ai = config
	model.aiProvider = config.DefaultProvider
	model.aiAgent = config.DefaultAgent
	if rebuilds {
		model.applyKeySettings()
	}
}

// agentModelsRead is the models one agent answered with, or why it answered none.
type agentModelsRead struct {
	agent  string
	models []acp.ModelOption
	err    error
}

// modelListTimeout is the time an agent has to report the models it offers.
const modelListTimeout = 2 * time.Minute

// readAgentModels starts the agent, reads the models it offers and closes it. It asks the
// agent nothing, so it spends nothing.
func readAgentModels(settings cfg.AiAgentSettings) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), modelListTimeout)
		defer stop()
		found, err := acp.ListModels(ctx, settings)
		return agentModelsRead{agent: settings.Name, models: found, err: err}
	}
}

// listAgentModels asks the agent whose page is on show what models it offers.
func (model *Model) listAgentModels() (tea.Model, tea.Cmd) {
	state := model.settingsForm
	name := state.EditedAgent()
	settings, held := state.Sources.Ai.Agents[name]
	if !held {
		return model, nil
	}
	state.Reading = true
	state.Saved, state.Message = false, "reading the models of "+name+"…"
	return model, readAgentModels(settings)
}

// keepAgentModels puts the models the agent answered with on the model row.
func (model *Model) keepAgentModels(read agentModelsRead) (tea.Model, tea.Cmd) {
	state := model.settingsForm
	if state == nil || !state.Reading {
		return model, nil
	}
	state.Reading = false
	if read.err != nil {
		state.Saved, state.Message = false, db.DescribeError(read.err)
		return model, nil
	}

	choices := make([]string, 0, len(read.models))
	for _, option := range read.models {
		choices = append(choices, option.Value)
	}
	state.Models = choices
	state.readItems()
	state.Saved = false
	state.Message = read.agent + " offers " + strconv.Itoa(len(choices)) + " models"
	return model, nil
}

// The shape of the settings card.
const (
	widestSettingsCard    = 76
	narrowestSettingsCard = 46
	// settingsSidebarWidth is the column the sections keep.
	settingsSidebarWidth = 15
	// widestSettingsValue is the room a value keeps beside the name of its row.
	widestSettingsValue = 18
	// widestSettingsField is the field a row of text is typed into.
	widestSettingsField = 36
	// settingsDividerWidth is the line between the two panes, with a blank on each side.
	settingsDividerWidth = 3
	// settingsCardChrome is the rows the card keeps besides the panes: the two borders, the
	// blank row inside each, the blank row over the foot, the line that says what the row
	// under the caret is, and the row of keys. The card is placed from this count, so a
	// count that is one out moves every row of the card under the pointer.
	settingsCardChrome = 7
	// settingsBareChrome is the rows of a card with no foot: the two borders and the blank
	// row inside each.
	settingsBareChrome = 4
	// fewestSettingsRows is the rows the panes keep while the card draws its foot.
	fewestSettingsRows = 3
)

// renderSettings draws the sections beside the rows of the section on show.
func (model *Model) renderSettings() string {
	state := model.settingsForm
	theme := model.styles.Theme
	cardWidth := present.ResolveCardWidth(
		widestSettingsCard, narrowestSettingsCard, model.width)
	inner := max(cardWidth-4, 1)
	itemWidth := max(inner-settingsSidebarWidth-settingsDividerWidth, 8)

	rows, drawsFoot := model.resolveSettingsRows()
	state.Offset = scrollTo(state.Item, state.Offset, rows, len(state.Items))
	left := halfRoundedUp(model.width - cardWidth)
	chrome := settingsBareChrome
	if drawsFoot {
		chrome = settingsCardChrome
	}
	cardTop := titleBarRows + halfRoundedUp(model.height-2-(rows+chrome))

	lines := make([]string, 0, rows+3)
	// A page with more rows than the card shows draws a bar in the last column of its rows.
	thumb := buildScrollThumb(state.Offset, rows, len(state.Items))
	for at := range rows {
		item := model.renderItemCell(at, itemWidth)
		if at < len(thumb) {
			item = model.styles.paintThumbColumn(item, thumb[at], itemWidth-1, theme.Panel)
		}
		lines = append(lines,
			model.renderSectionCell(at)+
				paintText(theme.Faint, theme.Panel, " │ ")+item)
	}

	// A screen too short for the foot draws the rows alone, so the card still fits the
	// frame and the row a press lands on is the row under the pointer.
	if drawsFoot {
		lines = append(lines, "", model.styles.Faint().Render(
			present.TruncateText(model.describeSettingsDetail(), inner)))
		keys := model.buildKeyLineOf(settingsKeySpecs, keyScene{})
		if text := present.TruncateText(keys.buildText(), inner); text != "" {
			lines = append(lines, model.renderKeyLine(keys, []string{text},
				cardTop+cardBodyRow+len(lines), left+cardBodyColumn, theme.Panel)[0])
		}
	}

	model.layout.settingsSections = rowsHit{
		top: cardTop + cardBodyRow, count: min(rows, len(state.Sections)),
		from: left + cardBodyColumn, to: left + cardBodyColumn + settingsSidebarWidth,
	}
	model.layout.settingsRows = rowsHit{
		top: cardTop + cardBodyRow, offset: state.Offset,
		count: min(rows, len(state.Items)-state.Offset),
		from:  left + cardBodyColumn + settingsSidebarWidth + settingsDividerWidth,
		to:    left + cardWidth - 2,
	}
	return model.renderCard(model.describeSettingsTitle(), cardWidth, lines, plainCard)
}

// describeSettingsTitle returns the heading of the card: the settings, and the pages that
// are open under the section.
func (model *Model) describeSettingsTitle() string {
	written := " settings"
	for _, page := range model.settingsForm.Trail {
		written += " · " + page
	}
	return written + " "
}

// resolveSettingsRows returns how many rows the panes draw, and whether the card has room
// for the foot under them.
func (model *Model) resolveSettingsRows() (int, bool) {
	state := model.settingsForm
	wanted := max(len(state.Sections), len(state.Items))
	body := model.height - 2
	if room := body - settingsCardChrome; room >= fewestSettingsRows {
		return min(wanted, room), true
	}
	return max(min(wanted, body-settingsBareChrome), 1), false
}

// renderSectionCell draws one row of the left pane.
func (model *Model) renderSectionCell(at int) string {
	state := model.settingsForm
	theme := model.styles.Theme
	if at >= len(state.Sections) {
		return paintBlanks(theme.Panel, settingsSidebarWidth)
	}

	label := present.FitText(
		" "+present.TruncateText(state.Sections[at].Label, settingsSidebarWidth-2),
		settingsSidebarWidth)
	switch {
	case at != state.Section:
		return paintText(theme.Muted, theme.Panel, label)
	case state.Pane == paneSections:
		return paintText(theme.Text, theme.Header, label)
	default:
		return paintText(theme.Accent, theme.Panel, label)
	}
}

// renderItemCell draws one row of the right pane: the name of the setting and what it holds.
func (model *Model) renderItemCell(at, width int) string {
	state := model.settingsForm
	theme := model.styles.Theme
	index := state.Offset + at
	if index >= len(state.Items) {
		return paintBlanks(theme.Panel, width)
	}

	item := state.Items[index]
	focused := index == state.Item && state.Pane == paneItems
	ground := theme.Panel
	if focused {
		ground = theme.Header
	}

	labelWidth := model.resolveSettingsLabelWidth(width)
	labelInk := theme.Muted
	switch {
	case focused:
		labelInk = theme.Text
	case item.Kind == cfg.SettingAction:
		labelInk = theme.Accent
	}
	label := paintText(labelInk, ground,
		present.FitText(" "+present.TruncateText(item.Label, labelWidth-1), labelWidth))
	return label + model.renderItemValue(item, focused, max(width-labelWidth, 1), ground)
}

// resolveSettingsLabelWidth returns the column the names of the page keep: what the longest
// one needs, and never so much that the value beside it has no room.
func (model *Model) resolveSettingsLabelWidth(width int) int {
	longest := 0
	for _, item := range model.settingsForm.Items {
		longest = max(longest, present.MeasureText(item.Label))
	}
	return min(longest+2, max(width-widestSettingsValue, 8))
}

// renderItemValue draws what one row holds, in the form its kind is changed by. The value
// stands at the right end of the row, as a settings page of a desktop client draws it.
func (model *Model) renderItemValue(
	item cfg.SettingItem, focused bool, width int, ground color.Color,
) string {
	state := model.settingsForm
	theme := model.styles.Theme
	if item.Kind == cfg.SettingText && focused && state.TypesIntoRow() {
		// The field keeps one column for the caret it draws after the last character,
		// and one for the blank every other row ends with.
		room := min(max(width-2, 1), widestSettingsField)
		return paintBlanks(ground, max(width-room-1, 0)) +
			model.renderField(state.Draft, room, FieldLook{
				Ground: theme.Accent, Ink: theme.OnAccent, Focused: true,
				Placeholder: item.Value, KeepsPlaceholder: true,
			})
	}

	ink := theme.Muted
	if focused {
		ink = theme.Text
	}
	switch item.Kind {
	case cfg.SettingToggle:
		dot, word, dotInk := model.icons.Icon(cfg.IconDot), "off", theme.Faint
		if cfg.IsSettingOn(item.Value) {
			word, dotInk = "on", theme.Success
		}
		if dot != "" {
			dot += " "
		}
		return alignSettingValue(width, ground,
			paintText(dotInk, ground, dot)+paintText(ink, ground, word),
			present.MeasureText(dot+word))
	case cfg.SettingAction:
		return paintBlanks(ground, width)
	case cfg.SettingChoice:
		return model.renderSettingsChoice(item.Value, focused, width, ground)
	case cfg.SettingGroup:
		mark := " " + model.icons.Icon(cfg.IconStepOn)
		written := present.TruncateText(item.Value,
			max(width-present.MeasureText(mark)-1, 1))
		if item.Faint {
			ink = theme.Faint
		}
		return alignSettingValue(width, ground,
			paintText(ink, ground, written)+paintText(theme.Accent, ground, mark),
			present.MeasureText(written+mark))
	}

	written := item.Value
	switch {
	case written == "":
		written, ink = "not set", theme.Faint
	case item.Unit != "":
		written += " " + item.Unit
	}
	written = present.TruncateText(written, width-1)
	return alignSettingValue(width, ground,
		paintText(ink, ground, written), present.MeasureText(written))
}

// alignSettingValue puts one value at the right end of the row, with a blank after it.
func alignSettingValue(width int, ground color.Color, written string, measured int) string {
	return paintBlanks(ground, max(width-measured-1, 0)) + written +
		paintBlanks(ground, min(1, width))
}

// renderSettingsChoice draws the value of a row that steps through values, with a mark on
// each side that a press steps it by. The marks take the accent on the focused row.
func (model *Model) renderSettingsChoice(
	value string, focused bool, width int, ground color.Color,
) string {
	theme := model.styles.Theme
	markInk, valueInk := theme.Faint, theme.Muted
	if focused {
		markInk, valueInk = theme.Accent, theme.Text
	}
	written := present.TruncateText(value, max(width-choiceMarkChrome-1, 1))
	if written == "" {
		written = "not set"
	}
	return alignSettingValue(width, ground,
		paintText(markInk, ground, model.icons.Icon(cfg.IconStepBack)+" ")+
			paintText(valueInk, ground, written)+
			paintText(markInk, ground, " "+model.icons.Icon(cfg.IconStepOn)),
		present.MeasureText(written)+choiceMarkChrome)
}

// describeSettingsDetail returns the line that says what the row under the caret is.
func (model *Model) describeSettingsDetail() string {
	state := model.settingsForm
	if state.Pane == paneSections {
		if state.Section < len(state.Sections) {
			return state.Sections[state.Section].Detail
		}
		return ""
	}
	if item, found := state.FocusedItem(); found {
		return item.Detail
	}
	return ""
}

// describeSettingsNotice returns what the status bar reports of the last change.
func (model *Model) describeSettingsNotice() (string, app.NoticeTone) {
	state := model.settingsForm
	if state == nil || state.Message == "" {
		return "", app.NoticeInfo
	}
	if state.Saved {
		return state.Message, app.NoticeActive
	}
	return model.writeProblemSign() + state.Message, app.NoticeError
}

// pressSettings returns a press on the settings screen.
func (model *Model) pressSettings(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	state := model.settingsForm
	if state == nil {
		return model, nil
	}
	if scope, action, key, pressed := findButton(
		model.layout.buttons, mouse.X, mouse.Y); pressed {
		model.frame.flashKey(key)
		if held, command, ran := model.runSettingsAction(
			Match{Action: action, Scope: scope}); ran {
			return held, command
		}
		return model, nil
	}

	if row, found := model.layout.settingsSections.holds(mouse.X, mouse.Y); found {
		state.Pane = paneSections
		state.stepSection(clamp(row, len(state.Sections)) - state.Section)
		return model, nil
	}
	row, found := model.layout.settingsRows.holds(mouse.X, mouse.Y)
	if !found {
		return model, nil
	}
	if state.Pane == paneItems && row == state.Item {
		return model.openSettingsRow()
	}
	state.closeDraft()
	state.Pane, state.Item = paneItems, clamp(row, len(state.Items))
	return model, nil
}
