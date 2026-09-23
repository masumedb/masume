package ui

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// The command palette lists actions, key presets, AI providers, and themes.

// The prefix of the id of a palette row that changes the AI provider, and of one that
// changes the key preset.
const (
	aiProviderPrefix = "ai-provider:"
	aiAgentPrefix    = "ai-agent:"
	keyPresetPrefix  = "key-preset:"
	// configProblemsAction is available when configuration problems exist.
	configProblemsAction = "config-problems"
	// settingsAction opens the settings screen.
	settingsAction = "show-settings"
)

// paletteEntry is one row of the palette. Scope and Action name the action, so the row
// shows the chord bound to it.
type paletteEntry struct {
	id string
	// label is the text of a row with no action.
	label  string
	detail string
	scope  cfg.KeyScope
	action ActionID
	// needs is the capability of a row that runs no action. A row with one takes the
	// capability of that action.
	needs Capability
	// when returns false where the state leaves this row out. A row the client cannot
	// run now is left out, and not offered and refused.
	when func(keyScene) bool
	// detailScope and detailAction are for a row reached by the key of another action,
	// such as the one that moves between the panes.
	detailScope  cfg.KeyScope
	detailAction ActionID
}

// paneChordAction names the key that moves between the panes.
const paneChordAction = ActionFocusNextPane

