package ui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/acp"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// openSettings opens the settings screen on a client with a config file of its own, and
// returns the path of that file.
func openSettings(t *testing.T, model *Model) string {
	t.Helper()
	path := useConfigFile(t)
	model.settingsCameFrom = model.screen
	model.showSettings()
	return path
}

// The presses the settings screen reads.
func pressOpen() tea.Key  { return tea.Key{Code: tea.KeyEnter} }
func pressBack() tea.Key  { return tea.Key{Code: tea.KeyEscape} }
func pressDown() tea.Key  { return tea.Key{Code: tea.KeyDown} }
func pressUp() tea.Key    { return tea.Key{Code: tea.KeyUp} }
func pressRight() tea.Key { return tea.Key{Code: tea.KeyRight} }
func pressLeft() tea.Key  { return tea.Key{Code: tea.KeyLeft} }
func pressTab() tea.Key   { return tea.Key{Code: tea.KeyTab} }

// typeInto presses one character at a time.
func typeInto(model *Model, text string) {
	for _, held := range text {
		model.readKey(tea.Key{Code: held, Text: string(held)})
	}
}

// showSection puts the caret of the left pane on this section.
func showSection(t *testing.T, model *Model, key string) {
	t.Helper()
	state := model.settingsForm
	for at, section := range state.Sections {
		if section.Key == key {
			state.Pane = paneSections
			state.stepSection(at - state.Section)
			return
		}
	}
	t.Fatalf("the settings hold no %s section", key)
}

// openPage opens the page of this row, from the page on show.
func openPage(t *testing.T, model *Model, key string) {
	t.Helper()
	focusItem(t, model, key)
	model.readKey(pressOpen())
}

// showAgentPage opens the settings of one agent, from the section on show.
func showAgentPage(t *testing.T, model *Model, name string) {
	t.Helper()
	showSection(t, model, cfg.SectionAi)
	openPage(t, model, cfg.PageAgents)
	openPage(t, model, name)
}

// showProviderPage opens the settings of the provider the chat sends to.
func showProviderPage(t *testing.T, model *Model) {
	t.Helper()
	showSection(t, model, cfg.SectionAi)
	openPage(t, model, cfg.PageProviders)
	openPage(t, model, string(model.aiProvider))
}

// focusItem puts the caret of the right pane on the row of this key.
func focusItem(t *testing.T, model *Model, key string) {
	t.Helper()
	state := model.settingsForm
	for at, item := range state.Items {
		if item.Key == key {
			state.Pane, state.Item = paneItems, at
			return
		}
	}
	t.Fatalf("the section holds no %s: %v", key, listItemKeysOf(state))
}

// listItemKeysOf returns the key of every row on show.
func listItemKeysOf(state *SettingsState) []string {
	keys := []string{}
	for _, item := range state.Items {
		keys = append(keys, item.Key)
	}
	return keys
}

// readConfigFile returns what the settings screen wrote.
func readConfigFile(t *testing.T, path string) string {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the config file: %v", err)
	}
	return string(written)
}

// The client sends to the source the config file names, so a save survives a restart.
func TestTheClientStartsOnTheSourceTheFileNames(t *testing.T) {
	loaded := loadedConfigForTest("tokyonight")
	loaded.Ai.DefaultAgent = "opencode"
	model := NewModel(loaded, nil, nil, nil)

	if model.aiAgent != "opencode" {
		t.Errorf("the chat sends to agent %q", model.aiAgent)
	}
	if held := model.describeChatSource(); held != "opencode agent" {
		t.Errorf("the panel names %q", held)
	}
}

// The screen draws the sections beside the rows of the section on show, so the settings are
// read without walking into a page and back out of it.
func TestTheSettingsScreenDrawsTheSectionsBesideTheRows(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)

	drawn := stripEscapes(model.renderSettings())
	for _, wanted := range []string{"AI", "Appearance", "Keys", "MCP", "Notebooks"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the sections do not hold %q:\n%s", wanted, drawn)
		}
	}
	for _, wanted := range []string{"ai chat", "source", "providers", "agents"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the AI rows do not hold %q:\n%s", wanted, drawn)
		}
	}
}

