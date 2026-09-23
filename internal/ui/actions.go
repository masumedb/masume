package ui

import (
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

// ActionID names one action the app returns for.
type ActionID string

// Capability names what a server must answer for before an action is offered.
type Capability string

// The capabilities an action can need.
const (
	NeedsNothing        Capability = ""
	NeedsPlansStatement Capability = "plansStatement"
	NeedsMeasuresPlan   Capability = "measuresPlan"
	NeedsServerSessions Capability = "hasServerSessions"
	NeedsCancelsRunning Capability = "cancelsRunningQuery"
	NeedsTransactions   Capability = "hasTransactions"
	NeedsSortsRead      Capability = "sortsRead"
	NeedsTruncatesTable Capability = "truncatesTable"
	NeedsWritesDDL      Capability = "writesDdl"
	NeedsPlansWrites    Capability = "plansWrites"
	// NeedsJoinsTables is the capability of a server that joins tables in one statement.
	NeedsJoinsTables Capability = "joinsTables"
)

// AnswersFor is true if the server has the capability the action needs. An action that needs
// nothing is always true.
func AnswersFor(capabilities core.Capabilities, needs Capability) bool {
	switch needs {
	case NeedsNothing:
		return true
	case NeedsPlansStatement:
		return capabilities.PlansStatement
	case NeedsMeasuresPlan:
		return capabilities.MeasuresPlan
	case NeedsServerSessions:
		return capabilities.HasServerSessions
	case NeedsCancelsRunning:
		return capabilities.CancelsRunningQuery
	case NeedsTransactions:
		return capabilities.HasTransactions
	case NeedsSortsRead:
		return capabilities.SortsRead
	case NeedsTruncatesTable:
		return capabilities.TruncatesTable
	case NeedsWritesDDL:
		return capabilities.WritesDDL
	case NeedsJoinsTables:
		return capabilities.JoinsTables
	case NeedsPlansWrites:
		return capabilities.PlansWrites
	}
	// A capability no arm answers for is one the catalog names and this switch does not,
	// so the action stays out of reach rather than being offered on every server.
	return false
}

// ActionDefinition says what an action may do, apart from the chord that runs it. The
// registry holds the chord, so a rebound key keeps the same rules.
type ActionDefinition struct {
	ID    ActionID
	Scope cfg.KeyScope
	// Label is the lowercase text of the action in the help and the palette.
	Label string
	// A server without this capability leaves the chord unbound and the hint hidden.
	Needs Capability
	// True for an action that works while a query runs, such as a cancel.
	WhileRunning bool
	// True for an action that works while a field or the editor holds the caret.
	WhileTyping bool
	// True for an action on the statement in the editor. It works only while the editor
	// holds the caret.
	EditorOnly bool
	// True for an action whose result goes into the result pane. A hidden result is shown
	// again before one of these runs.
	AnswersInResult bool
	// True for a primary action: the action a pane or a card is there for, the action that
	// answers it, or the action no other key runs. The main mode of the key hints shows
	// these. The full mode shows every key hint.
	MainHint bool
}

// The ids of every action, so a typo is a build error rather than a key that silently does
// nothing.
const (
	ActionRunBatch            ActionID = "run-batch"
	ActionNewQueryTab         ActionID = "new-query-tab"
	ActionToggleSidebar       ActionID = "toggle-sidebar"
	ActionToggleResult        ActionID = "toggle-result"
	ActionRevealSQL           ActionID = "reveal-sql"
	ActionNameTab             ActionID = "name-tab"
	ActionCloseTab            ActionID = "close-tab"
	ActionReopenTab           ActionID = "reopen-tab"
	ActionPreviousTab         ActionID = "previous-tab"
	ActionNextTab             ActionID = "next-tab"
	ActionPreviousConnection  ActionID = "previous-connection"
	ActionNextConnection      ActionID = "next-connection"
	ActionActivateTab         ActionID = "activate-tab"
	ActionRunAtCursor         ActionID = "run-at-cursor"
	ActionExplain             ActionID = "explain"
	ActionExplainAnalyze      ActionID = "explain-analyze"
	ActionShowHistory         ActionID = "show-history"
	ActionShowSaved           ActionID = "show-saved"
	ActionCancelQuery         ActionID = "cancel-query"
	ActionShowPalette         ActionID = "show-palette"
	ActionShowAiChat          ActionID = "show-ai-chat"
	ActionAiFixError          ActionID = "ai-fix-error"
	ActionSendToAi            ActionID = "send-to-ai"
	ActionNextPage            ActionID = "next-page"
	ActionExportCSV           ActionID = "export-csv"
	ActionExportJSON          ActionID = "export-json"
	ActionBeginTransaction    ActionID = "begin-transaction"
	ActionCommitTransaction   ActionID = "commit-transaction"
	ActionRollbackTransaction ActionID = "rollback-transaction"
	ActionToggleAutocommit    ActionID = "toggle-autocommit"
	ActionOpenPicker          ActionID = "open-picker"
	ActionCloseConnection     ActionID = "close-connection"
	ActionShowHelp            ActionID = "show-help"
	ActionFocusNextPane       ActionID = "focus-next-pane"
	ActionFocusPreviousPane   ActionID = "focus-previous-pane"
	ActionPreviousStatement   ActionID = "previous-statement"
	ActionNextStatement       ActionID = "next-statement"
	ActionRefreshObjects      ActionID = "refresh-objects"
	ActionSelectView          ActionID = "select-view"
	ActionPreviousView        ActionID = "previous-view"
	ActionNextView            ActionID = "next-view"
	ActionSaveQuery           ActionID = "save-query"
	ActionFocusSidebar        ActionID = "focus-sidebar"
	ActionFocusEditor         ActionID = "focus-editor"
	ActionFocusResult         ActionID = "focus-result"
	ActionShowActivity        ActionID = "show-activity"
	ActionUndoWrite           ActionID = "undo-write"
	ActionShowThemes          ActionID = "show-themes"

	ActionNewNotebookTab      ActionID = "new-notebook-tab"
	ActionShowNotebooks       ActionID = "show-notebooks"
	ActionNotebookRunPolicy   ActionID = "notebook-run-policy"
	ActionWriteNotebookReport ActionID = "write-notebook-report"

	// The statement being written. A move takes the selection with it while Shift is held,
	// so one action returns a chord and its shifted twin.
	ActionCaretLeft          ActionID = "caret-left"
	ActionCaretRight         ActionID = "caret-right"
	ActionCaretUp            ActionID = "caret-up"
	ActionCaretDown          ActionID = "caret-down"
	ActionCaretWordLeft      ActionID = "caret-word-left"
	ActionCaretWordRight     ActionID = "caret-word-right"
	ActionCaretLineStart     ActionID = "caret-line-start"
	ActionCaretLineEnd       ActionID = "caret-line-end"
	ActionCaretTextStart     ActionID = "caret-text-start"
	ActionCaretTextEnd       ActionID = "caret-text-end"
	ActionCaretPageUp        ActionID = "caret-page-up"
	ActionCaretPageDown      ActionID = "caret-page-down"
	ActionSelectAll          ActionID = "select-all"
	ActionDeleteBack         ActionID = "delete-back"
	ActionDeleteForward      ActionID = "delete-forward"
	ActionDeleteWordBack     ActionID = "delete-word-back"
	ActionDeleteWordForward  ActionID = "delete-word-forward"
	ActionOpenLine           ActionID = "open-line"
	ActionUndoEdit           ActionID = "undo-edit"
	ActionRedoEdit           ActionID = "redo-edit"
	ActionPasteText          ActionID = "paste-text"
	ActionFormatSQL          ActionID = "format-sql"
	ActionCommentLines       ActionID = "comment-lines"
	ActionIndentLines        ActionID = "indent-lines"
	ActionOutdentLines       ActionID = "outdent-lines"
	ActionFindInStatement    ActionID = "find-in-statement"
	ActionReplaceInStatement ActionID = "replace-in-statement"
	ActionNextMatch          ActionID = "next-match"
	ActionPreviousMatch      ActionID = "previous-match"
	ActionReplaceMatch       ActionID = "replace-match"
	ActionNextProblem        ActionID = "next-problem"
	ActionAcceptCompletion   ActionID = "accept-completion"
	ActionShowCompletion     ActionID = "show-completion"
	ActionToggleWholeWord    ActionID = "toggle-whole-word"
	ActionLeaveCell          ActionID = "leave-cell"

	ActionCursorUp         ActionID = "cursor-up"
	ActionCursorDown       ActionID = "cursor-down"
	ActionCursorPageUp     ActionID = "cursor-page-up"
	ActionCursorPageDown   ActionID = "cursor-page-down"
	ActionCursorFirstRow   ActionID = "cursor-first-row"
	ActionCursorLastRow    ActionID = "cursor-last-row"
	ActionCursorLeft       ActionID = "cursor-left"
	ActionCursorRight      ActionID = "cursor-right"
	ActionSortColumn       ActionID = "sort-column"
	ActionAddSortColumn    ActionID = "add-sort-column"
	ActionOpenRow          ActionID = "open-row"
	ActionViewCell         ActionID = "view-cell"
	ActionEditCell         ActionID = "edit-cell"
	ActionToggleDelete     ActionID = "toggle-delete"
	ActionDuplicateRow     ActionID = "duplicate-row"
	ActionReviewChanges    ActionID = "review-changes"
	ActionUndoChange       ActionID = "undo-change"
	ActionRedoChange       ActionID = "redo-change"
	ActionCountRows        ActionID = "count-rows"
	ActionFollowForeignKey ActionID = "follow-foreign-key"
	ActionInsertRow        ActionID = "insert-row"
	ActionCopyMenu         ActionID = "copy-menu"
	ActionCopyCSV          ActionID = "copy-csv"
	ActionCopyJSON         ActionID = "copy-json"
	ActionCopyMarkdown     ActionID = "copy-markdown"
	ActionCopyInserts      ActionID = "copy-inserts"
	ActionOpenMenu         ActionID = "open-menu"
	ActionFilterByCell     ActionID = "filter-by-cell"
	ActionFilterByValues   ActionID = "filter-by-values"
	ActionExcludeCell      ActionID = "exclude-cell"
	ActionClearRewrites    ActionID = "clear-rewrites"
	ActionPopFilter        ActionID = "pop-filter"
	ActionFreezeColumns    ActionID = "freeze-columns"
	ActionToggleMasking    ActionID = "toggle-masking"
	ActionGoToColumn       ActionID = "go-to-column"
	ActionSearchColumns    ActionID = "search-columns"
	ActionFilterWhere      ActionID = "filter-where"

	ActionToggleRawPlan ActionID = "toggle-raw-plan"
	ActionCopyPlan      ActionID = "copy-plan"
	ActionAiCheckPlan   ActionID = "ai-check-plan"

	ActionCopyPath ActionID = "copy-path"

	ActionFoldRow             ActionID = "fold-row"
	ActionUnfoldRow           ActionID = "unfold-row"
	ActionOpenNode            ActionID = "open-node"
	ActionOpenInNewTab        ActionID = "open-in-new-tab"
	ActionDescribeTable       ActionID = "describe-table"
	ActionObjectMenu          ActionID = "object-menu"
	ActionFilterTree          ActionID = "filter-tree"
	ActionToggleFavourite     ActionID = "toggle-favourite"
	ActionToggleSystemSchemas ActionID = "toggle-system-schemas"

	// The query builder.
	ActionAddBuilderTable  ActionID = "add-table"
	ActionAddBuilderFilter ActionID = "add-filter"
	ActionPickColumn       ActionID = "pick-column"
	ActionEditBuilderRow   ActionID = "edit-row"
	ActionDropBuilderRow   ActionID = "drop-row"
	ActionPreviousTable    ActionID = "previous-table"
	ActionNextTable        ActionID = "next-table"
	ActionSendToEditor     ActionID = "send-to-editor"
	ActionNewBuilderTab    ActionID = "new-builder-tab"

	ActionChooseRow ActionID = "choose-row"

	// The filter over the list of the connection picker.
	ActionFilterConnections ActionID = "filter-connections"

	// The cell list of a notebook.
	ActionEditCellSource    ActionID = "edit-cell-source"
	ActionRunCell           ActionID = "run-cell"
	ActionRunFromCell       ActionID = "run-from-cell"
	ActionRunMarkedCells    ActionID = "run-marked-cells"
	ActionAddCellBelow      ActionID = "add-cell-below"
	ActionAddCellAbove      ActionID = "add-cell-above"
	ActionSetCellKind       ActionID = "set-cell-kind"
	ActionDeleteCell        ActionID = "delete-cell"
	ActionUndoCellChange    ActionID = "undo-cell-change"
	ActionRedoCellChange    ActionID = "redo-cell-change"
	ActionMoveCellUp        ActionID = "move-cell-up"
	ActionMoveCellDown      ActionID = "move-cell-down"
	ActionCopyCell          ActionID = "copy-cell"
	ActionCutCell           ActionID = "cut-cell"
	ActionPasteCell         ActionID = "paste-cell"
	ActionToggleCellOutput  ActionID = "toggle-cell-output"
	ActionToggleEveryOutput ActionID = "toggle-every-output"
	ActionMarkCell          ActionID = "mark-cell"
	ActionNameCell          ActionID = "name-cell"

	ActionClose            ActionID = "close"
	ActionAnswerYes        ActionID = "answer-yes"
	ActionAnswerNo         ActionID = "answer-no"
	ActionNewConnection    ActionID = "new-connection"
	ActionEditConnection   ActionID = "edit-connection"
	ActionDeleteConnection ActionID = "delete-connection"
	ActionListModels       ActionID = "list-models"
	ActionSaveForm         ActionID = "save-form"
	ActionTestConnection   ActionID = "test-connection"
	ActionSaveCell         ActionID = "save-cell"
	ActionPrettifyJSON     ActionID = "prettify-json"
	ActionSetNull          ActionID = "set-null"
	ActionSetEmpty         ActionID = "set-empty"
	ActionSetDefault       ActionID = "set-default"
	ActionRunWithValues    ActionID = "run-with-values"
	ActionWriteExport      ActionID = "write-export"
	ActionCopyValue        ActionID = "copy-value"
	ActionListSecondary    ActionID = "list-secondary"
	ActionStopSession      ActionID = "stop-session"
	ActionToggleValue      ActionID = "toggle-value"
	ActionKeepAllValues    ActionID = "keep-all-values"
	ActionKeepOnlyValue    ActionID = "keep-only-value"
	ActionApplyChanges     ActionID = "apply-changes"
	ActionDiscardChanges   ActionID = "discard-changes"
	ActionInsertAiSQL      ActionID = "insert-ai-sql"
	ActionStopAiReply      ActionID = "stop-ai-reply"
	ActionAskAiAgain       ActionID = "ask-ai-again"
	ActionCopyAiReply      ActionID = "copy-ai-reply"
	ActionNewAiChat        ActionID = "new-ai-chat"
	ActionShowAiChats      ActionID = "show-ai-chats"
	ActionScrollBack       ActionID = "scroll-back"
	ActionScrollForward    ActionID = "scroll-forward"
	ActionPreviousTurn     ActionID = "previous-turn"
	ActionNextTurn         ActionID = "next-turn"
	ActionChatToNotebook   ActionID = "chat-to-notebook"

	// The keys of a form, a field and a file picker. Every one of these was a chord
	// written into the interface before it was an action.
	ActionPreviousField  ActionID = "previous-field"
	ActionNextField      ActionID = "next-field"
	ActionPreviousValue  ActionID = "previous-value"
	ActionNextValue      ActionID = "next-value"
	ActionApplyStep      ActionID = "apply-step"
	ActionStepBack       ActionID = "step-back"
	ActionSendQuestion   ActionID = "send-question"
	ActionWriteNewline   ActionID = "write-newline"
	ActionPreviousRow    ActionID = "previous-row"
	ActionNextRow        ActionID = "next-row"
	ActionScrollLeft     ActionID = "scroll-left"
	ActionScrollRight    ActionID = "scroll-right"
	ActionOpenDirectory  ActionID = "open-directory"
	ActionLeaveDirectory ActionID = "leave-directory"
)

// globalActions are the ones the workspace handles wherever the focus is.
var globalActions = []ActionDefinition{
	{ID: ActionRunBatch, Label: "run every statement", WhileRunning: true, AnswersInResult: true, MainHint: true},
	{ID: ActionNewQueryTab, Label: "new query tab", WhileRunning: true},
	{ID: ActionToggleSidebar, Label: "show or hide the object tree", WhileRunning: true, MainHint: true},
	{ID: ActionToggleResult, Label: "show or hide the result", WhileRunning: true},
	{ID: ActionRevealSQL, Label: "edit the query for this result", WhileRunning: true, MainHint: true},
	{ID: ActionNameTab, Label: "name this tab", WhileRunning: true},
	{ID: ActionCloseTab, Label: "close the tab", WhileRunning: true},
	{ID: ActionReopenTab, Label: "reopen the last closed tab", WhileRunning: true},
	{ID: ActionPreviousTab, WhileRunning: true},
	{ID: ActionNextTab, Label: "next tab", WhileRunning: true},
	{ID: ActionPreviousConnection, WhileRunning: true},
	{ID: ActionNextConnection, WhileRunning: true},
	{ID: ActionActivateTab, Label: "go to a tab by its number", WhileRunning: true},

	{ID: ActionRunAtCursor, Label: "run the selection or the statement", WhileRunning: true, AnswersInResult: true, MainHint: true},
	{ID: ActionExplain, Label: "explain plan", Needs: NeedsPlansStatement, WhileRunning: true, AnswersInResult: true},
	{ID: ActionExplainAnalyze, Label: "explain analyze", Needs: NeedsMeasuresPlan, WhileRunning: true, AnswersInResult: true},
	{ID: ActionShowHistory, Label: "query history", WhileRunning: true},
	{ID: ActionShowSaved, Label: "saved queries", WhileRunning: true},
	{ID: ActionSaveQuery, Label: "save this query", WhileRunning: true, EditorOnly: true, MainHint: true},
	{ID: ActionShowActivity, Label: "server activity", Needs: NeedsServerSessions, WhileRunning: true},
	{ID: ActionUndoWrite, Label: "undo the last write", Needs: NeedsPlansWrites, WhileRunning: true},
	{ID: ActionShowThemes, Label: "theme", WhileRunning: true},
	{ID: ActionNewNotebookTab, Label: "new notebook tab", WhileRunning: true},
	{ID: ActionNewBuilderTab, Label: "new query builder tab", WhileRunning: true},
	{ID: ActionShowNotebooks, Label: "notebooks", WhileRunning: true},
	{ID: ActionNotebookRunPolicy, Label: "notebook run policy", WhileRunning: true},
	{ID: ActionWriteNotebookReport, Label: "write a report of this notebook", WhileRunning: true},
	{ID: ActionFocusSidebar, Label: "focus the object tree", WhileRunning: true},
	{ID: ActionFocusEditor, Label: "focus the editor", WhileRunning: true},
	{ID: ActionFocusResult, Label: "focus the result", WhileRunning: true},
	{ID: ActionCancelQuery, Label: "cancel the running query", Needs: NeedsCancelsRunning, WhileRunning: true, MainHint: true},
	{ID: ActionShowPalette, Label: "command palette", WhileRunning: true, MainHint: true},
	{ID: ActionShowAiChat, Label: "ask AI", WhileRunning: true, MainHint: true},
	{ID: ActionAiFixError, Label: "ask AI: fix the error", WhileRunning: true, MainHint: true},
	{ID: ActionSendToAi, Label: "copy the editor query into the chat field", WhileRunning: true, EditorOnly: true, MainHint: true},
	{ID: ActionNextPage, Label: "fetch more rows", WhileRunning: true, AnswersInResult: true, MainHint: true},
	{ID: ActionExportCSV, Label: "export the result as CSV", WhileRunning: true},
	{ID: ActionExportJSON, Label: "export the result as JSON", WhileRunning: true},
	{ID: ActionBeginTransaction, Label: "begin transaction", Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionCommitTransaction, Label: "commit transaction", Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionRollbackTransaction, Label: "rollback transaction", Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionToggleAutocommit, Label: "toggle autocommit", Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionOpenPicker, Label: "open the connection picker", WhileRunning: true},
	{ID: ActionCloseConnection, Label: "close the connection", WhileRunning: true},

	// The help is most useful while the user waits for the server.
	{ID: ActionShowHelp, Label: "help", WhileRunning: true, MainHint: true},

	// These leave whatever holds the caret, so both work from inside it.
	{ID: ActionFocusNextPane, Label: "move the focus to the next pane", WhileRunning: true, WhileTyping: true},
	{ID: ActionFocusPreviousPane, Label: "move the focus to the previous pane", WhileRunning: true, WhileTyping: true},

	{ID: ActionPreviousStatement},
	{ID: ActionNextStatement},
	{ID: ActionRefreshObjects, Label: "refresh the object tree"},
	{ID: ActionSelectView, Label: "go to a view of the result by its number"},
	{ID: ActionPreviousView},
	{ID: ActionNextView},
}

// gridActions work only on a result the grid already drew.
var gridActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionCursorLeft}, {ID: ActionCursorRight},
	{ID: ActionSortColumn, Label: "sort by the column under the cursor", Needs: NeedsSortsRead},
	{ID: ActionAddSortColumn, Label: "add that column to the sort", Needs: NeedsSortsRead},
	{ID: ActionOpenRow, Label: "open the row under the cursor"},
	{ID: ActionViewCell, Label: "open the cell viewer"},
	{ID: ActionEditCell, Label: "edit the cell under the cursor"},
	{ID: ActionToggleDelete, Label: "mark the row for deletion"},
	{ID: ActionDuplicateRow, Label: "duplicate the row"},
	{ID: ActionReviewChanges, Label: "review the staged changes"},
	{ID: ActionUndoChange, Label: "undo the last staged change"},
	{ID: ActionRedoChange, Label: "redo the last undone change"},
	{ID: ActionCountRows, Label: "count all result rows", AnswersInResult: true, MainHint: true},
	{ID: ActionFollowForeignKey, Label: "open the row referenced by the foreign key"},
	{ID: ActionInsertRow, Label: "insert a row"},
	{ID: ActionCopyMenu, Label: "copy: cell, row, or the whole result"},
	{ID: ActionOpenMenu, Label: "open the row and cell menu", MainHint: true},
	{ID: ActionCopyCSV, Label: "copy the result as CSV"},
	{ID: ActionCopyJSON, Label: "copy the result as JSON"},
	{ID: ActionCopyMarkdown, Label: "copy the result as Markdown"},
	{ID: ActionCopyInserts, Label: "copy the result as INSERTs"},
	{ID: ActionDiscardChanges, Label: "discard the staged changes"},
	{ID: ActionFilterByCell, Label: "filter by the cell under the cursor"},
	{ID: ActionFilterByValues, Label: "filter by values chosen from a list"},
	{ID: ActionExcludeCell, Label: "exclude that value"},
	{ID: ActionClearRewrites, Label: "clear the sort and the filters"},
	{ID: ActionPopFilter, Label: "remove the last filter"},
	{ID: ActionFreezeColumns, Label: "freeze the column under the cursor"},
	{ID: ActionToggleMasking, Label: "show or hide masked values"},
	{ID: ActionGoToColumn, Label: "go to a column by name"},
	{ID: ActionSearchColumns, Label: "search the rows on screen"},
	{ID: ActionFilterWhere, Label: "filter with a WHERE predicate"},
}