// paletteEntries are the rows the palette offers, in order.
var paletteEntries = []paletteEntry{
	{id: "run-at-cursor", scope: cfg.ScopeGlobal, action: ActionRunAtCursor, when: holdsStatement},
	{id: "run-batch", detail: "one result each",
		scope: cfg.ScopeGlobal, action: ActionRunBatch, when: holdsStatement},
	{id: "explain", scope: cfg.ScopeGlobal, action: ActionExplain, when: holdsStatement},
	{id: "explain-analyze",
		scope: cfg.ScopeGlobal, action: ActionExplainAnalyze, when: holdsStatement},
	{id: "cancel-query", scope: cfg.ScopeGlobal, action: ActionCancelQuery, when: runsQuery},
	{id: "show-history", scope: cfg.ScopeGlobal, action: ActionShowHistory},
	{id: "save-query", detail: "under a name",
		scope: cfg.ScopeGlobal, action: ActionSaveQuery, when: savesQuery},
	{id: "show-saved", scope: cfg.ScopeGlobal, action: ActionShowSaved},
	{id: "show-activity", detail: "load, locks, and other sessions",
		scope: cfg.ScopeGlobal, action: ActionShowActivity},
	{id: "undo-write", detail: "run the saved undo statement",
		scope: cfg.ScopeGlobal, action: ActionUndoWrite, when: undoesWrite},
	{id: "export-csv", scope: cfg.ScopeGlobal, action: ActionExportCSV, when: holdsResult},
	{id: "export-json", scope: cfg.ScopeGlobal, action: ActionExportJSON, when: holdsResult},
	{id: "reopen-tab", scope: cfg.ScopeGlobal, action: ActionReopenTab, when: holdsClosedTab},
	{id: "undo-change", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionUndoChange, when: undoesChange},
	{id: "redo-change", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionRedoChange, when: redoesChange},
	{id: "review-changes", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionReviewChanges, when: stagesChanges},
	{id: "discard-changes", detail: "asks first",
		scope: cfg.ScopeGrid, action: ActionDiscardChanges, when: stagesChanges},
	{id: "begin-transaction",
		scope: cfg.ScopeGlobal, action: ActionBeginTransaction, when: opensTransaction},
	{id: "commit-transaction",
		scope: cfg.ScopeGlobal, action: ActionCommitTransaction, when: holdsTransaction},
	{id: "rollback-transaction", scope: cfg.ScopeGlobal, action: ActionRollbackTransaction,
		when: holdsTransaction},
	{id: "toggle-autocommit", scope: cfg.ScopeGlobal, action: ActionToggleAutocommit},
	{id: "tab-data", label: "View: Data", detail: "result rows",
		when: offersResultView(app.ViewData)},
	{id: "tab-fields", label: "View: Fields",
		detail: "the columns the server returned",
		when:   offersResultView(app.ViewFields)},
	{id: "tab-statistics", label: "View: Statistics",
		detail: "affected rows and execution times",
		when:   offersResultView(app.ViewStatistics)},
	{id: "tab-columns", label: "View: Columns", detail: "table columns",
		when: offersResultView(app.ViewColumns)},
	{id: "tab-indexes", label: "View: Indexes", detail: "table indexes",
		when: offersResultView(app.ViewIndexes)},
	{id: "tab-constraints", label: "View: Constraints", detail: "table constraints",
		when: offersResultView(app.ViewConstraints)},
	{id: "tab-ddl", label: "View: DDL", detail: "the statement that defines the table",
		when: offersResultView(app.ViewDDL)},
	{id: "tab-plan", label: "View: Plan", detail: "query plan",
		when: offersResultView(app.ViewPlan)},
	{id: "reveal-sql", detail: "a table opens as a query",
		scope: cfg.ScopeGlobal, action: ActionRevealSQL, when: revealsStatement},
	{id: "toggle-sidebar", scope: cfg.ScopeGlobal, action: ActionToggleSidebar},
	{id: "toggle-result", detail: "the editor fills the pane",
		scope: cfg.ScopeGlobal, action: ActionToggleResult},
	{id: "focus-sidebar", scope: cfg.ScopeGlobal, action: ActionFocusSidebar,
		detailScope: cfg.ScopeGlobal, detailAction: paneChordAction},
	{id: "focus-editor", scope: cfg.ScopeGlobal, action: ActionFocusEditor, when: showsEditor,
		detailScope: cfg.ScopeGlobal, detailAction: paneChordAction},
	{id: "focus-result", scope: cfg.ScopeGlobal, action: ActionFocusResult,
		detailScope: cfg.ScopeGlobal, detailAction: paneChordAction},
	{id: "new-query-tab", scope: cfg.ScopeGlobal, action: ActionNewQueryTab},
	{id: "new-notebook-tab", detail: "cells that share one connection",
		scope: cfg.ScopeGlobal, action: ActionNewNotebookTab},
	{id: "new-builder-tab", detail: "pick tables, join them, read the SQL",
		scope: cfg.ScopeGlobal, action: ActionNewBuilderTab, needs: NeedsJoinsTables},
	{id: "show-notebooks", detail: "of the project and of the user",
		scope: cfg.ScopeGlobal, action: ActionShowNotebooks},
	{id: "write-notebook-report", detail: "prose, statements and the rows of every cell",
		scope: cfg.ScopeGlobal, action: ActionWriteNotebookReport,
		when: editsNotebook},
	{id: "notebook-run-policy", detail: "transaction and error policy",
		scope: cfg.ScopeGlobal, action: ActionNotebookRunPolicy, when: editsNotebook},
	{id: "run-cell", detail: "in a notebook",
		scope: cfg.ScopeNotebook, action: ActionRunCell, when: editsNotebook},
	{id: "run-from-cell", detail: "in a notebook",
		scope: cfg.ScopeNotebook, action: ActionRunFromCell, when: editsNotebook},
	{id: "run-marked-cells", detail: "in a notebook",
		scope: cfg.ScopeNotebook, action: ActionRunMarkedCells, when: editsNotebook},
	{id: "add-cell-below", detail: "in a notebook",
		scope: cfg.ScopeNotebook, action: ActionAddCellBelow, when: editsNotebook},
	{id: "set-cell-kind", detail: "sql, md, param, or chart",
		scope: cfg.ScopeNotebook, action: ActionSetCellKind, when: editsNotebook},
	{id: "edit-cell-source", detail: "a chart cell opens its form",
		scope: cfg.ScopeNotebook, action: ActionEditCellSource, when: editsNotebook},
	{id: "next-tab", scope: cfg.ScopeGlobal, action: ActionNextTab, when: showsManyTabs},
	{id: "close-tab", detail: "asks if changes are staged",
		scope: cfg.ScopeGlobal, action: ActionCloseTab},
	{id: "name-tab", scope: cfg.ScopeGlobal, action: ActionNameTab, when: namesTab},
	{id: "refresh-objects", detail: "read the catalog again",
		scope: cfg.ScopeGlobal, action: ActionRefreshObjects},
	{id: "copy-csv", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyCSV, when: holdsResult},
	{id: "copy-json", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyJSON, when: holdsResult},
	{id: "copy-markdown", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyMarkdown, when: holdsResult},
	{id: "copy-inserts", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyInserts, when: holdsResult},
	{id: "copy-plan", detail: "in the plan view · raw server output",
		scope: cfg.ScopePlan, action: ActionCopyPlan, when: holdsQueryPlan},
	{id: "open-picker", scope: cfg.ScopeGlobal, action: ActionOpenPicker},
	{id: "close-connection", detail: "close all its tabs",
		scope: cfg.ScopeGlobal, action: ActionCloseConnection},
	{id: "next-page", scope: cfg.ScopeGlobal, action: ActionNextPage, when: fetchesMoreRows},
	{id: "count-rows", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCountRows, when: countsRows},
	{id: "format-sql", detail: "one clause per line",
		scope: cfg.ScopeEditor, action: ActionFormatSQL, when: editsStatement},
	{id: "show-themes", detail: "preview the selected theme",
		scope: cfg.ScopeGlobal, action: ActionShowThemes},
	{id: "reload-themes", label: "Reload the theme files",
		detail: "read the theme files again"},
	{id: settingsAction, label: "Settings", detail: "the client settings, written to config.toml"},
	{id: "show-help", scope: cfg.ScopeGlobal, action: ActionShowHelp},
	{id: "show-ai-chat", detail: "ask about this database, or for a query",
		scope: cfg.ScopeGlobal, action: ActionShowAiChat},
	{id: "ai-explain-query", label: "Ask AI: explain this query",
		detail: "the query in the editor", when: editsStatement},
	{id: "ai-optimize-query", label: "Ask AI: optimize this query",
		detail: "the query in the editor", when: editsStatement},
	{id: "ai-build-notebook", label: "Ask AI: build a notebook",
		detail: "prose and one cell per query"},
	{id: "chat-to-notebook", detail: "one cell per statement the model wrote",
		scope: cfg.ScopeDialog, action: ActionChatToNotebook, when: holdsChatText},
	{id: "ai-fix-error", detail: "the last failed run in the editor",
		scope: cfg.ScopeGlobal, action: ActionAiFixError, when: failedLastRun},
}