// The caret of the left pane changes the rows on the right as it moves, so a section is read
// by stepping to it.
func TestMovingDownTheSectionsShowsTheirRowsAtOnce(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	state := model.settingsForm

	model.readKey(pressDown())
	if state.SectionKey() != cfg.SectionAppearance {
		t.Fatalf("the section reads %q", state.SectionKey())
	}
	if !strings.Contains(strings.Join(listItemKeysOf(state), ","), cfg.ItemTheme) {
		t.Errorf("the rows read %v", listItemKeysOf(state))
	}
	if !strings.Contains(stripEscapes(model.renderSettings()), "theme") {
		t.Error("the theme row is not drawn")
	}

	model.readKey(pressUp())
	if state.SectionKey() != cfg.SectionAi {
		t.Errorf("the section reads %q", state.SectionKey())
	}
}

// An arrow moves the caret of the pane that has the keyboard, and never the other one.
func TestTheArrowsMoveTheCaretOfTheFocusedPane(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	state := model.settingsForm

	model.readKey(pressTab())
	if state.Pane != paneItems {
		t.Fatal("the rows did not take the keyboard")
	}
	model.readKey(pressDown())
	if state.Item != 1 {
		t.Errorf("the caret of the rows stands on %d", state.Item)
	}
	if state.Section != 0 {
		t.Errorf("the caret of the sections moved to %d", state.Section)
	}

	// The row under the caret is the source, which steps through its values. The arrow
	// that leaves the rows is the one on a row that steps through nothing.
	focusItem(t, model, cfg.ItemChatTimeout)
	model.readKey(pressLeft())
	if state.Pane != paneSections {
		t.Error("the sections did not take the keyboard back")
	}
}

// A row that is on or off is written as it is flipped, and the client applies it at once.
func TestFlippingARowWritesItAndAppliesIt(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	showSection(t, model, cfg.SectionAppearance)
	focusItem(t, model, cfg.ItemHideSystem)
	was := model.settings.HideSystemSchemas

	model.readKey(pressOpen())

	if model.settings.HideSystemSchemas == was {
		t.Error("the client did not apply the change")
	}
	if !strings.Contains(readConfigFile(t, path), "hide_system_schemas") {
		t.Errorf("the file holds no change:\n%s", readConfigFile(t, path))
	}
	if !model.settingsForm.Saved {
		t.Errorf("the screen reports %q", model.settingsForm.Message)
	}
	if !strings.Contains(stripEscapes(model.render()), "Saved") {
		t.Error("the status bar does not report the change")
	}
}

// A row that steps through values writes each step, so the chat sends to the source on show.
func TestSteppingTheSourceSendsTheChatToIt(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	focusItem(t, model, cfg.ItemChatSource)
	was := model.aiProvider

	model.readKey(pressRight())

	if model.aiProvider == was && model.aiAgent == "" {
		t.Errorf("the chat still sends to %q", model.aiProvider)
	}
	written := readConfigFile(t, path)
	if !strings.Contains(written, "default_provider") {
		t.Errorf("the file names no source:\n%s", written)
	}
}

// A row of text opens for typing, and what was typed is written on Enter.
func TestTypingIntoARowWritesItOnEnter(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	showProviderPage(t, model)
	focusItem(t, model, cfg.ItemModel)

	model.readKey(pressOpen())
	if !model.settingsForm.TypesIntoRow() {
		t.Fatal("the row did not open for typing")
	}
	model.settingsForm.Draft = nil
	focusItem(t, model, cfg.ItemModel)
	typeInto(model, "haiku")
	if !model.settingsForm.TypesIntoRow() {
		t.Fatal("a character did not open the row for typing")
	}
	model.readKey(pressOpen())

	if model.settingsForm.TypesIntoRow() {
		t.Error("the row is still open for typing")
	}
	if held := model.ai.Providers[model.aiProvider].Model; !strings.HasSuffix(
		held, "haiku") {
		t.Errorf("the model reads %q", held)
	}
	if !strings.Contains(readConfigFile(t, path), "haiku") {
		t.Errorf("the model was not written:\n%s", readConfigFile(t, path))
	}
}

