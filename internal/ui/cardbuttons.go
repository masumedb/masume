package ui

import (
	"strings"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// cardButton is one button of the row at the foot of a card.
type cardButton struct {
	chord  string
	label  string
	scope  cfg.KeyScope
	action ActionID
	// True for the primary action, which is drawn filled.
	primary bool
	// True for a primary action that writes or removes, drawn in the error colour.
	destructive bool
}

// buildCardButton returns a button with the first chord of the action.
func (model *Model) buildCardButton(scope cfg.KeyScope, action ActionID, label string) cardButton {
	return cardButton{
		chord: model.registry.FormatFirstActionChord(scope, action), label: label,
		scope: scope, action: action,
	}
}

// cardButtonGap is the blank cells between two buttons.
const cardButtonGap = 2

// renderButtonRow draws the buttons of a card and records each one as a button of the frame.
// The row and the left column are counted from the card.
func (model *Model) renderButtonRow(buttons []cardButton, row, left int) string {
	theme := model.styles.Theme
	var written strings.Builder
	at := left
	for index, button := range buttons {
		if index > 0 {
			writeBlanksOn(&written, theme.Panel, cardButtonGap)
			at += cardButtonGap
		}
		ground, ink := theme.Header, theme.Text
		if button.primary {
			ground = theme.Accent
			if button.destructive {
				ground = theme.Error
			}
			ink = model.styles.InkOn(ground)
		}
		text := "  " + strings.TrimSpace(button.chord+" "+button.label) + "  "
		writeTextOn(&written, ink, ground, text)
		width := present.MeasureText(text)
		if button.action != "" {
			model.layout.buttons = append(model.layout.buttons, buttonHit{
				row: row, from: at, to: at + width - 1, keyTo: at + width - 1,
				scope: button.scope, action: button.action,
			})
		}
		at += width
	}
	return written.String()
}

// measureButtonRow returns the cells the buttons of a card take on one row.
func measureButtonRow(buttons []cardButton) int {
	width := 0
	for index, button := range buttons {
		if index > 0 {
			width += cardButtonGap
		}
		width += present.MeasureText("  " + strings.TrimSpace(button.chord+" "+button.label) + "  ")
	}
	return width
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