// planActions answer while the plan view is drawn in place of the grid.
var planActions = []ActionDefinition{
	{ID: ActionToggleRawPlan, Label: "switch between the plan tree and the raw plan"},
	{ID: ActionCopyPlan, Label: "copy the query plan"},
	{ID: ActionAiCheckPlan, Label: "send the plan to the chat, in the plan view", Needs: NeedsPlansStatement, MainHint: true},
}

// documentActions answer while the tree that opens the rows as documents is drawn. It holds
// a cursor and folds like the object tree, and it copies like the grid, so it answers to both
// kinds of key under bindings of its own.
var documentActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionFoldRow},
	{ID: ActionUnfoldRow},
	{ID: ActionOpenNode, Label: "open or fold the current document", MainHint: true},
	{ID: ActionCopyValue, Label: "copy the value under the cursor"},
	{ID: ActionCopyPath, Label: "copy the field name under the cursor"},
	{ID: ActionSearchColumns}, {ID: ActionCountRows},
	{ID: ActionClearRewrites}, {ID: ActionPopFilter},
}

// treeActions answer while the object tree holds the caret.
var treeActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionFoldRow}, {ID: ActionUnfoldRow},
	{ID: ActionOpenNode, Label: "open the object under the cursor", MainHint: true},
	{ID: ActionOpenInNewTab, Label: "open a second tab on the same table"},
	{ID: ActionDescribeTable, Label: "describe the table"},
	{ID: ActionObjectMenu, Label: "open the object menu", MainHint: true},
	{ID: ActionFilterTree, Label: "filter the tree"},
	{ID: ActionToggleFavourite, Label: "mark this object as a favourite"},
	{ID: ActionToggleSystemSchemas, Label: "show or hide the system schemas"},
}