// Escape goes back one step at a time: the typing, the page, the pane, and then the screen.
// A page with no way back is what a settings screen must never leave the reader in.
func TestEscapeGoesBackOneStepAtATime(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showProviderPage(t, model)
	focusItem(t, model, cfg.ItemModel)
	was := model.ai.Providers[model.aiProvider].Model

	model.readKey(pressOpen())
	typeInto(model, "typed")
	model.readKey(pressBack())
	state := model.settingsForm
	if state == nil || state.TypesIntoRow() {
		t.Fatal("Escape did not drop the typing")
	}
	if held := model.ai.Providers[model.aiProvider].Model; held != was {
		t.Errorf("the dropped typing was written: %q", held)
	}

	model.readKey(pressBack())
	if len(state.Path) != 1 {
		t.Fatalf("Escape stands on %v, wanted the list of the providers", state.Path)
	}
	model.readKey(pressBack())
	if len(state.Path) != 0 {
		t.Fatalf("Escape stands on %v, wanted the section itself", state.Path)
	}
	model.readKey(pressBack())
	if state.Pane != paneSections {
		t.Fatal("Escape did not give the keyboard back to the sections")
	}
	model.readKey(pressBack())
	if model.settingsForm != nil {
		t.Error("the screen is still open")
	}
}

// The arrow that steps a value back closes the page where the row under the caret steps
// through nothing, so the reader is never held on a page.
func TestTheLeftArrowClosesThePageUnderTheCaret(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showSection(t, model, cfg.SectionAppearance)
	focusItem(t, model, cfg.ItemTheme)
	state := model.settingsForm

	// The theme row steps through the themes, so the arrow changes it.
	model.readKey(pressLeft())
	if state.Pane != paneItems {
		t.Fatal("the arrow left a row that steps through values")
	}
	focusItem(t, model, cfg.ItemHideSystem)
	model.readKey(pressLeft())
	if state.Pane != paneSections {
		t.Fatal("the arrow did not give the keyboard back to the sections")
	}

	showSection(t, model, cfg.SectionMcp)
	openPage(t, model, cfg.PageConnections)
	model.readKey(pressLeft())
	if len(state.Path) != 0 {
		t.Errorf("the arrow stands on %v", state.Path)
	}
}

// A value the settings cannot use is reported and nothing is written.
func TestAValueThatIsRefusedIsReported(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	focusItem(t, model, cfg.ItemChatTimeout)

	model.readKey(pressOpen())
	typeInto(model, "soon")
	model.readKey(pressOpen())

	state := model.settingsForm
	if state.Saved || state.Message == "" {
		t.Errorf("the screen reports %q", state.Message)
	}
	if strings.Contains(readConfigFile(t, path), "soon") {
		t.Errorf("the refused value was written:\n%s", readConfigFile(t, path))
	}
	if !strings.Contains(stripEscapes(model.render()), "soon") {
		t.Error("the status bar does not say what was refused")
	}
}

// The row that adds an agent sends the chat to it and puts the caret on its name, because a
// new agent is renamed first.
func TestAddingAnAgentPutsTheCaretOnItsName(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	showSection(t, model, cfg.SectionAi)
	openPage(t, model, cfg.PageAgents)
	focusItem(t, model, cfg.ItemAddAgent)

	model.readKey(pressOpen())

	state := model.settingsForm
	if held := state.EditedAgent(); held == "" {
		t.Fatalf("the page of the added agent is not open: %v", state.Path)
	}
	item, found := state.FocusedItem()
	if !found || item.Key != cfg.ItemAgentName {
		t.Errorf("the caret stands on %+v", item)
	}
	if !strings.Contains(readConfigFile(t, path), "[ai.agents."+item.Value+"]") {
		t.Errorf("the agent has no table:\n%s", readConfigFile(t, path))
	}
}

// A row that removes an agent asks first, and a no leaves the agent alone.
func TestRemovingAnAgentAsksFirst(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showAgentPage(t, model, "opencode")
	focusItem(t, model, cfg.ItemUseSource)
	model.readKey(pressOpen())
	focusItem(t, model, cfg.ItemRemoveAgent)

	model.readKey(pressOpen())
	if model.confirm == nil {
		t.Fatal("the agent was removed without a question")
	}
	model.confirm.Answer(false)
	if _, held := model.settingsForm.Sources.Ai.Agents["opencode"]; !held {
		t.Error("a no still removed the agent")
	}

	model.readKey(pressOpen())
	if model.confirm == nil {
		t.Fatal("the question was not asked again")
	}
	model.confirm.Answer(true)
	if _, held := model.settingsForm.Sources.Ai.Agents["opencode"]; held {
		t.Error("a yes left the agent in the file")
	}
	if model.aiAgent != "" {
		t.Errorf("the chat still sends to %q", model.aiAgent)
	}
	// The page of the agent is gone, so the list of the agents is what stands there.
	if held := strings.Join(listItemKeysOf(model.settingsForm), ","); !strings.Contains(
		held, cfg.ItemAddAgent) {
		t.Errorf("the screen stands on %q", held)
	}
}

