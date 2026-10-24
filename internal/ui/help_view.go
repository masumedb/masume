package ui

import (
	"strings"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
)

// helpRow is one row of the help: a key and what it does, under the name of a section.
type helpRow struct {
	Section string
	Chord   string
	Label   string
}

// buildHelpRows returns every row of the help, in the order of the groups.
func (model *Model) buildHelpRows() []helpRow {
	rows := []helpRow{}
	for _, section := range model.listHelpSections() {
		for _, entry := range section.Entries {
			chord := model.describeHelpKeys(entry)
			if chord == "" {
				continue
			}
			rows = append(rows, helpRow{
				Section: section.Title, Chord: chord, Label: readHelpText(entry),
			})
		}
	}
	return rows
}

// findHelpRows returns the rows whose keys, text or group name hold the term.
func (model *Model) findHelpRows(term string) []helpRow {
	kept := []helpRow{}
	for _, row := range model.buildHelpRows() {
		if present.MatchesText(row.Chord+" "+row.Label+" "+row.Section, term) {
			kept = append(kept, row)
		}
	}
	return kept
}

// describeHelpKeys writes the keys of one help row. The help is the reference for the keys,
// so it writes a chord in full where the rest of the interface draws a glyph. A row of
// actions carries the chord each one is bound to now, so a rebound key moves the help with it.
func (model *Model) describeHelpKeys(entry HelpEntry) string {
	if len(entry.Actions) == 0 {
		return entry.Keys
	}
	written := []string{}
	for _, action := range entry.Actions {
		if chord := model.registry.FormatFirstActionChordName(
			entry.Scope, action); chord != "" {
			written = append(written, chord)
		}
	}
	return strings.Join(written, "  ")
}

// helpPlaceholder is what the search line of the help asks for.
const helpPlaceholder = "filter keys"

// scrollHelpByCursor moves the help, which scrolls without a cursor of its own and so keeps
// how far it has scrolled where a list keeps its cursor. Without this a drag of its bar writes
// an offset the card never reads, and the bar returns a drag by standing still.
func scrollHelpByCursor(overlay *app.Overlay, offset int) {
	overlay.List.Cursor, overlay.List.Offset = offset, offset
}

// renderHelp draws every key that is bound, grouped the way a reader looks for one. A term
// drops the groups, and names the group of every row it kept.
func (model *Model) renderHelp(overlay app.Overlay, width int) string {
	term := model.readOverlayTerm(overlay)
	filter := model.renderSearchField(overlay, width, helpPlaceholder, -1)

	if term != "" {
		found := model.findHelpRows(term)
		filter = model.renderSearchField(overlay, width, helpPlaceholder, len(found))
		keyWidth := measureHelpKeyWidth(found, width)
		sectionWidth := measureHelpSectionWidth(found, width-6-keyWidth)
		written := make([]string, 0, len(found))
		for _, row := range found {
			written = append(written, model.renderFoundHelpRow(row, keyWidth, sectionWidth, width))
		}
		return model.renderListCard(ListCard{
			Kind: app.OverlayHelp, Title: " help ", Filter: filter, Rows: written,
			Cursor: overlay.List.Cursor, Offset: overlay.List.Cursor, Width: width,
			Scrolls: scrollHelpByCursor,
			Keys:    model.buildCardKeys(app.OverlayHelp, keyScene{overlay: overlay}),
		})
	}

	written := []string{}
	for _, section := range model.listHelpSections() {
		written = append(written, paintText(model.styles.Theme.AccentAlt, model.styles.Theme.Panel, " "+section.Title))
		rows := []helpRow{}
		for _, entry := range section.Entries {
			// An action nothing is bound to has no row of its own: it is reached from the
			// palette until a chord is given to it.
			chord := model.describeHelpKeys(entry)
			if chord == "" {
				continue
			}
			rows = append(rows, helpRow{Chord: chord, Label: readHelpText(entry)})
		}
		keyWidth := measureHelpKeyWidth(rows, width)
		for _, row := range rows {
			written = append(written, model.renderHelpRow(row.Chord, row.Label, keyWidth, width))
		}
		// Every group keeps a blank row under it, the last one too.
		written = append(written, "")
	}
	return model.renderListCard(ListCard{
		Kind: app.OverlayHelp, Title: " help ", Filter: filter, Rows: written,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Cursor, Width: width,
		Scrolls: scrollHelpByCursor,
		Keys:    model.buildCardKeys(app.OverlayHelp, keyScene{overlay: overlay}),
	})
}

// measureHelpKeyWidth returns the width of the key column: the widest key of the rows, with
// the indent and the gap, and at most half the card.
func measureHelpKeyWidth(rows []helpRow, width int) int {
	widest := 0
	for _, row := range rows {
		widest = max(widest, present.MeasureText(row.Chord))
	}
	return min(helpKeyIndent+widest+helpColumnGap, max(width-4, 0)/2)
}

// measureHelpSectionWidth returns the width of the section column: the widest section of the
// rows, and at most half the room.
func measureHelpSectionWidth(rows []helpRow, room int) int {
	widest := 0
	for _, row := range rows {
		widest = max(widest, present.MeasureText(row.Section))
	}
	return min(widest, max(room, 0)/2)
}

// renderHelpRow draws one row of the help: the keys, then what they do.
func (model *Model) renderHelpRow(chord, text string, keyWidth, width int) string {
	theme := model.styles.Theme
	room := max(width-4-keyWidth, 0)
	written := paintText(theme.Muted, theme.Panel, present.FitText("  "+chord, keyWidth)) +
		paintText(theme.Text, theme.Panel, present.TruncateText(text, room))
	return padStyledOn(" "+written, width-1, theme.Panel) +
		paintOn(theme.Panel, " ")
}

// renderFoundHelpRow draws one row the search kept, with its section on the right.
func (model *Model) renderFoundHelpRow(row helpRow, keyWidth, sectionWidth, width int) string {
	theme := model.styles.Theme
	labelWidth := max(width-6-keyWidth-sectionWidth, 0)
	written := paintText(theme.Muted, theme.Panel, present.FitText("  "+row.Chord, keyWidth)) +
		paintText(theme.Text, theme.Panel, present.FitText(
			present.TruncateText(row.Label, max(labelWidth-helpColumnGap, 0)), labelWidth)) +
		paintText(theme.Faint, theme.Panel, present.TruncateText(row.Section, sectionWidth))
	return padStyledOn(" "+written, width-1, theme.Panel) +
		paintOn(theme.Panel, " ")
}