// editorActions answer while the statement being written holds the caret. A move is bound to
// its plain chord and to its shifted twin, and the shifted one takes the selection along.
var editorActions = []ActionDefinition{
	{ID: ActionCaretLeft}, {ID: ActionCaretRight},
	{ID: ActionCaretUp}, {ID: ActionCaretDown},
	{ID: ActionCaretWordLeft}, {ID: ActionCaretWordRight},
	{ID: ActionCaretLineStart, Label: "go to the first word of the line, then to the start of the line"},
	{ID: ActionCaretLineEnd, Label: "go to the end of the line"},
	{ID: ActionCaretTextStart}, {ID: ActionCaretTextEnd},
	{ID: ActionCaretPageUp}, {ID: ActionCaretPageDown},
	{ID: ActionSelectAll, Label: "select the whole statement"},
	{ID: ActionDeleteBack}, {ID: ActionDeleteForward},
	{ID: ActionDeleteWordBack, Label: "delete the word before the caret"},
	{ID: ActionDeleteWordForward, Label: "delete the word after the caret"},
	{ID: ActionOpenLine},
	{ID: ActionUndoEdit, Label: "undo the last edit"},
	{ID: ActionRedoEdit, Label: "redo the last edit"},
	{ID: ActionPasteText, Label: "paste from the system clipboard"},
	{ID: ActionFormatSQL, Label: "format the statement"},
	{ID: ActionCommentLines, Label: "comment or uncomment the lines"},
	{ID: ActionIndentLines}, {ID: ActionOutdentLines},
	{ID: ActionFindInStatement, Label: "find text in the statement"},
	{ID: ActionNextMatch},
	{ID: ActionPreviousMatch},
	{ID: ActionReplaceMatch, Label: "replace the match and go to the next"},
	{ID: ActionNextProblem},
	{ID: ActionShowCompletion, Label: "complete the word under the caret"},
	{ID: ActionLeaveCell, Label: "leave the cell and go back to the list", MainHint: true},
}