// The models an agent answers with are put on the model row, which then steps through them.
func TestTheModelRowStepsThroughTheModelsTheAgentOffers(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showAgentPage(t, model, "opencode")
	state := model.settingsForm

	if _, command, _ := model.runSettingsAction(
		Match{Action: ActionListModels, Scope: cfg.ScopeDialog}); command == nil {
		t.Fatal("the screen asked no agent for its models")
	}
	model.keepAgentModels(agentModelsRead{agent: "opencode", models: []acp.ModelOption{
		{Value: "grok-code", Name: "Grok Code"}, {Value: "qwen", Name: "Qwen"},
	}})

	focusItem(t, model, cfg.ItemModel)
	item, _ := state.FocusedItem()
	if item.Kind != cfg.SettingChoice {
		t.Fatalf("the model row is a %q", item.Kind)
	}
	if strings.Join(item.Choices, ",") != ",grok-code,qwen" {
		t.Errorf("the model row offers %v", item.Choices)
	}
	if !strings.Contains(state.Message, "2 models") {
		t.Errorf("the screen reports %q", state.Message)
	}
}

// An agent that answers nothing reports why, and the screen stays as it was.
func TestAnAgentThatAnswersNoModelsReportsWhy(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	state := model.settingsForm
	state.Reading = true

	model.keepAgentModels(agentModelsRead{
		agent: "opencode", err: errors.New("opencode is not on the PATH"),
	})

	if state.Saved || !strings.Contains(state.Message, "not on the PATH") {
		t.Errorf("the screen reports %q", state.Message)
	}
	if state.Reading {
		t.Error("the screen is still waiting for the agent")
	}
}

// A press on a section shows its rows, and a press on the row under the caret opens it.
func TestAPressOnASectionShowsItsRows(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	model.render()
	state := model.settingsForm

	sections := model.layout.settingsSections
	model.pressSettings(tea.Mouse{X: sections.from + 1, Y: sections.top + 1})
	if state.SectionKey() != cfg.SectionAppearance {
		t.Fatalf("the section reads %q", state.SectionKey())
	}
	if state.Pane != paneSections {
		t.Error("the press did not give the keyboard to the sections")
	}

	model.render()
	rows := model.layout.settingsRows
	model.pressSettings(tea.Mouse{X: rows.from + 1, Y: rows.top})
	if state.Pane != paneItems || state.Item != 0 {
		t.Errorf("the press stands on %d of pane %d", state.Item, state.Pane)
	}
}

// The MCP section holds one row per connection of the config file.
func TestTheMcpSectionHoldsTheConnections(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	model.settingsForm.Sources.Profiles = []cfg.Profile{{Name: "shop"}}
	showSection(t, model, cfg.SectionMcp)
	openPage(t, model, cfg.PageConnections)
	focusItem(t, model, cfg.ItemProfilePrefix+"shop")

	model.readKey(pressOpen())

	if !cfg.ServesMcpProfile(model.mcp, "shop") {
		t.Errorf("the server serves %v", model.mcp.Profiles)
	}
	if !strings.Contains(readConfigFile(t, path), "shop") {
		t.Errorf("the connection was not written:\n%s", readConfigFile(t, path))
	}
}

// The key line of the screen names the keys the row under the caret takes.
func TestTheKeyLineFollowsTheRowUnderTheCaret(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	focusItem(t, model, cfg.ItemChatSource)

	if drawn := stripEscapes(model.renderSettings()); !strings.Contains(drawn, "change") {
		t.Errorf("a row that steps through values names no key:\n%s", drawn)
	}
	showProviderPage(t, model)
	focusItem(t, model, cfg.ItemModel)
	if drawn := stripEscapes(model.renderSettings()); !strings.Contains(drawn, "type") {
		t.Errorf("a row of text names no key:\n%s", drawn)
	}
}

