package ui

import (
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
)

// keyScene is the state a key spec reads.
type keyScene struct {
	model      *Model
	connection *app.Connection
	tab        *app.Tab
	overlay    app.Overlay
	// cellKind is the kind of the focused cell of a notebook.
	cellKind notebook.CellKind
	// chat is the conversation of the panel, where the keys are the ones of the chat.
	chat *app.Chat
	// text is a readout the caller measured, such as the count of the statements.
	text string
	// hasFault is true while the scanner marked the statement in the editor.
	hasFault bool
}

// keySpec is one part of a key line: a key of the registry, a key a widget reads itself, or
// a readout with no key.
type keySpec struct {
	scope  cfg.KeyScope
	action ActionID
	// second is the other half of a pair, such as the step on beside the step back.
	second    ActionID
	separator string
	icon      cfg.IconKind
	// chord holds the key of a widget the registry does not hold.
	chord string
	// aside is true for a widget key that only the full mode shows.
	aside bool
	// firstChord is true for a key drawn with its first chord alone.
	firstChord bool
	label      string
	// labelOf returns the label where it follows the state.
	labelOf func(keyScene) string
	// textOf returns a readout with no key.
	textOf func(keyScene) string
	// when returns false where the state leaves this part out.
	when func(keyScene) bool
	// silent is true for a key the card reads and does not show.
	silent bool
}

// keyOf is a key of the registry, drawn with every chord bound to it.
func keyOf(scope cfg.KeyScope, action ActionID, label string) keySpec {
	return keySpec{scope: scope, action: action, label: label}
}

// firstChordOf is a key of the registry, drawn with the first chord alone.
func firstChordOf(scope cfg.KeyScope, action ActionID, label string) keySpec {
	return keySpec{scope: scope, action: action, label: label, firstChord: true}
}

// iconKeyOf is a key of the registry with the glyph of what it acts on before it.
func iconKeyOf(
	scope cfg.KeyScope, action ActionID, icon cfg.IconKind, label string,
) keySpec {
	return keySpec{scope: scope, action: action, icon: icon, label: label}
}

// pairOf is one key for two actions, such as the step back and the step on.
func pairOf(
	scope cfg.KeyScope, previous, next ActionID, label, separator string,
) keySpec {
	return keySpec{
		scope: scope, action: previous, second: next, label: label, separator: separator,
	}
}

// takesKey is a key the card reads and does not show, such as a second chord for the same
// answer.
func takesKey(scope cfg.KeyScope, action ActionID) keySpec {
	return keySpec{scope: scope, action: action, silent: true}
}

// readout is a word of the card with no key.
func readout(text string) keySpec {
	return keySpec{textOf: func(keyScene) string { return text }}
}

// readoutOf is a word of the card that follows the state.
func readoutOf(text func(keyScene) string) keySpec {
	return keySpec{textOf: text}
}

// withLabel returns the spec with a label that follows the state.
func (spec keySpec) withLabel(label func(keyScene) string) keySpec {
	spec.labelOf = label
	return spec
}

// onlyWhen returns the spec with the state it is drawn in.
func (spec keySpec) onlyWhen(when func(keyScene) bool) keySpec {
	spec.when = when
	return spec
}

// addTo puts the spec on the line, in the form its kind asks for.
func (spec keySpec) addTo(line *KeyLine, scene keyScene) {
	if spec.silent || (spec.when != nil && !spec.when(scene)) {
		return
	}
	label := spec.label
	if spec.labelOf != nil {
		label = spec.labelOf(scene)
	}
	switch {
	case spec.textOf != nil:
		line.addText(spec.textOf(scene))
	case spec.chord != "" && spec.aside:
		line.addAsideKey(spec.chord, label)
	case spec.chord != "":
		line.addAnswerKey(spec.chord, label)
	case spec.second != "":
		line.bindPair(spec.scope, spec.action, spec.second, label, spec.separator)
	case spec.icon != "":
		line.bindIcon(spec.scope, spec.action, spec.icon, label)
	case spec.firstChord:
		line.bindFirstChord(spec.scope, spec.action, label)
	default:
		line.bind(spec.scope, spec.action, label)
	}
}

// keepServerHints drops a hint the server cannot run. The status bar leaves such a key out
// rather than drawing it and refusing the press.
func keepServerHints(hints []Hint, capabilities core.Capabilities) []Hint {
	kept := make([]Hint, 0, len(hints))
	for _, hint := range hints {
		if AnswersFor(capabilities, FindActionCapability(hint.Scope, hint.Action)) {
			kept = append(kept, hint)
		}
	}
	return kept
}

