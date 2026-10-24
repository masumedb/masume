package ui

import (
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// promptHints name what each prompt does, which the field alone cannot show.
var promptHints = map[app.PromptKind]string{
	app.PromptBuilderFilter: "one condition of the where clause",
	app.PromptSearch:        "filters loaded rows · no server query",
	app.PromptWhere:         "filters on the server",
	app.PromptGoToColumn:    "goes to the first column whose name matches",
	app.PromptTabName:       "written as a comment on the first line of the query",
	app.PromptFind:          "marks every match",
	app.PromptReplace:       "replaces every match, in one step",
	app.PromptCellName:      "written as a comment on the first line of the cell",
}

// promptCards are the prompts that open a card of their own. A prompt that belongs to no
// pane is one of them, and so is one that takes a path and needs a line to say where it
// goes.
var promptCards = map[app.PromptKind]bool{
	app.PromptSaveName:       true,
	app.PromptNotebookName:   true,
	app.PromptNotebookRename: true,
	app.PromptNotebookReport: true,
	app.PromptAiNotebook:     true,
	app.PromptDataSearch:     true,
}

// drawsPromptBar is true for a prompt that opens a field at the foot of a pane.
func drawsPromptBar(overlay app.Overlay) bool {
	return overlay.Kind == app.OverlayPrompt && !promptCards[overlay.Prompt]
}

// promptPlaceholders name what a card of a prompt takes while its field is empty.
var promptPlaceholders = map[app.PromptKind]string{
	app.PromptNotebookName:   "name or path",
	app.PromptNotebookRename: "name",
	app.PromptNotebookReport: "path for the report",
	app.PromptAiNotebook:     "subject of the notebook",
	app.PromptDataSearch:     "search text",
}

// describePromptPlaceholder returns what the field of a card takes while it is empty.
func describePromptPlaceholder(prompt app.PromptKind) string {
	if held, found := promptPlaceholders[prompt]; found {
		return held
	}
	return "name for this query"
}

// findPromptBar returns the prompt to draw at the foot of a pane, and whether there is one.
func findPromptBar(connection *app.Connection, kinds ...app.PromptKind) (app.Overlay, bool) {
	overlay := connection.Overlay
	if !drawsPromptBar(overlay) {
		return app.Overlay{}, false
	}
	if slices.Contains(kinds, overlay.Prompt) {
		return overlay, true
	}
	return app.Overlay{}, false
}

// renderPromptBar draws the two rows a prompt takes at the foot of a pane: the field, and what
// answering it does.
func (model *Model) renderPromptBar(overlay app.Overlay, width int) []string {
	theme := model.styles.Theme
	label := paintText(theme.Accent, theme.Header, overlay.Title+" ")
	room := max(width-1-measureStyledWidth(label), 1)
	field := ""
	if overlay.Prompt == app.PromptWhere {
		field = model.renderSQLField(overlay.Draft, room)
		model.promptCaretCell = 1 + measureStyledWidth(label) +
			min(present.MeasureText(overlay.Draft.Text[:overlay.Draft.Caret]), room-1)
	} else {
		field = model.renderField(overlay.Draft, room, FieldLook{
			Ground: theme.Header, Ink: theme.Text, Focused: true,
		})
	}

	// The hint is cut where the pane ends, with nothing to mark the cut, because the keys it
	// names are on the bar below as well.
	hint := promptHints[overlay.Prompt]
	if counted := model.describeMatchCount(overlay); counted != "" {
		hint += " · " + counted
	}
	if model.showsKeyHints() {
		text := []string{hint}
		// Finding and replacing is one key. The find field shows the second half and the
		// step to the next match.
		if overlay.Prompt == app.PromptFind {
			if chord := model.registry.FormatFirstActionChord(
				cfg.ScopeDialog, ActionReplaceInStatement); chord != "" {
				text = append(text, chord+" replace")
			}
			if chord := model.registry.FormatFirstActionChord(
				cfg.ScopeEditor, ActionNextMatch); chord != "" {
				text = append(text, chord+" next match")
			}
		}
		if word := model.describeWholeWordKey(overlay); word != "" {
			text = append(text, word)
		}
		if chord := model.registry.FormatFirstActionChord(
			cfg.ScopeList, ActionChooseRow); chord != "" {
			text = append(text, chord+" apply")
		}
		if chord := model.registry.FormatFirstActionChord(
			cfg.ScopeDialog, ActionClose); chord != "" {
			text = append(text, chord+" cancel")
		}
		hint = strings.Join(text, " · ")
	}
	return []string{
		paintOn(theme.Header, " ") + label + field,
		" " + model.styles.Muted().Render(truncateCells(hint, width-1)),
	}
}

// renderSQLField draws a one-line field of SQL in the colours of the editor, with the caret.
func (model *Model) renderSQLField(buffer *app.EditorBuffer, width int) string {
	caret := present.MeasureText(buffer.Text[:min(buffer.Caret, len(buffer.Text))])
	return model.renderCodeLineOn(model.styles.Theme.Header, codeLine{
		text: buffer.Text, spans: buildSQLLineHighlights(buffer.Text)[0], width: width,
		columnOffset: max(caret-width+1, 0), caretColumn: caret, showCaret: true,
	})
}

// describeMatchCount counts what the term in the find field matches in the statement, so the
// count stands under the field while the term is still being typed.
func (model *Model) describeMatchCount(overlay app.Overlay) string {
	if overlay.Prompt != app.PromptFind && overlay.Prompt != app.PromptReplace {
		return ""
	}
	connection := model.Active()
	if connection == nil {
		return ""
	}
	tab := connection.Active()
	if tab == nil || !tab.EditorVisible() {
		return ""
	}
	term := tab.Find.Term
	if overlay.Prompt == app.PromptFind {
		term = overlay.Draft.Text
	}
	if term == "" {
		return ""
	}
	found := tab.Editor.FindMatches(term, tab.Find.WholeWord)
	if len(found) == 0 {
		return "no match"
	}
	return present.FormatCountOf(int64(len(found)), "match", "matches")
}

// describeWholeWordKey names the key that marks whole words only, and says which way the
// field stands now.
func (model *Model) describeWholeWordKey(overlay app.Overlay) string {
	if overlay.Prompt != app.PromptFind && overlay.Prompt != app.PromptReplace {
		return ""
	}
	chord := model.registry.FormatFirstActionChord(cfg.ScopeDialog, ActionToggleWholeWord)
	if chord == "" {
		return ""
	}
	connection := model.Active()
	if connection == nil {
		return ""
	}
	tab := connection.Active()
	if tab == nil {
		return ""
	}
	text := chord + " whole words only"
	if tab.Find.WholeWord {
		text = chord + " every match"
	}
	return text
}

// renderPrompt draws a one-line field, with what it is for under it.
func (model *Model) renderPrompt(overlay app.Overlay, width int) string {
	inner := width - 4
	keys := model.buildCardKeys(app.OverlayPrompt, keyScene{overlay: overlay})
	lines := []string{
		model.renderField(overlay.Draft, inner, FieldLook{
			Ground: model.styles.Theme.Header, Ink: model.styles.Theme.Text,
			Focused: true, KeepsPlaceholder: true,
			Placeholder: describePromptPlaceholder(overlay.Prompt),
		}),
		model.styles.Muted().Render(present.TruncateText(overlay.Hint, inner)),
	}
	lines = model.appendCardKeyRow(lines, keys, inner, cardBodyRow, cardBodyColumn)
	return model.renderCard(" "+overlay.Title+" ", width, lines, plainCard)
}