// A chord chosen on the keys page is written, and the client answers it at once.
func TestChoosingAChordBindsItAtOnce(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeTree))
	focusItem(t, model, cfg.ItemChordPrefix+"filter-tree")

	model.readKey(pressOpen())
	typeInto(model, "ctrl+alt+f")
	model.readKey(pressOpen())

	if !model.settingsForm.Saved {
		t.Fatalf("the screen reports %q", model.settingsForm.Message)
	}
	if !strings.Contains(readConfigFile(t, path), "filter-tree = \"ctrl+alt+f\"") {
		t.Errorf("the chord was not written:\n%s", readConfigFile(t, path))
	}
	// The row shows the chord the client now answers, and no longer the one of the preset.
	item, _ := model.settingsForm.FocusedItem()
	if item.Value != "ctrl+alt+f" || item.Faint {
		t.Errorf("the row reads %+v", item)
	}
	held := model.registry.FindActionChords(cfg.ScopeTree, ActionFilterTree)
	if cfg.DescribeChordChoice(held) != "ctrl+alt+f" {
		t.Errorf("the client answers %q", cfg.DescribeChordChoice(held))
	}
}

// A row of a chord opens empty, so what is typed takes the place of the chord it has. An
// empty row takes back the chord of the preset.
func TestAnEmptyChordRowTakesBackTheChordOfThePreset(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeTree))
	focusItem(t, model, cfg.ItemChordPrefix+"filter-tree")

	model.readKey(pressOpen())
	if held := model.settingsForm.Draft.Text; held != "" {
		t.Fatalf("the row opened holding %q", held)
	}
	typeInto(model, "ctrl+alt+f")
	model.readKey(pressOpen())

	model.readKey(pressOpen())
	model.readKey(pressOpen())
	item, _ := model.settingsForm.FocusedItem()
	if item.Value != "/" || !item.Faint {
		t.Errorf("the row reads %+v", item)
	}
	if len(model.keys.Choices) != 0 {
		t.Errorf("the client still holds %v", model.keys.Choices)
	}
}

// A chord another action of the same scope holds is written and reported, so the reader is
// told which two actions the press reaches.
func TestAChordTwoActionsHoldIsReported(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeTree))
	focusItem(t, model, cfg.ItemChordPrefix+"filter-tree")

	model.readKey(pressOpen())
	typeInto(model, "i")
	model.readKey(pressOpen())

	state := model.settingsForm
	if state.Saved {
		t.Fatal("a chord two actions hold was reported as saved")
	}
	if !strings.Contains(state.Message, "describe-table") ||
		!strings.Contains(state.Message, "filter-tree") {
		t.Errorf("the screen reports %q", state.Message)
	}
}

// A chord no terminal reports is refused, and the row keeps what it had.
func TestAChordNoTerminalReportsIsRefused(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeTree))
	focusItem(t, model, cfg.ItemChordPrefix+"filter-tree")

	model.readKey(pressOpen())
	typeInto(model, "ctrl+i")
	model.readKey(pressOpen())

	if model.settingsForm.Saved || model.settingsForm.Message == "" {
		t.Errorf("the screen reports %q", model.settingsForm.Message)
	}
	item, _ := model.settingsForm.FocusedItem()
	if item.Value != "/" {
		t.Errorf("the row reads %q", item.Value)
	}
}

// The keys of the dialog scope stand together under the card each one belongs to, so a key
// of the chat is not read as a key of every card.
func TestTheKeysOfOneCardStandTogether(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeDialog))

	owners := []string{}
	for _, item := range model.settingsForm.Items {
		if item.Key == cfg.ItemChordPrefix+"stop-ai-reply" ||
			item.Key == cfg.ItemChordPrefix+"chat-to-notebook" {
			if !strings.HasPrefix(item.Detail, aiHelpSection+" ·") {
				t.Errorf("%s reads %q", item.Key, item.Detail)
			}
		}
		if held, _, _ := strings.Cut(item.Detail, " · "); held != "" {
			owners = append(owners, held)
		}
	}
	// A card names its keys once, so the page never returns to a card it has left.
	seen := map[string]bool{}
	last := ""
	for _, owner := range owners {
		if owner == last {
			continue
		}
		if seen[owner] {
			t.Errorf("the page returns to %q: %v", owner, owners)
			break
		}
		seen[owner], last = true, owner
	}
	if !seen[aiHelpSection] {
		t.Errorf("no key belongs to the chat: %v", owners)
	}
}

