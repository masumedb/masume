package ui

import (
	"image/color"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// keyPart is one part of the line a card names its keys on: the key, what it does, and what
// a press on the words runs. A part with no action behind it is a word the card says.
type keyPart struct {
	// The glyph of what the key acts on, drawn before the chord, or nothing for a key that
	// needs none.
	icon   cfg.IconKind
	chord  string
	label  string
	scope  cfg.KeyScope
	action ActionID
	// second is what the other half of a key of a pair runs, such as the step on beside
	// the step back.
	second ActionID
	// True for a primary key, which the main mode of the key hints shows as well.
	main bool
}

// buildText writes the part as the reader sees it: the key, the glyph of what it acts on, then
// what it does.
func (part keyPart) buildText(icons IconSet) string {
	written := []string{}
	if part.chord != "" {
		written = append(written, part.chord)
	}
	if glyph := icons.Icon(part.icon); part.icon != "" && glyph != "" {
		written = append(written, glyph)
	}
	if part.label != "" {
		written = append(written, part.label)
	}
	return strings.Join(written, " ")
}

// KeyLine is the keys a card names at its foot. It writes them as one line, and keeps what
// each one runs, so a press on the word runs what the key runs.
type KeyLine struct {
	registry *KeyRegistry
	icons    IconSet
	mode     cfg.KeyHintsMode
	parts    []keyPart
}

// buildKeyLine starts a line of keys in the key hints mode of this model.
func (model *Model) buildKeyLine() *KeyLine {
	return &KeyLine{
		registry: model.registry, icons: model.icons, mode: model.resolveKeyHints(),
	}
}

// listParts returns the parts this mode draws. A part with no chord is a readout. Every mode
// draws it.
func (line *KeyLine) listParts() []keyPart {
	if line == nil {
		return nil
	}
	drawn := make([]keyPart, 0, len(line.parts))
	for _, part := range line.parts {
		switch {
		case isArrowKeyText(part.chord):
		case part.chord == "":
			drawn = append(drawn, part)
		case line.mode == cfg.KeyHintsFull || line.mode == "":
			drawn = append(drawn, part)
		case line.mode == cfg.KeyHintsOff:
		case !part.main:
		default:
			drawn = append(drawn, part)
		}
	}
	return drawn
}

// isArrowKeyText is true for a key drawn as bare arrow keys only, such as ↑↓ or ←→. A hint
// for such a key is never drawn.
func isArrowKeyText(chord string) bool {
	arrows := false
	for _, character := range chord {
		switch character {
		case '↑', '↓', '←', '→':
			arrows = true
		case ' ', '/':
		default:
			return false
		}
	}
	return arrows
}

// bind adds every chord of an action, with its label after them. An action with no chord is
// left out, as the status bar leaves it out.
func (line *KeyLine) bind(scope cfg.KeyScope, action ActionID, label string) *KeyLine {
	chord := line.registry.FormatActionChords(scope, action)
	if chord == "" {
		return line
	}
	return line.addKey(chord, label, scope, action)
}

// addKey adds a key drawn as this chord and this label, which runs this action. The catalog
// has the rank of the key.
func (line *KeyLine) addKey(chord, label string, scope cfg.KeyScope, action ActionID) *KeyLine {
	if chord == "" && label == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{
		chord: chord, label: label, scope: scope, action: action,
		main: IsMainHint(scope, action),
	})
	return line
}

// bindIcon adds the first chord of an action, with the glyph of what it acts on before it.
// A key that reaches the model carries the mark of the model.
func (line *KeyLine) bindIcon(
	scope cfg.KeyScope, action ActionID, icon cfg.IconKind, label string,
) *KeyLine {
	chord := line.registry.FormatFirstActionChord(scope, action)
	if chord == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{
		icon: icon, chord: chord, label: label, scope: scope, action: action,
		main: IsMainHint(scope, action),
	})
	return line
}

// bindFirstChord adds the first chord of an action, for a strip with room for one.
func (line *KeyLine) bindFirstChord(
	scope cfg.KeyScope, action ActionID, label string,
) *KeyLine {
	chord := line.registry.FormatFirstActionChord(scope, action)
	if chord == "" {
		return line
	}
	return line.addKey(chord, label, scope, action)
}

