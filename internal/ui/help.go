package ui

import "github.com/masumedb/masume/internal/cfg"

// Help groups actions by task and reads key bindings from the registry.

// HelpEntry is one help row with an action and its keys.
type HelpEntry struct {
	Scope   cfg.KeyScope
	Actions []ActionID
	// Keys is the displayed text for keys outside the registry.
	Keys string
	// Text is empty on the first row of one action. That row draws the label of the action.
	Text string
}

// readHelpText returns the text of a help row.
func readHelpText(entry HelpEntry) string {
	if entry.Text != "" || len(entry.Actions) != 1 {
		return entry.Text
	}
	action, _ := FindAction(entry.Scope, entry.Actions[0])
	return action.Label
}

// HelpSection is one titled group of help rows.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

// HelpSections are the groups the help draws, in order.
var HelpSections = []HelpSection{
	{
		Title: "tabs",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNewQueryTab}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionActivateTab}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousTab, ActionNextTab}, Text: "previous or next tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNameTab}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCloseTab}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionReopenTab}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRevealSQL}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleSidebar}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleResult}},
		},
	},
	{
		Title: "connections",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionOpenPicker}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousConnection, ActionNextConnection}, Text: "switch connection"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCloseConnection}},
		},
	},
	{
		Title: "panes",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionFocusNextPane}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionFocusPreviousPane}},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "move in the tree"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionFoldRow, ActionUnfoldRow}, Text: "fold and unfold a row"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenNode}},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenInNewTab}},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionDescribeTable}},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionObjectMenu}},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionFilterTree}},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleFavourite}},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleSystemSchemas}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRefreshObjects}},
		},
	},
	{
		Title: "grid",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "go to another row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorLeft, ActionCursorRight}, Text: "go to another column"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenRow}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionViewCell}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionCopyValue}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFollowForeignKey}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCopyMenu}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenMenu}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCountRows}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionGoToColumn}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionSearchColumns}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFreezeColumns}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionToggleMasking}},
			{Keys: "drag", Text: "select text under the pointer"},
		},
	},
	{
		Title: "sort and filter",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionSortColumn}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionAddSortColumn}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFilterByCell}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFilterByValues}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionExcludeCell}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFilterWhere}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionPopFilter}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionClearRewrites}},
		},
	},
	{
		Title: "query",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunAtCursor}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunBatch}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousStatement, ActionNextStatement}, Text: "previous or next statement in a batch"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExplain}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExplainAnalyze}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCancelQuery}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNextPage}},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionOpenNode}},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionUnfoldRow, ActionFoldRow}, Text: "open a field, or fold it and step out"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionCopyValue}},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionCopyPath}},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionToggleRawPlan}},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionCopyPlan}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowPalette}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowHistory}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowSaved}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExportCSV}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExportJSON}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowHelp}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSelectView}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousView, ActionNextView}, Text: "go to the previous or the next view"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSaveQuery}},
		},
	},
	{
		Title: "notebooks",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNewNotebookTab}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowNotebooks}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSaveQuery}, Text: "save this notebook to its file"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNotebookRunPolicy}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionCursorUp, ActionCursorDown}, Text: "move between cells"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionEditCellSource}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionLeaveCell}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionRunCell}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionRunFromCell}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunBatch}, Text: "run every cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionMarkCell}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionRunMarkedCells}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionAddCellBelow, ActionAddCellAbove}, Text: "add a cell below or above"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionSetCellKind}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionNameCell}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionDeleteCell}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionUndoCellChange, ActionRedoCellChange}, Text: "undo or redo a cell change"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionMoveCellUp, ActionMoveCellDown}, Text: "move the focused cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionCopyCell, ActionCutCell}, Text: "copy or cut the focused cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionPasteCell}},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionToggleCellOutput, ActionToggleEveryOutput}, Text: "fold one cell, or every cell"},
		},
	},
	{
		Title: "query builder",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNewBuilderTab}},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionAddBuilderTable}},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionPreviousTable, ActionNextTable}, Text: "move between tables"},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionCursorUp, ActionCursorDown}, Text: "move between columns and filters"},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionPickColumn}},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionEditBuilderRow}},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionAddBuilderFilter}},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionDropBuilderRow}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunAtCursor}, Text: "run the built statement"},
			{Scope: cfg.ScopeBuilder, Actions: []ActionID{ActionSendToEditor}},
		},
	},
	{
		Title: "editing rows",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionEditCell}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionInsertRow}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionToggleDelete}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionDuplicateRow}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionUndoChange}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionRedoChange}},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionReviewChanges}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionApplyChanges}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionDiscardChanges}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionUndoWrite}},
		},
	},
	{
		Title: "transaction",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionBeginTransaction}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCommitTransaction}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRollbackTransaction}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleAutocommit}},
		},
	},
	{
		Title: "writing a statement",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretLineStart}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretLineEnd}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretTextStart, ActionCaretTextEnd}, Text: "go to the top or the bottom of the statement"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretWordLeft, ActionCaretWordRight}, Text: "go one word back or forward"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretPageUp, ActionCaretPageDown}, Text: "go a page up or down"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionDeleteWordBack}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionDeleteWordForward}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionUndoEdit}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionRedoEdit}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionPasteText}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionFormatSQL}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCommentLines}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionIndentLines, ActionOutdentLines}, Text: "indent or outdent the lines"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAcceptCompletion}},
		},
	},
	{
		Title: "searching the statement",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionFindInStatement}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionReplaceInStatement}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionNextMatch, ActionPreviousMatch}, Text: "go to the next or the previous match"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionReplaceMatch}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionToggleWholeWord}},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionShowCompletion}},
		},
	},
	{
		Title: "selecting text",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionSelectAll}},
			{Keys: "Shift+Left", Text: "extend the selection one cell left, in the editor"},
			{Keys: "Shift+Right", Text: "extend it one cell right"},
			{Keys: "Shift+Up", Text: "extend it one line up"},
			{Keys: "Shift+Down", Text: "extend it one line down"},
			{Keys: "Shift+Home", Text: "extend it to the start of the line"},
			{Keys: "Shift+End", Text: "extend it to the end of the line"},
			{Keys: "Ctrl+Shift+Left", Text: "extend it one word back"},
			{Keys: "Ctrl+Shift+Right", Text: "extend it one word forward"},
			{Keys: "double click", Text: "select the word under the pointer"},
			{Keys: "triple click", Text: "select the whole line"},
			{Keys: "Ctrl+C", Text: "copy the selection, or quit when nothing is selected"},
		},
	},
	{
		Title: "lists",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "move through a dialog list"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionChooseRow}},
		},
	},
	{
		Title: "dialogs and forms",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionClose}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAnswerYes}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAnswerNo}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionNewConnection}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionEditConnection}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionDeleteConnection}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionListModels}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionTestConnection}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSaveForm}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSaveCell}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetNull}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetEmpty}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetDefault}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPrettifyJSON}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionRunWithValues}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionWriteExport}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionOpenInNewTab}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionListSecondary}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStopSession}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionToggleValue}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionKeepOnlyValue}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionKeepAllValues}},
		},
	},
	{
		Title: "cards and forms",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionClose}, Text: "close the card on show"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionChooseRow}, Text: "take the row under the cursor"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPreviousField, ActionNextField}, Text: "the row before or after, in a form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPreviousValue, ActionNextValue}, Text: "the value before or after, in a row of choices"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionApplyStep}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStepBack}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionOpenDirectory, ActionLeaveDirectory}, Text: "open a directory or go up, in a file picker"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPreviousRow, ActionNextRow}, Text: "the row before or after, in the card of one row"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionScrollLeft, ActionScrollRight}, Text: "pan a diagram sideways"},
		},
	},
	{
		Title: "ai chat",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowAiChat}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSendToAi}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSendQuestion}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionWriteNewline}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionInsertAiSQL}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionChatToNotebook}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAskAiAgain}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionCopyAiReply}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStopAiReply}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionNewAiChat}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionShowAiChats}},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPreviousTurn, ActionNextTurn}, Text: "go to the previous or the next turn"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionScrollBack, ActionScrollForward}, Text: "scroll a page back or forward"},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionAiCheckPlan}},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionAiFixError}},
			{Keys: "", Text: "more AI prompts in the command palette"},
		},
	},
}