// A press lands on the row the pointer stands on. The card is placed from the rows it holds,
// so a page of another length has to record the rows it actually drew: an estimate that is
// one out reads every press as the row above.
func TestAPressLandsOnTheRowUnderThePointer(t *testing.T) {
	for _, height := range []int{24, 25, 30, 31, 40} {
		for _, page := range [][]string{
			nil, {cfg.PageProviders}, {cfg.PageAgents}, {cfg.PageAgents, "opencode"},
		} {
			model := buildOfflineModel(t, 110, height)
			openSettings(t, model)
			showSection(t, model, cfg.SectionAi)
			for _, held := range page {
				openPage(t, model, held)
			}
			state := model.settingsForm
			frame := strings.Split(stripEscapes(model.render()), "\n")

			for at, item := range state.Items {
				row := model.layout.settingsRows.top + at - state.Offset
				if row < 0 || row >= len(frame) {
					t.Fatalf("%v of %d rows: row %d is off the screen",
						page, height, row)
				}
				if !strings.Contains(frame[row], item.Label) {
					t.Fatalf("%v of %d rows: row %d reads %q, wanted %q",
						page, height, row, frame[row], item.Label)
				}
				model.pressSettings(tea.Mouse{
					X: model.layout.settingsRows.from + 1, Y: row,
				})
				if state.Item != at {
					t.Fatalf("%v of %d rows: the press on row %d took row %d",
						page, height, at, state.Item)
				}
			}
			for at, section := range state.Sections {
				row := model.layout.settingsSections.top + at
				if !strings.Contains(frame[row], section.Label) {
					t.Fatalf("%d rows: row %d reads %q, wanted the %s section",
						height, row, frame[row], section.Label)
				}
			}
		}
	}
}

// The settings are drawn over the workspace, the way every card is, so the panes keep
// standing around them.
func TestTheSettingsAreDrawnOverTheWorkspace(t *testing.T) {
	model := buildOfflineModel(t, 96, 26)
	openSettings(t, model)

	drawn := stripEscapes(model.render())
	if !strings.Contains(drawn, "settings") {
		t.Fatalf("the settings are not drawn:\n%s", drawn)
	}
	for _, wanted := range []string{"explorer", "query"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the %s pane is hidden behind the settings:\n%s", wanted, drawn)
		}
	}
	// The panes behind hold no key the pointer can mark or press, because the settings
	// own the pointer while they are open. The title bar is drawn over them and keeps its
	// own keys.
	for _, button := range model.layout.buttons {
		if button.row > 0 && button.row < model.layout.settingsSections.top {
			t.Errorf("a key of a pane behind the settings is still pressed: %+v",
				button)
		}
	}
}

// The card fits the frame on a short screen. A card taller than the frame pushes the status
// bar off the screen, so the rows it draws are capped and the foot goes first.
func TestTheSettingsCardFitsAShortScreen(t *testing.T) {
	for _, size := range [][2]int{{40, 10}, {50, 12}, {60, 16}, {110, 40}, {200, 60}} {
		model := buildOfflineModel(t, size[0], size[1])
		workspace := len(strings.Split(stripEscapes(model.render()), "\n"))
		openSettings(t, model)

		for _, section := range []string{cfg.SectionAi, cfg.SectionKeys} {
			showSection(t, model, section)
			card := strings.Split(stripEscapes(model.renderSettings()), "\n")
			if len(card) > size[1]-2 {
				t.Errorf("%v: the card holds %d rows of a body of %d",
					size, len(card), size[1]-2)
			}
			for _, line := range card {
				if held := present.MeasureText(line); held > size[0] {
					t.Errorf("%v: a row of the card is %d cells wide", size, held)
				}
			}
			if held := len(strings.Split(stripEscapes(model.render()), "\n")); held !=
				workspace {
				t.Errorf("%v: the screen holds %d rows, and the workspace holds %d",
					size, held, workspace)
			}
		}
	}
}

// A page of more rows than the card draws scrolls, and the row under the caret is drawn
// wherever it stands in the page.
func TestALongPageScrollsUnderTheCaret(t *testing.T) {
	model := buildOfflineModel(t, 110, 20)
	openSettings(t, model)
	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeGlobal))
	state := model.settingsForm

	if len(state.Items) < 20 {
		t.Fatalf("the page holds %d rows, which is not enough to scroll",
			len(state.Items))
	}
	for at := range state.Items {
		state.Item = at
		frame := strings.Split(stripEscapes(model.render()), "\n")
		block := model.layout.settingsRows
		row := block.top + at - state.Offset
		if row < block.top || row >= block.top+block.count {
			t.Fatalf("row %d of %d is off the card", at, len(state.Items))
		}
		if !strings.Contains(frame[row], state.Items[at].Label) {
			t.Fatalf("row %d reads %q, wanted %q", at, frame[row],
				state.Items[at].Label)
		}
	}
}