// offersResultView returns the test that offers a view while the result holds it. The pane
// draws the views of a result, so a client that has run nothing offers none of them.
func offersResultView(view app.ResultView) func(keyScene) bool {
	return func(scene keyScene) bool {
		if scene.tab.Results.State().Kind == app.QueryIdle {
			return false
		}
		return slices.Contains(scene.tab.Views(scene.connection.Session), view)
	}
}

// holdsQueryPlan is true where the result on show is a plan, which is the one the plan is
// copied from.
func holdsQueryPlan(scene keyScene) bool {
	return scene.tab.ViewData.Kind == app.DataPlan
}

// runsQuery is true while a statement runs, which is the only time one can be cancelled.
func runsQuery(scene keyScene) bool {
	return scene.tab.Results.State().Kind == app.QueryRunning
}

// stagesChanges is true where the grid holds a change that is not written yet.
func stagesChanges(scene keyScene) bool {
	return core.CountChanges(scene.tab.Pending) > 0
}

// editsNotebook is true on a notebook tab.
func editsNotebook(scene keyScene) bool {
	return scene.tab.Kind == app.TabNotebook
}

// holdsStatement is true where the pane above the result holds something to run.
func holdsStatement(scene keyScene) bool {
	tab := scene.tab
	if tab.Kind == app.TabNotebook {
		return tab.Notebook != nil && len(tab.Notebook.Cells) > 0
	}
	if tab.Kind == app.TabBuilder {
		return tab.BuildsQuery()
	}
	return strings.TrimSpace(tab.Editor.Text) != ""
}

// editsStatement is true where the editor itself holds text, which is what the rows that
// read it need.
func editsStatement(scene keyScene) bool {
	return scene.tab.Kind == app.TabQuery &&
		strings.TrimSpace(scene.tab.Editor.Text) != ""
}