// bindPair adds one key for a pair of actions, such as the step back and the step on. A press
// on the first half runs the first action.
func (line *KeyLine) bindPair(
	scope cfg.KeyScope, previous, next ActionID, label, separator string,
) *KeyLine {
	chord := line.registry.FormatChordPair(scope, previous, next, separator)
	if chord == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{
		chord: chord, label: label, scope: scope, action: previous, second: next,
		main: IsMainHint(scope, previous),
	})
	return line
}

// bindJoinedPairs adds one key for two pairs of actions, drawn as one chord. A press on the
// first half runs the first action of the first pair.
func (line *KeyLine) bindJoinedPairs(first, second keySpec, label string) *KeyLine {
	chord := line.registry.FormatChordPair(first.scope, first.action, first.second, "") +
		line.registry.FormatChordPair(second.scope, second.action, second.second, "")
	if chord == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{
		chord: chord, label: label, scope: first.scope, action: first.action,
		second: first.second, main: IsMainHint(first.scope, first.action),
	})
	return line
}

// addAnswerKey adds the primary key of a field or a list: the key it is answered or left
// with. The registry has no such key, and no press runs it. It is drawn as every key is: the
// chord in the ink of a key, and what it does in the quiet ink.
func (line *KeyLine) addAnswerKey(chord, label string) *KeyLine {
	line.parts = append(line.parts, keyPart{chord: chord, label: label, main: true})
	return line
}

// addAsideKey adds a secondary key of a field or a list, such as a move or a scroll. Only
// the full mode shows it.
func (line *KeyLine) addAsideKey(chord, label string) *KeyLine {
	line.parts = append(line.parts, keyPart{chord: chord, label: label})
	return line
}

// addText adds a word of the card. It has no key, and no press runs it.
func (line *KeyLine) addText(text string) *KeyLine {
	if text == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{label: text})
	return line
}

// buildText writes the line as one string, with the parts a middle dot apart.
func (line *KeyLine) buildText() string {
	if line == nil {
		return ""
	}
	drawn := line.listParts()
	written := make([]string, 0, len(drawn))
	for _, part := range drawn {
		written = append(written, part.buildText(line.icons))
	}
	return strings.Join(written, hintSeparator)
}

// isEmpty is true for a line that names nothing.
func (line *KeyLine) isEmpty() bool {
	return line == nil || len(line.listParts()) == 0
}

// buildHints returns the line as the keys of the status bar. The bar under an open card then
// shows the keys of the card and not the keys of the pane behind it. The line already dropped
// the keys its mode hides. Every key it kept is a key the bar shows.
func (line *KeyLine) buildHints() []Hint {
	if line == nil {
		return nil
	}
	drawn := line.listParts()
	hints := make([]Hint, 0, len(drawn))
	for _, part := range drawn {
		if part.chord == "" {
			continue
		}
		hints = append(hints, Hint{
			Key: part.chord, Label: part.label, Scope: part.scope, Action: part.action,
			Main: true,
		})
	}
	return hints
}

// appendCardKeyRow puts the buttons of a card on one row at its foot, under a blank row that
// holds it off the content. A button that does not fit the width is left out.
func (model *Model) appendCardKeyRow(
	lines []string, keys *KeyLine, width, top, left int,
) []string {
	if keys.isEmpty() {
		return lines
	}
	lines = append(lines, "", "")
	lines[len(lines)-1] = model.renderKeyButtons(keys, width, 1, top+len(lines)-1, left)[0]
	return lines
}

// The actions a card button runs that submit the card, and the actions that remove or stop
// something.
var (
	submitActions = []ActionID{
		ActionChooseRow, ActionApplyStep, ActionSaveForm, ActionWriteExport,
		ActionRunWithValues, ActionSaveCell, ActionApplyChanges, ActionSendQuestion,
	}
	destructiveActions = []ActionID{
		ActionDeleteConnection, ActionDiscardChanges, ActionListSecondary, ActionStopSession,
	}
)

// buildButtons returns the parts of the line as the buttons of a card. The first part that
// submits the card is the primary button.
func (line *KeyLine) buildButtons() []cardButton {
	buttons := []cardButton{}
	primary := false
	for _, part := range line.listParts() {
		button := cardButton{
			icon: part.icon, chord: part.chord, label: part.label,
			scope: part.scope, action: part.action, second: part.second,
			readout:     part.chord == "" && part.action == "",
			destructive: slices.Contains(destructiveActions, part.action),
		}
		if !primary && slices.Contains(submitActions, part.action) {
			button.primary, primary = true, true
		}
		buttons = append(buttons, button)
	}
	return buttons
}