// Escape answers on every page of every section, and the screen closes from the top.
func TestEveryPageAnswersEscape(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	state := model.settingsForm
	state.Sources.Profiles = []cfg.Profile{{Name: "shop"}}
	state.readItems()

	for _, page := range [][]string{
		{cfg.SectionAi, cfg.PageProviders, "openai"},
		{cfg.SectionAi, cfg.PageAgents, "claude"},
		{cfg.SectionKeys, string(cfg.ScopeTree)},
		{cfg.SectionMcp, cfg.PageConnections},
		{cfg.SectionNotebooks, cfg.PageDirectories},
	} {
		showSection(t, model, page[0])
		for _, held := range page[1:] {
			openPage(t, model, held)
		}
		for range len(page) - 1 {
			model.readKey(pressBack())
		}
		if len(state.Path) != 0 {
			t.Errorf("%v: Escape stands on %v", page, state.Path)
		}
		model.readKey(pressBack())
		if state.Pane != paneSections {
			t.Errorf("%v: Escape left the keyboard on the rows", page)
		}
	}
	model.readKey(pressBack())
	if model.settingsForm != nil {
		t.Error("the screen is still open")
	}
}

// A press lands on the row under the pointer in every section.
func TestAPressLandsOnTheRowOfEverySection(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	openSettings(t, model)
	state := model.settingsForm
	state.Sources.Profiles = []cfg.Profile{{Name: "shop"}, {Name: "warehouse"}}

	for at := range state.Sections {
		state.Pane = paneSections
		state.stepSection(at - state.Section)
		model.render()
		for row := range state.Items {
			block := model.layout.settingsRows
			model.pressSettings(tea.Mouse{
				X: block.from + 1, Y: block.top + row - state.Offset,
			})
			if state.Item != row {
				t.Errorf("%s: a press on row %d took row %d",
					state.SectionKey(), row, state.Item)
			}
		}
	}
}

// typeValue writes one value into the row under the caret, over what it holds.
func typeValue(t *testing.T, model *Model, text string) {
	t.Helper()
	model.readKey(pressOpen())
	for range 48 {
		model.readKey(tea.Key{Code: tea.KeyBackspace})
	}
	typeInto(model, text)
	model.readKey(pressOpen())
	if !model.settingsForm.Saved {
		t.Fatalf("%q was refused: %q", text, model.settingsForm.Message)
	}
}