// holdsResult is true where the statement that ran returned something to read.
func holdsResult(scene keyScene) bool {
	active := scene.tab.Results.Active()
	return active != nil && active.State.Kind == app.QuerySucceeded
}

// countsRows is true where the result is one the server can be asked the row count of.
func countsRows(scene keyScene) bool { return scene.tab.Results.CanCountRows() }

// fetchesMoreRows is true where the result holds rows the client has not read yet.
func fetchesMoreRows(scene keyScene) bool {
	return scene.tab.Results.Active() != nil && scene.tab.Results.CanFetchMore()
}

// revealsStatement is true where the result came from a statement the editor can hold.
func revealsStatement(scene keyScene) bool {
	return scene.tab.EffectiveSQL(scene.connection.Session) != ""
}

// holdsClosedTab is true where a tab was closed and can be opened again.
func holdsClosedTab(scene keyScene) bool {
	return scene.connection.HasClosedTab()
}

// undoesWrite is true where the last write left a statement that takes it back.
func undoesWrite(scene keyScene) bool {
	held := scene.connection.Undo
	return held != nil && held.Undo.IsHeld()
}

// undoesChange is true where a staged change can be stepped back, and redoesChange where one
// that was stepped back can be restored.
func undoesChange(scene keyScene) bool { return scene.tab.CanUndoChange() }

func redoesChange(scene keyScene) bool { return scene.tab.CanRedoChange() }

// holdsTransaction is true while a transaction is open on the server, and opensTransaction
// while none is.
func holdsTransaction(scene keyScene) bool {
	return scene.connection.Session.ReadTransactionState() == db.TransactionOpen
}

func opensTransaction(scene keyScene) bool { return !holdsTransaction(scene) }

// showsEditor is true where the pane above the result is drawn, which is the pane the
// keyboard is given to.
func showsEditor(scene keyScene) bool { return scene.tab.EditorVisible() }

// savesQuery is true where the pane above the result holds something to save under a name.
func savesQuery(scene keyScene) bool {
	return scene.tab.EditorVisible() && holdsStatement(scene)
}

// providerLabels name each provider the way a reader writes it, not the way the config
// file keys it.
var providerLabels = map[cfg.AiProviderID]string{
	cfg.ProviderAnthropic:        "Anthropic",
	cfg.ProviderOpenai:           "OpenAI",
	cfg.ProviderOpenaiCompatible: "Local or OpenAI compatible",
}

// readEntryDetail returns the detail of a row, which can be the chord of another action. A
// row of the palette spells the chord, as a row of the help does.
func (model *Model) readEntryDetail(entry paletteEntry) string {
	if entry.detailAction != "" {
		return model.registry.FormatFirstActionChordName(
			entry.detailScope, entry.detailAction)
	}
	return entry.detail
}

// readEntryLabel returns the text of a row: its own, or the label of its action in sentence
// case.
func readEntryLabel(entry paletteEntry) string {
	if entry.action == "" {
		return entry.label
	}
	action, _ := FindAction(entry.scope, entry.action)
	first, size := utf8.DecodeRuneInString(action.Label)
	return string(unicode.ToUpper(first)) + action.Label[size:]
}

// findEntryCapability returns the capability a row needs: its own, or the one of its action.
func findEntryCapability(entry paletteEntry) Capability {
	if entry.needs != "" {
		return entry.needs
	}
	if entry.action == "" {
		return ""
	}
	return FindActionCapability(entry.scope, entry.action)
}