// buildKeyLineOf returns the line these specs draw in this scene.
func (model *Model) buildKeyLineOf(specs []keySpec, scene keyScene) *KeyLine {
	scene.model = model
	line := model.buildKeyLine()
	for _, spec := range specs {
		spec.addTo(line, scene)
	}
	return line
}

// buildCardKeys returns the key line of a card. A card of no keys returns an empty line.
func (model *Model) buildCardKeys(kind app.OverlayKind, scene keyScene) *KeyLine {
	return model.buildKeyLineOf(listCardSpecs(kind, scene), scene)
}

// listCardSpecs returns the specs of a card. The import card and the dump card read a set of
// their own at each stage.
func listCardSpecs(kind app.OverlayKind, scene keyScene) []keySpec {
	if kind == app.OverlayImport || kind == app.OverlayDump {
		return keyGroups[describeOverlayGroup(scene.overlay)]
	}
	return cardKeySpecs[kind]
}

// The keys of every card, in the order the card draws them. A card that follows its state
// holds every key of every state, and each one carries the state it is drawn in.
var cardKeySpecs = map[app.OverlayKind][]keySpec{
	app.OverlayHelp: {
		readoutOf(describeHelpMatches).onlyWhen(filtersHelp),
		readout("type to search").onlyWhen(notFilters(filtersHelp)),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayBuilderTables: {
		readout("type to filter"),
		keyOf(cfg.ScopeList, ActionChooseRow, "add"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayBuilderJoin: {
		pairOf(cfg.ScopeDialog, ActionPreviousField, ActionNextField, "row", ""),
		pairOf(cfg.ScopeDialog, ActionPreviousValue, ActionNextValue, "change", ""),
		keyOf(cfg.ScopeDialog, ActionApplyStep, "join"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayBuilderField: {
		pairOf(cfg.ScopeDialog, ActionPreviousField, ActionNextField, "row", ""),
		pairOf(cfg.ScopeDialog, ActionPreviousValue, ActionNextValue, "change", ""),
		keyOf(cfg.ScopeDialog, ActionApplyStep, "apply"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayPalette: {
		readout("type to filter"),
		keyOf(cfg.ScopeList, ActionChooseRow, "run"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayHistory: {
		keyOf(cfg.ScopeList, ActionChooseRow, "load in this tab"),
		keyOf(cfg.ScopeDialog, ActionOpenInNewTab, "load in a new tab"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlaySaved: {
		takesKey(cfg.ScopeDialog, ActionOpenInNewTab),
		keyOf(cfg.ScopeList, ActionChooseRow, "load"),
		keyOf(cfg.ScopeDialog, ActionListSecondary, "delete"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayNotebooks: {
		keyOf(cfg.ScopeList, ActionChooseRow, "open"),
		keyOf(cfg.ScopeDialog, ActionOpenInNewTab, "open in a new tab"),
		keyOf(cfg.ScopeDialog, ActionNewConnection, "new"),
		keyOf(cfg.ScopeDialog, ActionEditConnection, "rename"),
		keyOf(cfg.ScopeDialog, ActionDeleteConnection, "delete"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayActionMenu: {
		keyOf(cfg.ScopeList, ActionChooseRow, "run"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayCopyMenu: {
		keyOf(cfg.ScopeList, ActionChooseRow, "copy"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayObjectMenu: {
		keyOf(cfg.ScopeList, ActionChooseRow, "run the action"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayConfirm: {
		takesKey(cfg.ScopeList, ActionChooseRow),
		keyOf(cfg.ScopeDialog, ActionAnswerYes, "run"),
		keyOf(cfg.ScopeDialog, ActionAnswerNo, "cancel"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayWritePlan: {
		keyOf(cfg.ScopeDialog, ActionAnswerYes, "run"),
		takesKey(cfg.ScopeDialog, ActionAnswerNo),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayChoice: {
		keyOf(cfg.ScopeDialog, ActionClose, "stay here"),
	},
	app.OverlayMessage: {
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayDiagram: {
		pairOf(cfg.ScopeList, ActionCursorUp, ActionCursorDown, "scroll", ""),
		pairOf(cfg.ScopeDialog, ActionScrollLeft, ActionScrollRight, "scroll", ""),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayCell: {
		readoutOf(readOverlayNotice),
		takesKey(cfg.ScopeList, ActionCursorUp),
		readoutOf(describeCellType),
		readoutOf(countCellLines),
		keyOf(cfg.ScopeDialog, ActionCopyValue, "copy"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayParameters: {
		readoutOf(readOverlayNotice),
		keyOf(cfg.ScopeDialog, ActionRunWithValues, "run"),
		keyOf(cfg.ScopeDialog, ActionPrettifyJSON, "format JSON"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayCellEdit: {
		readoutOf(readOverlayNotice),
		pairOf(cfg.ScopeList, ActionCursorUp, ActionCursorDown, "pick", "").
			onlyWhen(picksCellValue),
		keyOf(cfg.ScopeList, ActionChooseRow, "stage").onlyWhen(picksCellValue),
		keyOf(cfg.ScopeDialog, ActionSaveCell, "stage").
			onlyWhen(notFilters(picksCellValue)),
		keyOf(cfg.ScopeDialog, ActionPrettifyJSON, "format JSON").onlyWhen(editsJSONCell),
		keyOf(cfg.ScopeDialog, ActionSetNull, "NULL"),
		keyOf(cfg.ScopeDialog, ActionSetEmpty, "empty"),
		keyOf(cfg.ScopeDialog, ActionSetDefault, "default"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayRowDetail: {
		pairOf(cfg.ScopeDialog, ActionPreviousRow, ActionNextRow, "another row", ""),
		pairOf(cfg.ScopeList, ActionCursorUp, ActionCursorDown, "scroll", ""),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayChanges: {
		keyOf(cfg.ScopeDialog, ActionApplyChanges, "apply"),
		keyOf(cfg.ScopeDialog, ActionDiscardChanges, "discard"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayValueFilter: {
		keyOf(cfg.ScopeDialog, ActionToggleValue, "pick"),
		keyOf(cfg.ScopeDialog, ActionKeepOnlyValue, "only this"),
		keyOf(cfg.ScopeDialog, ActionKeepAllValues, "all"),
		keyOf(cfg.ScopeList, ActionChooseRow, "apply"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayThemePicker: {
		readout("preview the selected theme"),
		keyOf(cfg.ScopeList, ActionChooseRow, "select"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayActivity: {
		keyOf(cfg.ScopeList, ActionChooseRow, "open statement"),
		keyOf(cfg.ScopeDialog, ActionStopSession, "stop statement"),
		keyOf(cfg.ScopeDialog, ActionListSecondary, "end session"),
		keyOf(cfg.ScopeDialog, ActionFoldRow, "fold").onlyWhen(showsLocks),
		keyOf(cfg.ScopeDialog, ActionUnfoldRow, "open").onlyWhen(showsLocks),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
	app.OverlayExport: {
		pairOf(cfg.ScopeDialog, ActionPreviousField, ActionNextField, "field", ""),
		pairOf(cfg.ScopeDialog, ActionPreviousValue, ActionNextValue, "change", ""),
		keyOf(cfg.ScopeDialog, ActionWriteExport, "export"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayChart: {
		pairOf(cfg.ScopeDialog, ActionPreviousField, ActionNextField, "field", ""),
		pairOf(cfg.ScopeDialog, ActionPreviousValue, ActionNextValue, "change", ""),
		keyOf(cfg.ScopeDialog, ActionSaveForm, "apply"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayPrompt: {
		takesKey(cfg.ScopeDialog, ActionReplaceInStatement),
		keyOf(cfg.ScopeList, ActionChooseRow, "save"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	},
	app.OverlayAiChat: {
		keyOf(cfg.ScopeDialog, ActionAnswerYes, "run").onlyWhen(asksToRun),
		keyOf(cfg.ScopeDialog, ActionAnswerNo, "do not run").onlyWhen(asksToRun),
		firstChordOf(cfg.ScopeDialog, ActionSendQuestion, "ask").
			onlyWhen(notFilters(asksToRun)),
		firstChordOf(cfg.ScopeDialog, ActionWriteNewline, "newline").
			onlyWhen(notFilters(asksToRun)),
		keyOf(cfg.ScopeDialog, ActionStopAiReply, "stop").onlyWhen(writesReply),
		keyOf(cfg.ScopeDialog, ActionAskAiAgain, "ask again").onlyWhen(asksAgain),
		keyOf(cfg.ScopeDialog, ActionCopyAiReply, "copy reply").onlyWhen(holdsChatText),
		// The panel keeps one row of keys. The rest of them are in the help, under the
		// group of the chat, and the status bar carries them as well.
		keyOf(cfg.ScopeDialog, ActionInsertAiSQL, "to editor").
			onlyWhen(holdsChatQuery),
		keyOf(cfg.ScopeDialog, ActionNewAiChat, "new").onlyWhen(notFilters(asksToRun)),
		keyOf(cfg.ScopeDialog, ActionShowAiChats, "chats").onlyWhen(notFilters(asksToRun)),
		keyOf(cfg.ScopeDialog, ActionClose, "close").onlyWhen(notFilters(asksToRun)),
	},
	app.OverlayAiChats: {
		keyOf(cfg.ScopeList, ActionChooseRow, "open"),
		keyOf(cfg.ScopeDialog, ActionListSecondary, "delete"),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	},
}

// The names of the key groups of the import card. Each stage reads a set of its own.
const (
	importPickGroup   = "import-pick"
	importFormGroup   = "import"
	importReviewGroup = "import-review"
)

// The names of the key groups of the dump card: the file picker of a restore, and the form.
const (
	dumpPickGroup = "dump-pick"
	dumpFormGroup = "dump"
)

// settingsGroup is the name of the key group of the settings screen.
const settingsGroup = "settings"

// The keys of the three stages of an import: the file picker, the form, and the review.
var (
	importPickKeySpecs = []keySpec{
		pairOf(cfg.ScopeList, ActionCursorUp, ActionCursorDown, "file", ""),
		keyOf(cfg.ScopeDialog, ActionOpenDirectory, "open the directory"),
		keyOf(cfg.ScopeDialog, ActionLeaveDirectory, "go up"),
		keyOf(cfg.ScopeList, ActionChooseRow, "choose"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	}
	importFormKeySpecs = []keySpec{
		pairOf(cfg.ScopeDialog, ActionPreviousField, ActionNextField, "field", ""),
		pairOf(cfg.ScopeDialog, ActionPreviousValue, ActionNextValue, "change", ""),
		keyOf(cfg.ScopeDialog, ActionApplyStep, "").withLabel(describeImportStepOf),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	}
	importReviewKeySpecs = []keySpec{
		keyOf(cfg.ScopeDialog, ActionApplyStep, "").withLabel(describeImportRunOf),
		keyOf(cfg.ScopeDialog, ActionStepBack, "back to the form"),
	}
)

// The keys of the form of a dump and of a restore. The picker of a restore reads the keys of
// the picker of an import.
var dumpFormKeySpecs = []keySpec{
	pairOf(cfg.ScopeDialog, ActionPreviousField, ActionNextField, "field", "").
		onlyWhen(dumpsTables),
	pairOf(cfg.ScopeDialog, ActionPreviousValue, ActionNextValue, "change", "").
		onlyWhen(dumpsTables),
	keyOf(cfg.ScopeDialog, ActionApplyStep, "").withLabel(describeDumpStepOf),
	keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
}

// The keys of a query builder tab: the ones on the border of the pane, and the ones an
// empty builder names in its body.
var (
	builderBorderKeySpecs = []keySpec{
		keyOf(cfg.ScopeBuilder, ActionPickColumn, "pick"),
		keyOf(cfg.ScopeBuilder, ActionEditBuilderRow, "edit"),
		keyOf(cfg.ScopeBuilder, ActionAddBuilderTable, "table or join"),
		keyOf(cfg.ScopeBuilder, ActionAddBuilderFilter, "where"),
		keyOf(cfg.ScopeBuilder, ActionDropBuilderRow, "drop"),
		keyOf(cfg.ScopeBuilder, ActionSendToEditor, "to editor"),
	}
	builderEmptyKeySpecs = []keySpec{
		keyOf(cfg.ScopeBuilder, ActionAddBuilderTable, "add a table"),
	}
)

// The keys of the two screens that have no connection: the picker of the profiles, and the
// field the password is typed into.
var (
	pickerKeySpecs = []keySpec{
		takesKey(cfg.ScopeDialog, ActionClose),
		keyOf(cfg.ScopeList, ActionChooseRow, "or double click connects"),
		keyOf(cfg.ScopeDialog, ActionNewConnection, "new").onlyWhen(listsConnections),
		keyOf(cfg.ScopeDialog, ActionEditConnection, "edit").onlyWhen(listsConnections),
		keyOf(cfg.ScopeDialog, ActionDeleteConnection, "delete").onlyWhen(listsConnections),
		keyOf(cfg.ScopeDialog, ActionFilterConnections, "filter").
			onlyWhen(listsConnections),
		keyOf(cfg.ScopeDialog, ActionClose, "stops the filter").
			onlyWhen(filtersConnections),
	}
	passwordKeySpecs = []keySpec{
		keyOf(cfg.ScopeList, ActionChooseRow, "").withLabel(describePasswordUse),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
		keyOf(cfg.ScopeDialog, ActionUseKeyring, "keyring").onlyWhen(offersKeyring),
	}
	settingsKeySpecs = []keySpec{
		pairOf(cfg.ScopeList, ActionCursorUp, ActionCursorDown, "move", ""),
		keyOf(cfg.ScopeDialog, ActionNextValue, "the rows").onlyWhen(showsSections),
		pairOf(cfg.ScopeDialog, ActionPreviousValue, ActionNextValue, "change", "").
			onlyWhen(stepsSettingValue),
		keyOf(cfg.ScopeDialog, ActionPreviousValue, "back").
			onlyWhen(leavesSettingPage),
		keyOf(cfg.ScopeDialog, ActionPreviousValue, "the sections").
			onlyWhen(leavesSettingRows),
		keyOf(cfg.ScopeList, ActionChooseRow, "").
			withLabel(describeSettingsChoose).onlyWhen(opensSettingRow),
		takesKey(cfg.ScopeDialog, ActionNextField),
		takesKey(cfg.ScopeDialog, ActionPreviousField),
		takesKey(cfg.ScopeDialog, ActionToggleValue),
		takesKey(cfg.ScopeList, ActionCursorFirstRow),
		takesKey(cfg.ScopeList, ActionCursorLastRow),
		keyOf(cfg.ScopeDialog, ActionListModels, "models").onlyWhen(picksAgentModel),
		keyOf(cfg.ScopeDialog, ActionClose, "").withLabel(describeSettingsClose),
	}
	connectionFormKeySpecs = []keySpec{
		pairOf(cfg.ScopeDialog, ActionPreviousField, ActionNextField, "field", ""),
		takesKey(cfg.ScopeDialog, ActionPreviousValue),
		takesKey(cfg.ScopeDialog, ActionNextValue),
		takesKey(cfg.ScopeList, ActionChooseRow),
		keyOf(cfg.ScopeList, ActionChooseRow, "pick the file").onlyWhen(picksFormFile),
		keyOf(cfg.ScopeDialog, ActionTestConnection, "test"),
		keyOf(cfg.ScopeDialog, ActionSaveForm, "save"),
		keyOf(cfg.ScopeDialog, ActionClose, "cancel"),
	}
)

// The keys of the strips and the borders of a pane.
var (
	tabRowKeySpecs = []keySpec{
		keyOf(cfg.ScopeGlobal, ActionNewQueryTab, "new"),
		pairOf(cfg.ScopeGlobal, ActionPreviousTab, ActionNextTab, "tab", " ").
			onlyWhen(showsManyTabs),
		keyOf(cfg.ScopeGlobal, ActionActivateTab, "go").onlyWhen(showsManyTabs),
		keyOf(cfg.ScopeGlobal, ActionNameTab, "name").onlyWhen(namesTab),
		keyOf(cfg.ScopeGlobal, ActionCloseTab, "close").onlyWhen(showsManyTabs),
	}
	statementStripKeySpecs = []keySpec{
		readoutOf(readSceneText),
		pairOf(cfg.ScopeGlobal, ActionPreviousStatement, ActionNextStatement,
			"prev/next", " "),
	}
	viewStripKeySpecs = []keySpec{
		pairOf(cfg.ScopeGlobal, ActionPreviousView, ActionNextView, "prev/next", " "),
	}
	planStripKeySpecs = []keySpec{
		firstChordOf(cfg.ScopePlan, ActionToggleRawPlan, "").withLabel(describePlanForm),
		firstChordOf(cfg.ScopePlan, ActionCopyPlan, "copy"),
		firstChordOf(cfg.ScopePlan, ActionAiCheckPlan, "ask ai"),
	}
	planCostKeySpecs = []keySpec{
		firstChordOf(cfg.ScopeGlobal, ActionExplainAnalyze, "for actual times"),
	}
	runningKeySpecs = []keySpec{
		firstChordOf(cfg.ScopeGlobal, ActionCancelQuery, "stop").onlyWhen(stopsRunning),
	}
	editorAiKeySpecs = []keySpec{
		iconKeyOf(cfg.ScopeGlobal, ActionAiFixError, cfg.IconAi, "explain the failure").
			onlyWhen(failedLastRun),
		iconKeyOf(cfg.ScopeGlobal, ActionAiFixError, cfg.IconAi, "diagnose this").
			onlyWhen(showsFault),
		iconKeyOf(cfg.ScopeGlobal, ActionShowAiChat, cfg.IconAi, "ask for a query").
			onlyWhen(editsNothing),
		iconKeyOf(cfg.ScopeGlobal, ActionSendToAi, cfg.IconAi, "ask about this").
			onlyWhen(asksAboutStatement),
	}
)

// notFilters returns the opposite of a state.
func notFilters(when func(keyScene) bool) func(keyScene) bool {
	return func(scene keyScene) bool { return !when(scene) }
}

func filtersHelp(scene keyScene) bool {
	return scene.model.readOverlayTerm(scene.overlay) != ""
}

func describeHelpMatches(scene keyScene) string {
	found := scene.model.findHelpRows(scene.model.readOverlayTerm(scene.overlay))
	return present.FormatCount(int64(len(found))) + " matching entries"
}

func readOverlayNotice(scene keyScene) string {
	return scene.overlay.Notice
}

func describeCellType(scene keyScene) string {
	named := scene.overlay.Cell.Column.DataType
	if present.IsJSONType(named) {
		named += " · formatted"
	}
	return named
}

func countCellLines(scene keyScene) string {
	written := present.FormatForViewer(
		scene.overlay.Cell.Value, scene.overlay.Cell.Column.DataType)
	return present.FormatCountOf(
		int64(len(strings.Split(written, "\n"))), "line", "lines")
}

// readSceneText returns the readout the caller measured.
func readSceneText(scene keyScene) string {
	return scene.text
}

func picksCellValue(scene keyScene) bool {
	return len(scene.overlay.Cell.Choices) > 0
}

func editsJSONCell(scene keyScene) bool {
	return !picksCellValue(scene) && present.IsJSONType(scene.overlay.Cell.Column.DataType)
}

func showsLocks(scene keyScene) bool {
	return len(scene.overlay.Server.Locks) > 0
}

func describeImportStepOf(scene keyScene) string {
	return describeImportStep(scene.overlay)
}

func describeImportRunOf(scene keyScene) string {
	return describeImportRun(scene.overlay.Import)
}

func describeDumpStepOf(scene keyScene) string {
	return describeDumpStep(scene.overlay)
}

// dumpsTables is true for the card of a dump, which has rows a restore has not.
func dumpsTables(scene keyScene) bool {
	return scene.overlay.Dump.Mode == app.DumpWrite
}

func asksToRun(scene keyScene) bool {
	chat := readSceneChat(scene)
	return chat != nil && chat.Pending != nil
}

func writesReply(scene keyScene) bool {
	chat := readSceneChat(scene)
	return chat != nil && chat.IsStreaming()
}

// readSceneChat returns the conversation of the scene, from the panel or the connection.
func readSceneChat(scene keyScene) *app.Chat {
	if scene.chat != nil {
		return scene.chat
	}
	if scene.connection != nil {
		return scene.connection.Chat
	}
	return nil
}

// describePasswordUse returns what the typed password does: it tests the connection form,
// or it opens the connection.
func describePasswordUse(scene keyScene) string {
	if scene.model.picker.testsForm {
		return "test"
	}
	return "connect"
}

// listsConnections is true while the filter field has no focus, so the letter keys run the
// actions of the card.
func listsConnections(scene keyScene) bool {
	return !scene.model.picker.filtersList()
}

func filtersConnections(scene keyScene) bool {
	return scene.model.picker.filtersList()
}

// asksAgain is true where the chat has a question to ask again, which is one that failed or
// one that was stopped.
func asksAgain(scene keyScene) bool {
	if scene.chat == nil || asksToRun(scene) || scene.chat.IsStreaming() {
		return false
	}
	_, held := scene.chat.FindLastQuestion()
	return held
}

// holdsChatText is true where the chat has a reply to copy.
func holdsChatText(scene keyScene) bool {
	if scene.chat == nil || asksToRun(scene) {
		return false
	}
	_, held := scene.chat.FindLastReply()
	return held
}

// holdsChatQuery is true where the chat has written a statement, which is what the key that
// fills the editor needs.
func holdsChatQuery(scene keyScene) bool {
	if scene.chat == nil || asksToRun(scene) {
		return false
	}
	_, held := scene.chat.FindLastQuery()
	return held
}

// picksAgentModel is true while the chat of the settings screen sends to an agent, which is
// the only source with a model list to read.
func picksAgentModel(scene keyScene) bool {
	held := scene.model.settingsForm
	return held != nil && held.EditedAgent() != ""
}

// showsSections is true while the sections have the keyboard.
func showsSections(scene keyScene) bool {
	held := scene.model.settingsForm
	return held != nil && held.Pane == paneSections
}

// stepsSettingValue is true where the row under the caret steps through a list of values.
func stepsSettingValue(scene keyScene) bool {
	held := scene.model.settingsForm
	if held == nil || held.Pane != paneItems {
		return false
	}
	item, found := held.FocusedItem()
	return found && item.Kind == cfg.SettingChoice
}

// leavesSettingRows is true where the arrow that steps a value gives the keyboard back to
// the sections.
func leavesSettingRows(scene keyScene) bool {
	held := scene.model.settingsForm
	return held != nil && held.Pane == paneItems && len(held.Path) == 0 &&
		!stepsSettingValue(scene)
}

// opensSettingRow is true where Enter has something to do: a section to open, or a row to
// change, to type into or to run.
func opensSettingRow(scene keyScene) bool {
	held := scene.model.settingsForm
	if held == nil || held.Pane == paneSections {
		return held != nil
	}
	return !stepsSettingValue(scene)
}

// describeSettingsChoose returns what Enter does to the row under the caret.
func describeSettingsChoose(scene keyScene) string {
	held := scene.model.settingsForm
	if held == nil || held.Pane == paneSections {
		return "open the section"
	}
	item, found := held.FocusedItem()
	if !found {
		return "open"
	}
	switch item.Kind {
	case cfg.SettingToggle, cfg.SettingChoice:
		return "change"
	case cfg.SettingAction:
		return "run"
	case cfg.SettingGroup:
		return "open"
	}
	return "type"
}

// describeSettingsClose returns what Escape does from the page on show.
func describeSettingsClose(scene keyScene) string {
	held := scene.model.settingsForm
	if held != nil && (len(held.Path) > 0 || held.Pane == paneItems) {
		return "back"
	}
	return "close"
}

// leavesSettingPage is true where the arrow that steps a value back closes a page instead.
func leavesSettingPage(scene keyScene) bool {
	held := scene.model.settingsForm
	return held != nil && len(held.Path) > 0 && !stepsSettingValue(scene)
}

func offersKeyring(scene keyScene) bool {
	return scene.model.picker.offersKeyring()
}

func showsManyTabs(scene keyScene) bool {
	return scene.connection != nil && len(scene.connection.Tabs) > 1
}

func namesTab(scene keyScene) bool {
	return scene.tab != nil &&
		(scene.tab.Kind == app.TabQuery || scene.tab.Kind == app.TabNotebook)
}

func describePlanForm(scene keyScene) string {
	if scene.tab.RawPlan {
		return "the plan tree"
	}
	return "the raw plan"
}

func stopsRunning(scene keyScene) bool {
	return AnswersFor(scene.connection.Session.Capabilities(), NeedsCancelsRunning)
}

func failedLastRun(scene keyScene) bool {
	return findLastRunError(scene.tab) != ""
}

func showsFault(scene keyScene) bool {
	return !failedLastRun(scene) && scene.hasFault
}

func editsNothing(scene keyScene) bool {
	return !failedLastRun(scene) && !showsFault(scene) &&
		strings.TrimSpace(scene.tab.Editor.Text) == ""
}

func asksAboutStatement(scene keyScene) bool {
	return !failedLastRun(scene) && !showsFault(scene) &&
		strings.TrimSpace(scene.tab.Editor.Text) != ""
}

// The keys of a notebook, which the status bar draws while the cell list or one cell holds
// the keyboard.
var (
	notebookListHintSpecs = []keySpec{
		pairOf(cfg.ScopeNotebook, ActionCursorUp, ActionCursorDown, "move", ""),
		firstChordOf(cfg.ScopeNotebook, ActionEditCellSource, "").withLabel(describeCellEdit),
		firstChordOf(cfg.ScopeNotebook, ActionRunCell, "run"),
		firstChordOf(cfg.ScopeNotebook, ActionRunFromCell, "run below"),
		firstChordOf(cfg.ScopeGlobal, ActionRunBatch, "run all"),
		firstChordOf(cfg.ScopeNotebook, ActionAddCellBelow, "add a cell"),
		firstChordOf(cfg.ScopeNotebook, ActionSetCellKind, "kind"),
		firstChordOf(cfg.ScopeNotebook, ActionToggleCellOutput, "fold"),
		firstChordOf(cfg.ScopeGlobal, ActionSaveQuery, "save"),
	}
	cellEditorHintSpecs = []keySpec{
		firstChordOf(cfg.ScopeEditor, ActionLeaveCell, "back to the cells"),
		firstChordOf(cfg.ScopeGlobal, ActionRunAtCursor, "run this cell").onlyWhen(editsSQLCell),
		firstChordOf(cfg.ScopeGlobal, ActionRunBatch, "run all cells"),
		firstChordOf(cfg.ScopeEditor, ActionFindInStatement, "find or replace").
			onlyWhen(editsSQLCell),
		firstChordOf(cfg.ScopeDialog, ActionAcceptCompletion, "complete").onlyWhen(editsSQLCell),
		firstChordOf(cfg.ScopeGlobal, ActionSaveQuery, "save"),
		firstChordOf(cfg.ScopeGlobal, ActionToggleResult, "full height"),
	}
)

func describeCellEdit(scene keyScene) string {
	if scene.cellKind == notebook.CellChart {
		return "chart form"
	}
	return "edit"
}

func editsSQLCell(scene keyScene) bool {
	return scene.cellKind == notebook.CellSQL
}

// buildNotebookHints returns the keys of the status bar while the cell list of a notebook
// holds the keyboard.
func (model *Model) buildNotebookHints(context HintContext) []Hint {
	return keepServerHints(model.buildKeyLineOf(notebookListHintSpecs, keyScene{
		cellKind: context.CellKind,
	}).buildHints(), context.Capabilities)
}

// buildCellEditorHints returns the keys of the status bar while one cell of a notebook holds
// the caret.
// buildBuilderHints returns the bar of a builder tab: the keys of the diagram, and the run.
func (model *Model) buildBuilderHints(capabilities core.Capabilities) []Hint {
	keys := hintList{}
	keys.add(model.buildHint(
		capabilities, cfg.ScopeBuilder, ActionAddBuilderTable, "table or join"))
	keys.add(model.buildHint(capabilities, cfg.ScopeBuilder, ActionPickColumn, "pick"))
	keys.add(model.buildHint(
		capabilities, cfg.ScopeBuilder, ActionEditBuilderRow, "aggregate and sort"))
	keys.add(model.buildHint(capabilities, cfg.ScopeBuilder, ActionAddBuilderFilter, "where"))
	keys.add(model.buildHint(capabilities, cfg.ScopeBuilder, ActionDropBuilderRow, "drop"))
	keys.add(model.buildHint(capabilities, cfg.ScopeGlobal, ActionRunAtCursor, "run"))
	keys.add(model.buildHint(capabilities, cfg.ScopeBuilder, ActionSendToEditor, "to editor"))
	return keys.build()
}

func (model *Model) buildCellEditorHints(context HintContext) []Hint {
	return keepServerHints(model.buildKeyLineOf(cellEditorHintSpecs, keyScene{
		cellKind: context.CellKind,
	}).buildHints(), context.Capabilities)
}

// keyGroups is the specs of every card and every screen, by the name the key matching and
// the conflict check use. A name that is a card is the kind of the card.
var keyGroups = func() map[string][]keySpec {
	groups := map[string][]keySpec{
		"picker":          pickerKeySpecs,
		"form":            connectionFormKeySpecs,
		settingsGroup:     settingsKeySpecs,
		"password":        passwordKeySpecs,
		importPickGroup:   importPickKeySpecs,
		importFormGroup:   importFormKeySpecs,
		importReviewGroup: importReviewKeySpecs,
		dumpPickGroup:     importPickKeySpecs,
		dumpFormGroup:     dumpFormKeySpecs,
	}
	for kind, specs := range cardKeySpecs {
		groups[string(kind)] = specs
	}
	return groups
}()

// dialogActionsByName is the actions each card and screen reads, for the key matching and
// the conflict check. Every card also reads the keys of a list, which reach it only while it
// draws rows: the matching searches the list scope for such a card alone.
var dialogActionsByName = func() map[string][]ActionID {
	listActions := collectScopeActions(cfg.ScopeList)
	actions := map[string][]ActionID{}
	for name, specs := range keyGroups {
		held := slices.Clone(listActions)
		for _, spec := range specs {
			for _, action := range []ActionID{spec.action, spec.second} {
				if action == "" || slices.Contains(held, action) {
					continue
				}
				held = append(held, action)
			}
		}
		actions[name] = held
	}
	return actions
}()

// FindDialogActions returns the actions the card or the screen of this name reads.
func FindDialogActions(name string) []ActionID {
	return dialogActionsByName[name]
}

// ListDialogNames returns every card and screen name, sorted.
func ListDialogNames() []string {
	names := make([]string, 0, len(dialogActionsByName))
	for name := range dialogActionsByName {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// describeOverlayGroup returns the name of the key group of this card. The import card and
// the dump card read a set of their own at each stage, so each stage is a group.
func describeOverlayGroup(overlay app.Overlay) string {
	if overlay.Kind == app.OverlayDump {
		if overlay.Dump.Stage == app.DumpPick {
			return dumpPickGroup
		}
		return dumpFormGroup
	}
	if overlay.Kind != app.OverlayImport {
		return string(overlay.Kind)
	}
	switch overlay.Import.Stage {
	case app.ImportPick:
		return importPickGroup
	case app.ImportReview:
		return importReviewGroup
	}
	return importFormGroup
}