// notebookActions answer while the cell list of a notebook holds the keyboard.
var notebookActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionEditCellSource, Label: "edit the focused cell"},
	{ID: ActionRunCell, Label: "run the focused cell", AnswersInResult: true, MainHint: true},
	{ID: ActionRunFromCell, Label: "run the focused cell and every cell below", AnswersInResult: true, MainHint: true},
	{ID: ActionRunMarkedCells, Label: "run the marked cells", AnswersInResult: true},
	{ID: ActionAddCellBelow, Label: "add a cell below"},
	{ID: ActionAddCellAbove},
	{ID: ActionSetCellKind, Label: "set the cell kind"},
	{ID: ActionDeleteCell, Label: "delete the focused cell"},
	{ID: ActionUndoCellChange}, {ID: ActionRedoCellChange},
	{ID: ActionMoveCellUp}, {ID: ActionMoveCellDown},
	{ID: ActionCopyCell},
	{ID: ActionCutCell},
	{ID: ActionPasteCell, Label: "paste the cell below the focused one"},
	{ID: ActionToggleCellOutput}, {ID: ActionToggleEveryOutput},
	{ID: ActionMarkCell, Label: "mark a cell for a partial run"},
	{ID: ActionNameCell, Label: "name the focused cell"},
}

// builderActions move the diagram of a query builder tab.
var builderActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionPreviousTable}, {ID: ActionNextTable},
	{ID: ActionPickColumn, Label: "add the column to SELECT", MainHint: true},
	{ID: ActionEditBuilderRow, Label: "edit the aggregate, alias and sort", MainHint: true},
	{ID: ActionAddBuilderTable, Label: "add a table, joined on its foreign key", MainHint: true},
	{ID: ActionAddBuilderFilter, Label: "add one condition of the where clause"},
	{ID: ActionDropBuilderRow, Label: "drop the table or the filter under the cursor"},
	{ID: ActionSendToEditor, Label: "send the statement to a query tab", MainHint: true},
}