// buildPaletteActions returns every row the command palette offers. A row the engine cannot
// do is left out, and not shown and refused.
func (model *Model) buildPaletteActions(connection *app.Connection) []app.PaletteAction {
	capabilities := connection.Session.Capabilities()
	actions := []app.PaletteAction{}

	scene := keyScene{
		model: model, connection: connection, tab: connection.Active(),
		chat: connection.Chat,
	}
	for _, entry := range paletteEntries {
		if !AnswersFor(capabilities, findEntryCapability(entry)) {
			continue
		}
		if entry.when != nil && !entry.when(scene) {
			continue
		}
		if !model.offersAi() && (aiPaletteRows[entry.id] || IsAiAction(entry.action)) {
			continue
		}
		chord := ""
		if entry.action != "" {
			chord = model.registry.FormatFirstActionChordName(entry.scope, entry.action)
		}
		actions = append(actions, app.PaletteAction{
			ID: entry.id, Label: readEntryLabel(entry),
			Detail: model.readEntryDetail(entry), Chord: chord,
		})
	}

	// One row per AI provider, with the model the config file set for it, and one row per
	// agent of the config file.
	if model.offersAi() {
		for _, id := range cfg.AiProviderIDs {
			detail := model.ai.Providers[id].Model
			if detail == "" {
				detail = "not configured"
			}
			actions = append(actions, app.PaletteAction{
				ID:     aiProviderPrefix + string(id),
				Label:  "AI provider: " + providerLabels[id],
				Detail: detail,
			})
		}
		for _, name := range sortedAgentNames(model.ai.Agents) {
			actions = append(actions, app.PaletteAction{
				ID: aiAgentPrefix + name, Label: "AI agent: " + name,
				Detail: describeAgentCommand(model.ai.Agents[name]),
			})
		}
	}
	// One row per key preset, so a new preset is offered without a second list. Nothing
	// is chosen while there is one preset.
	if presets := ListKeyPresets(); len(presets) > 1 {
		for _, preset := range presets {
			actions = append(actions, app.PaletteAction{
				ID: keyPresetPrefix + string(preset.ID), Label: "Keys: " + preset.Title,
				Detail: preset.Describe,
			})
		}
	}
	if len(model.problems) > 0 {
		actions = append(actions, app.PaletteAction{
			ID: configProblemsAction, Label: "Config problems",
			Detail: present.FormatCount(int64(len(model.problems))) +
				" · config and theme file problems",
		})
	}
	return actions
}

// paletteViews name the view each `tab-` row of the palette moves to.
var paletteViews = []app.ResultView{
	app.ViewTree,
	app.ViewData, app.ViewFields, app.ViewStatistics, app.ViewColumns,
	app.ViewIndexes, app.ViewConstraints, app.ViewDDL, app.ViewPlan,
}

// runPaletteAction returns a row of the command palette. A row that writes into the result
// pane opens it again first, so nothing is written where it cannot be read.
func (model *Model) runPaletteAction(
	connection *app.Connection, id string,
) (tea.Model, tea.Cmd) {
	connection.CloseEveryOverlay()
	tab := connection.Active()

	for _, view := range paletteViews {
		if id != "tab-"+string(view) {
			continue
		}
		return model.selectResultView(connection, tab, view)
	}
	if after, ok := strings.CutPrefix(id, keyPresetPrefix); ok {
		return model.switchKeyPreset(connection, after)
	}
	if after, ok := strings.CutPrefix(id, aiProviderPrefix); ok {
		return model.switchAiProvider(connection, after)
	}
	if after, ok := strings.CutPrefix(id, aiAgentPrefix); ok {
		return model.switchAiAgent(connection, after)
	}

	switch id {
	case "copy-plan":
		if tab.ViewData.Kind != app.DataPlan {
			return model, nil
		}
		connection.Show("plan copied")
		return model, model.keepOnClipboard(tab.ViewData.Plan.Raw)
	case "reload-themes":
		return model.reloadThemeFiles(connection)
	case settingsAction:
		model.settingsCameFrom = ScreenWorking
		return model.showSettings()
	case "ai-explain-query":
		return model.askAi(connection, connection.Active(),
			"Explain what the query in the editor does, in plain terms.")
	case "ai-build-notebook":
		return model.askAiForNotebook(connection)
	case "ai-optimize-query":
		return model.askAi(connection, connection.Active(),
			"Suggest how to make the query in the editor faster or clearer, and explain why.")
	case configProblemsAction:
		connection.Open(app.Overlay{
			Kind: app.OverlayMessage, Title: " config problems ",
			Body: strings.Join(model.problems, "\n"),
		})
		return model, nil
	}

	action, known := FindActionID(id)
	if !known {
		connection.ShowError("unknown action: \"" + id + "\"")
		return model, nil
	}
	scope := cfg.ScopeGlobal
	if _, isGlobal := FindAction(cfg.ScopeGlobal, action); !isGlobal {
		for _, held := range []cfg.KeyScope{cfg.ScopeNotebook, cfg.ScopeGrid} {
			if _, found := FindAction(held, action); found {
				scope = held
				break
			}
		}
	}
	if AnswersInResult(scope, action) {
		connection.ResultVisible = true
	}
	return model.runAction(connection, tab, Match{Action: action, Scope: scope})
}

