package ui

import (
	"math"
	"strings"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// cardButton is one button of the row at the foot of a card, or a word of that row.
type cardButton struct {
	// The glyph of what the button acts on, drawn after the chord.
	icon   cfg.IconKind
	chord  string
	label  string
	scope  cfg.KeyScope
	action ActionID
	// second is what the right half of a button of a pair runs.
	second ActionID
	// True for the primary action, drawn filled while no button has the keyboard focus.
	primary bool
	// True for an action that writes or removes, drawn in the error colour.
	destructive bool
	// True for a word of the card, drawn as plain text.
	readout bool
}

// buildCardButton returns a button with the first chord of the action.
func (model *Model) buildCardButton(scope cfg.KeyScope, action ActionID, label string) cardButton {
	return cardButton{
		chord: model.registry.FormatFirstActionChord(scope, action), label: label,
		scope: scope, action: action,
	}
}

// The blank cells between two buttons, and inside each end of a button.
const (
	cardButtonGap     = 1
	cardButtonPadding = 1
)

// noButtonFocus is the focus start of a card whose fields or list have the keyboard focus.
const noButtonFocus = -1

// buildText returns the text of the button, padding included.
func (button cardButton) buildText(icons IconSet) string {
	written := []string{}
	if button.chord != "" {
		written = append(written, button.chord)
	}
	if glyph := icons.Icon(button.icon); button.icon != "" && glyph != "" {
		written = append(written, glyph)
	}
	if button.label != "" {
		written = append(written, button.label)
	}
	if button.readout {
		return strings.Join(written, " ")
	}
	padding := strings.Repeat(" ", cardButtonPadding)
	return padding + strings.Join(written, " ") + padding
}

// isFocusable is true for a button the keyboard can move to.
func (button cardButton) isFocusable() bool {
	return !button.readout && button.action != ""
}

// layoutButtonRows splits the buttons into rows of at most width cells. A button wider than
// the row takes a row of its own.
func layoutButtonRows(icons IconSet, buttons []cardButton, width int) [][]cardButton {
	rows := [][]cardButton{}
	used := 0
	for _, button := range buttons {
		size := present.MeasureText(button.buildText(icons))
		if len(rows) == 0 || (used > 0 && used+cardButtonGap+size > width) {
			rows = append(rows, []cardButton{})
			used = 0
		}
		if used > 0 {
			used += cardButtonGap
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], button)
		used += size
	}
	return rows
}

// measureButtonRow returns the cells the buttons of a card take on one row.
func (model *Model) measureButtonRow(buttons []cardButton) int {
	width := 0
	for index, button := range buttons {
		if index > 0 {
			width += cardButtonGap
		}
		width += present.MeasureText(button.buildText(model.icons))
	}
	return width
}

// renderButtonRow draws the buttons of a card on one row. Start is the index of the focusable
// button with the keyboard focus when the card opens, or noButtonFocus.
func (model *Model) renderButtonRow(buttons []cardButton, start, row, left int) string {
	return model.renderButtons(buttons, start, math.MaxInt, row, left)[0]
}

// renderButtons draws the buttons of a card in rows of at most width cells, and records each
// one as a button of the frame. The row and the left column are counted from the card.
func (model *Model) renderButtons(
	buttons []cardButton, start, width, top, left int,
) []string {
	theme := model.styles.Theme
	focusable := 0
	for _, button := range buttons {
		if button.isFocusable() {
			focusable++
		}
	}
	model.layout.cardButtonStart = start
	focused := model.resolveButtonFocus(start, focusable)

	rows := layoutButtonRows(model.icons, buttons, width)
	lines := make([]string, 0, max(len(rows), 1))
	slot := 0
	for index, row := range rows {
		var written strings.Builder
		at := left
		for place, button := range row {
			if place > 0 {
				writeBlanksOn(&written, theme.Panel, cardButtonGap)
				at += cardButtonGap
			}
			text := button.buildText(model.icons)
			size := present.MeasureText(text)
			if button.readout {
				writeTextOn(&written, theme.Muted, theme.Panel, text)
				at += size
				continue
			}
			filled := button.primary && focused == noButtonFocus
			if button.isFocusable() {
				filled = filled || slot == focused
				slot++
			}
			model.writeButton(&written, button, filled)
			if button.action != "" {
				chordTo := at + cardButtonPadding + present.MeasureText(button.chord) - 1
				model.layout.buttons = append(model.layout.buttons, buttonHit{
					row: top + index, from: at, to: at + size - 1, keyTo: max(chordTo, at),
					scope: button.scope, action: button.action, second: button.second,
					slot: slot,
				})
			}
			at += size
		}
		lines = append(lines, written.String())
	}
	if len(lines) == 0 {
		lines = append(lines, "")
	}
	return lines
}

// writeButton draws one button: the chord in the accent ink and the label in the text ink on
// the header ground, or every part in the ink of a filled ground.
func (model *Model) writeButton(written *strings.Builder, button cardButton, filled bool) {
	theme := model.styles.Theme
	ground, chordInk, labelInk := theme.Header, theme.Accent, theme.Text
	if button.destructive {
		chordInk, labelInk = theme.Error, theme.Error
	}
	if filled {
		ground = theme.Accent
		if button.destructive {
			ground = theme.Error
		}
		chordInk = model.styles.InkOn(ground)
		labelInk = chordInk
	}
	padding := strings.Repeat(" ", cardButtonPadding)
	writeTextOn(written, labelInk, ground, padding)
	wrote := false
	if button.chord != "" {
		writeTextOn(written, chordInk, ground, button.chord)
		wrote = true
	}
	if glyph := model.icons.Icon(button.icon); button.icon != "" && glyph != "" {
		glyphInk := model.styles.IconColor(button.icon)
		if filled {
			glyphInk = labelInk
		}
		writeTextOn(written, glyphInk, ground, blankBefore(wrote)+glyph)
		wrote = true
	}
	if button.label != "" {
		writeTextOn(written, labelInk, ground, blankBefore(wrote)+button.label)
	}
	writeTextOn(written, labelInk, ground, padding)
}

// renderFieldHeading draws the heading over a group of fields of a card.
func (model *Model) renderFieldHeading(text string) string {
	theme := model.styles.Theme
	return paintBoldText(theme.Muted, theme.Panel, strings.ToUpper(text))
}

// renderEnvironmentBadge draws the badge a card of a production connection has on its top
// border, and nothing for any other environment.
func (model *Model) renderEnvironmentBadge(environment cfg.Environment) string {
	if environment != cfg.EnvironmentProd {
		return ""
	}
	ground := model.styles.Theme.EnvProd
	return paintBoldText(model.styles.InkOn(ground), ground, " PRODUCTION ")
}

// renderActiveEnvironmentBadge draws the badge of the active connection.
func (model *Model) renderActiveEnvironmentBadge() string {
	connection := model.Active()
	if connection == nil {
		return ""
	}
	return model.renderEnvironmentBadge(connection.Profile().Environment)
}