// listActions move any list moved by keys that is not the grid or the tree: palette,
// history, saved queries, connections, column values. One preset moves them all.
var listActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionChooseRow, Label: "choose the row under the cursor", MainHint: true},
}

// dialogActions answer while an overlay, the picker or a form is open, which owns the
// keyboard.
var dialogActions = []ActionDefinition{
	// The find field turns into the replace field, so replacing is bound where that
	// field stands rather than in the editor.
	{ID: ActionReplaceInStatement, Label: "replace every match, in the find field"},
	{ID: ActionClose, Label: "close the card, or cancel", MainHint: true},
	{ID: ActionAnswerYes, Label: "confirm the action", MainHint: true},
	{ID: ActionAnswerNo, Label: "decline the action", MainHint: true},
	{ID: ActionNewConnection, Label: "add a connection in the picker"},
	{ID: ActionEditConnection, Label: "edit the selected connection"},
	{ID: ActionDeleteConnection, Label: "delete the selected connection"},
	{ID: ActionFilterConnections},
	{ID: ActionListModels, Label: "read the models of the agent, in the settings"},
	{ID: ActionSaveForm, Label: "save the connection form", MainHint: true},
	{ID: ActionTestConnection, Label: "test the connection, in the form"},
	{ID: ActionSaveCell, Label: "stage the cell edit", MainHint: true},
	{ID: ActionPrettifyJSON, Label: "format the JSON"},
	{ID: ActionSetNull, Label: "stage NULL for the cell"},
	{ID: ActionSetEmpty, Label: "stage an empty value for the cell"},
	{ID: ActionSetDefault, Label: "stage DEFAULT for the cell"},
	{ID: ActionRunWithValues, Label: "run, in the values form", MainHint: true},
	{ID: ActionWriteExport, Label: "write the file, in the export form", MainHint: true},
	{ID: ActionCopyValue, Label: "copy the value from the cell viewer", MainHint: true},
	{ID: ActionOpenInNewTab, Label: "open a history query in a new tab"},
	{ID: ActionListSecondary, Label: "run the secondary list action"},
	{ID: ActionStopSession, Label: "stop the selected session's statement", MainHint: true},
	// A card with panels of its own folds them, with the keys that fold a schema.
	{ID: ActionFoldRow}, {ID: ActionUnfoldRow},
	{ID: ActionToggleValue, Label: "keep or drop a value, in the picker", MainHint: true},
	{ID: ActionKeepAllValues, Label: "keep every value again"},
	{ID: ActionKeepOnlyValue, Label: "keep only the value under the cursor"},
	{ID: ActionApplyChanges, Label: "apply staged changes from the review", MainHint: true},
	{ID: ActionDiscardChanges, Label: "discard staged changes from the review", MainHint: true},
	{ID: ActionInsertAiSQL, Label: "insert the most recent query of the chat into the editor, or as a cell", MainHint: true},
	{ID: ActionStopAiReply, Label: "stop the reply", MainHint: true},
	{ID: ActionAskAiAgain, Label: "ask the last question again"},
	{ID: ActionCopyAiReply, Label: "copy the last reply"},
	{ID: ActionNewAiChat, Label: "start a new conversation"},
	{ID: ActionShowAiChats, Label: "list conversations for this profile"},
	{ID: ActionScrollBack}, {ID: ActionScrollForward},
	{ID: ActionPreviousTurn}, {ID: ActionNextTurn},
	{ID: ActionChatToNotebook, Label: "turn this chat into a notebook"},
	{ID: ActionPreviousField}, {ID: ActionNextField},
	{ID: ActionPreviousValue}, {ID: ActionNextValue},
	{ID: ActionApplyStep, Label: "the next stage of an import, or the run of a dump", MainHint: true},
	{ID: ActionStepBack, Label: "back to the form, from the review of an import", MainHint: true},
	{ID: ActionSendQuestion, Label: "send the question", MainHint: true},
	{ID: ActionWriteNewline, Label: "a newline in the question"},
	{ID: ActionPreviousRow, MainHint: true}, {ID: ActionNextRow, MainHint: true},
	{ID: ActionScrollLeft}, {ID: ActionScrollRight},
	{ID: ActionOpenDirectory}, {ID: ActionLeaveDirectory},
	// The find field marks whole words only, or every match of the term.
	{ID: ActionToggleWholeWord, Label: "toggle whole-word matching"},
	// The list of completions owns the keyboard while it is open, as a card does.
	{ID: ActionAcceptCompletion, Label: "accept a completion"},
}