// focusPane moves the keyboard to one pane, which the palette does by name.
func (model *Model) focusPane(
	connection *app.Connection, tab *app.Tab, pane app.Pane,
) (tea.Model, tea.Cmd) {
	switch pane {
	case app.PaneSidebar:
		if !connection.SidebarVisible {
			return model, nil
		}
	case app.PaneEditor:
		if !tab.EditorVisible() {
			return model, nil
		}
	case app.PaneResult:
		if !connection.ResultVisible {
			return model, nil
		}
	}
	tab.Focus = pane
	return model, nil
}

// selectResultView moves to one view by name, which the palette does.
func (model *Model) selectResultView(
	connection *app.Connection, tab *app.Tab, view app.ResultView,
) (tea.Model, tea.Cmd) {
	for at, offered := range tab.Views(connection.Session) {
		if offered == view {
			return model.selectViewAt(connection, tab, at)
		}
	}
	return model, nil
}

// switchKeyPreset applies a preset without changing the config file.
func (model *Model) switchKeyPreset(
	connection *app.Connection, written string,
) (tea.Model, tea.Cmd) {
	id, known := cfg.FindPresetID(written)
	if !known {
		return model, nil
	}
	for _, problem := range model.registry.ApplyKeySettings(
		FindKeyPreset(id), cfg.ChordChoices{}, model.offersAi()) {
		model.problems = append(model.problems, "keys: "+problem)
	}
	connection.Show("keys: " + string(id) +
		" · write preset = \"" + string(id) + "\" under [keys] to keep it")
	return model, nil
}

// switchAiProvider changes the provider the chat would send to.
func (model *Model) switchAiProvider(
	connection *app.Connection, written string,
) (tea.Model, tea.Cmd) {
	for _, id := range cfg.AiProviderIDs {
		if string(id) != written {
			continue
		}
		model.aiProvider = id
		model.aiAgent = ""
		connection.Show("ai provider set to " + written)
		return model, nil
	}
	return model, nil
}

// reloadThemeFiles reloads theme files and reapplies the configured theme.
func (model *Model) reloadThemeFiles(connection *app.Connection) (tea.Model, tea.Cmd) {
	path := cfg.ResolveConfigPath()
	documents, problems := cfg.ReadThemeDocuments(cfg.ResolveThemesPath(path))

	registry := NewThemeRegistry()
	styles := NewStyles(registry)
	found := append([]string{}, registry.ListBuiltInProblems()...)
	found = append(found, problems...)
	found = append(found, registry.RegisterDocuments(documents)...)

	name := model.settings.Theme
	if name == "" {
		name = model.styles.Theme.Name
	}
	reported, applied := styles.ApplyThemeByName(name)
	found = append(found, reported...)
	if !applied {
		connection.ShowError("unknown theme: \"" + name + "\"")
		return model, nil
	}
	found = append(found, styles.ApplyColorOverrides(model.settings.Colors)...)
	model.styles = styles
	model.problems = found

	if len(found) == 0 {
		connection.Show(present.FormatCountOf(
			int64(len(documents)), "theme file", "theme files") + " read")
		return model, nil
	}
	if len(found) == 1 {
		connection.ShowError(found[0])
		return model, nil
	}
	connection.ShowError(present.FormatCount(int64(len(found))) +
		" problems · see Config problems in the palette")
	return model, nil
}

// themeCursor returns the row of the theme picker the applied theme stands on.
func (model *Model) themeCursor() int {
	for at, choice := range model.styles.registry.ListThemeChoices() {
		if choice.Name == model.styles.Theme.Name {
			return at
		}
	}
	return 0
}