// A change in every section is written to the config file, and the file reads back as the
// settings that were made. This is the whole screen end to end: every kind of row, every
// page, one file.
func TestEverySectionWritesAFileThatReadsBack(t *testing.T) {
	model := buildOfflineModel(t, 110, 40)
	path := openSettings(t, model)
	state := model.settingsForm
	state.Sources.Profiles = []cfg.Profile{{Name: "shop"}, {Name: "warehouse"}}
	state.readItems()

	stepped := func(name string) {
		t.Helper()
		if !state.Saved {
			t.Fatalf("%s was refused: %q", name, state.Message)
		}
	}

	showSection(t, model, cfg.SectionAppearance)
	for _, key := range []string{cfg.ItemTheme, cfg.ItemIcons, cfg.ItemKeyHints} {
		focusItem(t, model, key)
		model.readKey(pressRight())
		stepped(key)
	}
	focusItem(t, model, cfg.ItemHideSystem)
	model.readKey(pressOpen())
	stepped(cfg.ItemHideSystem)

	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeTree))
	focusItem(t, model, cfg.ItemChordPrefix+"filter-tree")
	typeValue(t, model, "ctrl+alt+f")
	state.closePage()

	showSection(t, model, cfg.SectionMcp)
	focusItem(t, model, cfg.ItemMcpAccess)
	model.readKey(pressRight())
	stepped(cfg.ItemMcpAccess)
	focusItem(t, model, cfg.ItemMcpRowLimit)
	typeValue(t, model, "250")
	focusItem(t, model, cfg.ItemMcpTimeout)
	typeValue(t, model, "12000")
	openPage(t, model, cfg.PageConnections)
	focusItem(t, model, cfg.ItemProfilePrefix+"shop")
	model.readKey(pressOpen())
	stepped("a connection")
	state.closePage()

	showSection(t, model, cfg.SectionNotebooks)
	openPage(t, model, cfg.PageDirectories)
	focusItem(t, model, cfg.ItemNotebookAdd)
	typeValue(t, model, "~/team/sql")
	state.closePage()

	showSection(t, model, cfg.SectionAi)
	focusItem(t, model, cfg.ItemChatTimeout)
	typeValue(t, model, "45000")
	openPage(t, model, cfg.PageProviders)
	openPage(t, model, string(cfg.ProviderOpenai))
	focusItem(t, model, cfg.ItemAPIKeyEnv)
	typeValue(t, model, "MY_KEY")
	focusItem(t, model, cfg.ItemToolSteps)
	typeValue(t, model, "12")
	focusItem(t, model, cfg.ItemUseSource)
	model.readKey(pressOpen())
	stepped("the provider as the source")
	state.closePage()
	state.closePage()

	openPage(t, model, cfg.PageAgents)
	focusItem(t, model, cfg.ItemAddAgent)
	model.readKey(pressOpen())
	stepped(cfg.ItemAddAgent)
	focusItem(t, model, cfg.ItemAgentCommand)
	typeValue(t, model, "my-acp")
	focusItem(t, model, cfg.ItemAgentArgs)
	typeValue(t, model, "serve --acp")
	focusItem(t, model, cfg.ItemAgentName)
	typeValue(t, model, "mine")
	focusItem(t, model, cfg.ItemUseSource)
	model.readKey(pressOpen())
	stepped("the agent as the source")

	// The file holds every change, and reads back with nothing to report.
	document, err := cfg.DecodeDocument(readConfigFile(t, path))
	if err != nil {
		t.Fatalf("the file does not read: %v", err)
	}
	ai := cfg.ParseAiConfig(document)
	settings := cfg.ParseUISettings(document)
	keys := cfg.ParseKeySettings(document)
	mcpConfig := cfg.ParseMcpConfig(document)
	books := cfg.ParseNotebookSettings(document)
	for _, held := range [][]string{ai.Problems, keys.Problems, settings.Problems} {
		if len(held) != 0 {
			t.Errorf("the file reports %v", held)
		}
	}

	if settings.Theme == "" || settings.IconSet == cfg.IconsPlain ||
		settings.KeyHints == cfg.KeyHintsFull || !settings.HideSystemSchemas {
		t.Errorf("the appearance reads %+v", settings)
	}
	if held := cfg.DescribeChordChoice(
		keys.Choices["tree:filter-tree"]); held != "ctrl+alt+f" {
		t.Errorf("the chord reads %q", held)
	}
	if mcpConfig.Access != cfg.McpReadWrite || mcpConfig.RowLimit != 250 ||
		mcpConfig.Timeout != 12*time.Second ||
		!cfg.ServesMcpProfile(mcpConfig, "shop") {
		t.Errorf("the MCP settings read %+v", mcpConfig)
	}
	if len(books.Paths) != 1 || books.Paths[0] != "~/team/sql" {
		t.Errorf("the notebook directories read %v", books.Paths)
	}
	if ai.StatementTimeout != 45*time.Second || ai.DefaultAgent != "mine" {
		t.Errorf("the chat reads %+v", ai)
	}
	if held := ai.Providers[cfg.ProviderOpenai]; held.APIKeyEnv != "MY_KEY" ||
		held.MaxToolSteps != 12 {
		t.Errorf("the provider reads %+v", held)
	}
	if held := ai.Agents["mine"]; held.Command != "my-acp" ||
		strings.Join(held.Args, " ") != "serve --acp" {
		t.Errorf("the agent reads %+v", held)
	}

	// The client answers what the file holds.
	if model.aiAgent != "mine" || model.settings.Theme != settings.Theme ||
		!cfg.ServesMcpProfile(model.mcp, "shop") {
		t.Error("the client did not take the settings that were written")
	}
	if held := model.registry.FindActionChords(
		cfg.ScopeTree, ActionFilterTree); cfg.DescribeChordChoice(held) != "ctrl+alt+f" {
		t.Errorf("the client answers %q", cfg.DescribeChordChoice(held))
	}
}
