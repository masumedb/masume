package ui

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
)

// The timing of the effects of a drag: the time between two frames, the fade-in of a drop
// zone, and the flash of an item where it lands.
const (
	animationFrame = 33 * time.Millisecond
	dropFadeIn     = 120 * time.Millisecond
	landingFlash   = 360 * time.Millisecond
)

// The weights the effects of a drag mix a ground with: the slot of the dragged item toward
// the background, and the landed item toward the accent at the start of its flash.
const (
	placeholderTint = 0.55
	landingTint     = 0.4
)

// cellSpan is a run of cells of one screen row.
type cellSpan struct {
	row, from, to int
}

// landing is the flash of an item where a drag dropped it. The spans are read from the
// frame on screen, so the flash follows the item while the frame changes.
type landing struct {
	spans func() []cellSpan
	at    time.Time
}

// animationState is the effect running between frames, and whether a frame for it is asked
// for already.
type animationState struct {
	landing   *landing
	scheduled bool
}

// startLanding flashes an item where a drag dropped it, and asks for the frames of the flash.
func (model *Model) startLanding(spans func() []cellSpan) tea.Cmd {
	model.animation.landing = &landing{spans: spans, at: time.Now()}
	return model.scheduleAnimation()
}

// isAnimating is true while a drop zone fades in or an item flashes where it landed.
func (model *Model) isAnimating() bool {
	if held := model.animation.landing; held != nil && time.Since(held.at) < landingFlash {
		return true
	}
	return model.drag.holds(dragTab) && model.drag.dropping &&
		time.Since(model.drag.dropSince) < dropFadeIn
}

// scheduleAnimation asks for the next frame of a running effect, and for nothing where none
// runs or a frame is asked for already.
func (model *Model) scheduleAnimation() tea.Cmd {
	if model.animation.scheduled || !model.isAnimating() {
		return nil
	}
	model.animation.scheduled = true
	return wake(animationFrame)
}

// continueAnimation reads a wake: the frame it drew was the one asked for, and the next one is
// asked for while the effect runs.
func (model *Model) continueAnimation() tea.Cmd {
	model.animation.scheduled = false
	if held := model.animation.landing; held != nil && time.Since(held.at) >= landingFlash {
		model.animation.landing = nil
	}
	return model.scheduleAnimation()
}

// resolveDropTint returns the weight of the drop zone, which grows while it fades in.
func (model *Model) resolveDropTint() float64 {
	share := float64(time.Since(model.drag.dropSince)) / float64(dropFadeIn)
	return dropTint * min(max(share, 0.25), 1)
}

// findDragSpans returns the cells of the item a drag moves, where the frame on screen draws
// it.
func (model *Model) findDragSpans() []cellSpan {
	connection := model.Active()
	if connection == nil || !model.drag.lifted {
		return nil
	}
	switch model.drag.kind {
	case dragTab:
		return model.findTabSpans(connection, model.drag.dragged.tab)
	case dragColumnHeader:
		return model.findColumnSpans(model.drag.column)
	case dragCell:
		if book := connection.Active().Notebook; book != nil {
			return model.findCellSpans(book.Focused)
		}
	}
	return nil
}

// findTabSpans returns the cells of the tab of that id on the tab row.
func (model *Model) findTabSpans(connection *app.Connection, id int) []cellSpan {
	index := connection.IndexOfTab(id)
	for _, held := range model.layout.tabs {
		if held.index == index {
			return []cellSpan{{row: model.layout.tabRow, from: held.from, to: held.to}}
		}
	}
	return nil
}

// findColumnSpans returns the cells of a column of the grid: its name and its rows on screen.
func (model *Model) findColumnSpans(column int) []cellSpan {
	layout := model.layout
	for _, held := range layout.gridColumns {
		if held.index != column {
			continue
		}
		spans := []cellSpan{{row: layout.gridHeaderRow, from: held.from, to: held.to}}
		for at := range layout.gridRows.count {
			spans = append(spans, cellSpan{
				row: layout.gridRows.top + at, from: held.from, to: held.to,
			})
		}
		return spans
	}
	return nil
}

// findCellSpans returns the rows of a notebook cell on screen.
func (model *Model) findCellSpans(cell int) []cellSpan {
	block := model.layout.cellRows
	spans := []cellSpan{}
	for at := range block.count {
		item := block.offset + at
		if item < len(model.cellsOfRows) && model.cellsOfRows[item] == cell {
			spans = append(spans, cellSpan{row: block.top + at, from: block.from, to: block.to})
		}
	}
	return spans
}

// findSideSpans returns the cells of one side of the split view.
func (model *Model) findSideSpans(side int) []cellSpan {
	zone := model.layout.sideRects[side]
	spans := make([]cellSpan, 0, zone.height)
	for row := zone.top; row < zone.top+zone.height; row++ {
		spans = append(spans, cellSpan{row: row, from: zone.left, to: zone.left + zone.width - 1})
	}
	return spans
}