// ActionCatalog holds every action of every scope.
var ActionCatalog = func() []ActionDefinition {
	catalog := []ActionDefinition{}
	add := func(scope cfg.KeyScope, actions []ActionDefinition) {
		for _, action := range actions {
			action.Scope = scope
			catalog = append(catalog, action)
		}
	}
	add(cfg.ScopeGlobal, globalActions)
	add(cfg.ScopeGrid, gridActions)
	add(cfg.ScopePlan, planActions)
	add(cfg.ScopeDocument, documentActions)
	add(cfg.ScopeTree, treeActions)
	add(cfg.ScopeEditor, editorActions)
	add(cfg.ScopeNotebook, notebookActions)
	add(cfg.ScopeBuilder, builderActions)
	add(cfg.ScopeList, listActions)
	add(cfg.ScopeDialog, dialogActions)
	return catalog
}()

var actionsByKey = func() map[string]ActionDefinition {
	byKey := map[string]ActionDefinition{}
	for _, action := range ActionCatalog {
		byKey[cfg.BuildActionKey(action.Scope, string(action.ID))] = action
	}
	return byKey
}()

// collectScopeActions returns the id of every action a scope holds.
func collectScopeActions(scope cfg.KeyScope) []ActionID {
	ids := []ActionID{}
	for _, action := range ActionCatalog {
		if action.Scope == scope {
			ids = append(ids, action.ID)
		}
	}
	return ids
}