// countKeyRows returns the rows the buttons of a line take in this width.
func (line *KeyLine) countKeyRows(width int) int {
	if line.isEmpty() {
		return 0
	}
	return len(layoutButtonRows(line.icons, line.buildButtons(), width))
}

// measureKeyButtons returns the cells the buttons of a line take on one row.
func (model *Model) measureKeyButtons(line *KeyLine) int {
	if line.isEmpty() {
		return 0
	}
	return model.measureButtonRow(line.buildButtons())
}

// renderKeyButtons draws the parts of a line as the buttons of a card, in at most rows rows of
// width cells. A button past the last row is left out.
func (model *Model) renderKeyButtons(line *KeyLine, width, rows, top, left int) []string {
	buttons := line.buildButtons()
	laid := layoutButtonRows(model.icons, buttons, width)
	if len(laid) > rows {
		buttons = slices.Concat(laid[:rows]...)
	}
	return model.renderButtons(buttons, noButtonFocus, width, top, left)
}

// writeKeyPart draws one key: the chord, the glyph of what it acts on, and what it does.
func (model *Model) writeKeyPart(
	written *strings.Builder, part keyPart, ground color.Color,
) {
	theme := model.styles.Theme
	wrote := false
	if part.chord != "" {
		writeTextOn(written, theme.Accent, ground, part.chord)
		wrote = true
	}
	if glyph := model.icons.Icon(part.icon); part.icon != "" && glyph != "" {
		writeTextOn(written, model.styles.IconColor(part.icon), ground, blankBefore(wrote)+glyph)
		wrote = true
	}
	if part.label != "" {
		writeTextOn(written, theme.Muted, ground, blankBefore(wrote)+part.label)
	}
}

// blankBefore returns the blank that holds one part of a key apart from the one before it.
func blankBefore(wrote bool) string {
	if wrote {
		return " "
	}
	return ""
}

// measureKeyLine returns how many cells the line takes when it is drawn, so a strip that
// holds it against its right end knows where it starts.
func measureKeyLine(line *KeyLine) int {
	return present.MeasureText(line.buildText())
}

// writeKeyLine draws a line of keys on a strip and records the cells each key covers, so a
// press on the word runs what the key runs. The line is drawn from the column given, on the
// row given, both counted from the screen.
func (model *Model) writeKeyLine(
	line *KeyLine, ground color.Color, row, left int,
) string {
	if line.isEmpty() {
		return ""
	}
	var written strings.Builder
	at := left
	for index, part := range line.listParts() {
		if index > 0 {
			writeTextOn(&written, model.styles.Theme.Faint, ground, hintSeparator)
			at += present.MeasureText(hintSeparator)
		}
		width := present.MeasureText(part.buildText(line.icons))
		model.writeKeyPart(&written, part, ground)
		if part.action != "" {
			model.recordKeyPart(line, part, row, at, width)
		}
		at += width
	}
	return written.String()
}

// recordKeyPart keeps the cells one key of a line covers. A key of a pair holds both actions,
// and the half of the chord that was pressed decides which one runs.
func (model *Model) recordKeyPart(line *KeyLine, part keyPart, row, from, width int) {
	if width < 1 {
		return
	}
	model.layout.buttons = append(model.layout.buttons, buttonHit{
		row: row, from: from, to: from + width - 1,
		keyTo:  from + max(measureKeyWidth(part), 1) - 1,
		scope:  part.scope,
		action: part.action,
		second: part.second,
	})
}

// measureKeyWidth returns the cells the key itself covers, before the glyph and what it does.
// A press on the first half of a key of a pair runs the first action, and the half is measured
// from the key and not from what it says.
func measureKeyWidth(part keyPart) int {
	return present.MeasureText(part.chord)
}

// rememberCardKeys keeps the keys the card on show names, so the status bar under it names
// the same ones and a press on either runs the card and not the pane behind it.
func (model *Model) rememberCardKeys(line *KeyLine) {
	if line.isEmpty() {
		return
	}
	model.cardKeys = line
}