// tintSpans mixes the ground of every span toward a colour.
func (model *Model) tintSpans(
	frame string, spans []cellSpan, toward color.Color, weight float64,
) string {
	if len(spans) == 0 || weight <= 0 {
		return frame
	}
	rows := strings.Split(frame, "\n")
	for _, span := range spans {
		if span.row < 0 || span.row >= len(rows) {
			continue
		}
		cells := mapCells(rows[span.row])
		model.tintRowCells(cells, span.from, span.to, toward, weight)
		rows[span.row] = writeCells(cells)
	}
	return strings.Join(rows, "\n")
}

// paintDragPlaceholder dims the slot of the item a drag moves, ground and ink, so the frame
// shows where the item lands while the ghost of it follows the pointer.
func (model *Model) paintDragPlaceholder(frame string) string {
	spans := model.findDragSpans()
	if len(spans) == 0 {
		return frame
	}
	theme := model.styles.Theme
	rows := strings.Split(frame, "\n")
	for _, span := range spans {
		if span.row < 0 || span.row >= len(rows) {
			continue
		}
		cells := mapCells(rows[span.row])
		for at := max(span.from, 0); at <= span.to && at < len(cells); at++ {
			ink, ground := readCellColors(cells[at].sgr)
			if ground == nil {
				ground = theme.Panel
			}
			if ink == nil {
				ink = theme.Text
			}
			ground = MixColors(ground, theme.Background, placeholderTint)
			cells[at].sgr += writeColorSequence("48", ground) +
				writeColorSequence("38", MixColors(ink, ground, placeholderTint))
		}
		rows[span.row] = writeCells(cells)
	}
	return strings.Join(rows, "\n")
}

// paintLanding flashes the item a drag dropped, fading from the accent to its own colours.
func (model *Model) paintLanding(frame string) string {
	held := model.animation.landing
	if held == nil {
		return frame
	}
	share := float64(time.Since(held.at)) / float64(landingFlash)
	if share >= 1 {
		return frame
	}
	return model.tintSpans(frame, held.spans(), model.styles.Theme.Accent,
		landingTint*(1-share))
}

// describeDragGhost returns the label the ghost of a dragged item carries, or nothing where
// no item is dragged.
func (model *Model) describeDragGhost() string {
	connection := model.Active()
	if connection == nil || !model.drag.lifted {
		return ""
	}
	switch model.drag.kind {
	case dragTab:
		if index := connection.IndexOfTab(model.drag.dragged.tab); index >= 0 {
			tab := connection.Tabs[index]
			return model.icons.Prefix(resolveTabIcon(tab)) + tab.Label()
		}
	case dragColumnHeader:
		tab := connection.Active()
		shape := model.buildGridShape(connection, tab)
		if model.drag.column < len(shape.Columns) {
			return shape.Columns[model.drag.column].Name
		}
	case dragCell:
		if book := connection.Active().Notebook; book != nil {
			cell := book.GetFocusedCell()
			return string(cell.Kind) + "  " + cell.BuildTitle()
		}
	}
	return ""
}

// ghostOffset is the cells between the pointer and the ghost, so the ghost never covers the
// cell the pointer stands on.
const ghostOffset = 2

// ghostWidest is the most cells the label of a ghost takes.
const ghostWidest = 40

// paintDragGhost draws the label of the dragged item beside the pointer, as a chip that
// follows it cell by cell.
func (model *Model) paintDragGhost(frame string) string {
	label := model.describeDragGhost()
	if label == "" {
		return frame
	}
	text := " " + present.TruncateText(label, ghostWidest) + " "
	width := present.MeasureText(text)
	rows := strings.Split(frame, "\n")
	row := model.frame.pointerY + 1
	if row >= len(rows)-1 {
		row = model.frame.pointerY - 1
	}
	left := min(model.frame.pointerX+ghostOffset, model.width-width)
	if row < 0 || row >= len(rows) || left < 0 {
		return frame
	}

	theme := model.styles.Theme
	sgr := buildSgr(model.styles.InkOn(theme.AccentAlt), theme.AccentAlt)
	cells := mapCells(rows[row])
	// A wide character the chip cuts in half is drawn as a blank, so the row keeps its width.
	if left > 0 && left < len(cells) && cells[left].text == "" {
		cells[left-1].text = " "
	}
	at := left
	for _, character := range text {
		size := max(present.MeasureText(string(character)), 1)
		if at+size > len(cells) {
			break
		}
		cells[at] = styledCell{sgr: sgr, text: string(character)}
		for filler := 1; filler < size; filler++ {
			cells[at+filler] = styledCell{sgr: sgr}
		}
		at += size
	}
	if at < len(cells) && cells[at].text == "" {
		cells[at].text = " "
	}
	rows[row] = writeCells(cells)
	return strings.Join(rows, "\n")
}