// FindAction returns the definition of the action, or nothing if no scope has it.
func FindAction(scope cfg.KeyScope, id ActionID) (ActionDefinition, bool) {
	action, known := actionsByKey[cfg.BuildActionKey(scope, string(id))]
	return action, known
}

// FindActionID reads this text as an action of the catalog.
func FindActionID(written string) (ActionID, bool) {
	for _, action := range ActionCatalog {
		if string(action.ID) == written {
			return action.ID, true
		}
	}
	return "", false
}

// AnswersInResult is true if the result of this action goes into the result pane. The same id
// stands in more than one scope, so the scope picks which definition answers.
func AnswersInResult(scope cfg.KeyScope, id ActionID) bool {
	action, known := FindAction(scope, id)
	return known && action.AnswersInResult
}

// IsMainHint is true for an action the main mode of the key hints shows. The same id is in
// more than one scope, and the scope picks the definition.
func IsMainHint(scope cfg.KeyScope, id ActionID) bool {
	action, known := FindAction(scope, id)
	return known && action.MainHint
}

// FindActionCapability returns the capability this action needs in this scope, or nothing if
// every server has it.
func FindActionCapability(scope cfg.KeyScope, id ActionID) Capability {
	action, known := FindAction(scope, id)
	if !known {
		return NeedsNothing
	}
	return action.Needs
}
