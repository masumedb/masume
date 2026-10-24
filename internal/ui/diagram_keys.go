package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// scrollDiagram moves a diagram by rows and by columns, and stops where the last row and the
// last column come into view. It reports whether the action belonged to the diagram.
func (model *Model) scrollDiagram(overlay *app.Overlay, match Match) bool {
	rows, columns := model.layout.cardBody, model.layout.cardRoom
	lastRow := max(len(overlay.Diagram.Lines)-rows, 0)
	lastColumn := max(overlay.Diagram.Width-columns, 0)
	top := min(max(overlay.List.Cursor, 0), lastRow)
	switch match.Action {
	case ActionCursorUp:
		top--
	case ActionCursorDown:
		top++
	case ActionCursorPageUp, ActionScrollBack:
		top -= max(rows-1, 1)
	case ActionCursorPageDown, ActionScrollForward:
		top += max(rows-1, 1)
	case ActionCursorFirstRow:
		top, overlay.List.Offset = 0, 0
	case ActionCursorLastRow:
		top = lastRow
	case ActionCursorLeft, ActionScrollLeft:
		overlay.List.Offset -= diagramScrollStep
	case ActionCursorRight, ActionScrollRight:
		overlay.List.Offset += diagramScrollStep
	default:
		return false
	}
	overlay.List.Cursor = min(max(top, 0), lastRow)
	overlay.List.Offset = min(max(overlay.List.Offset, 0), lastColumn)
	return true
}

// diagramScrollStep is how many columns one press moves a diagram.
const diagramScrollStep = 8

// showDiagram draws the relation and the relations a foreign key joins to it. Every read
// happens off the frame, so the card opens with a line that says it is being read.
func (model *Model) showDiagram(
	connection *app.Connection, table db.TableRef,
) (tea.Model, tea.Cmd) {
	connection.Open(app.Overlay{
		Kind: app.OverlayMessage, Title: " diagram ", Body: "reading the catalog…",
	})
	return model, readDiagram(
		model.ActiveID(), connection.Session, table, connection.Catalog.Tables)
}

// readDiagramAnswer draws the diagram the reads answered, over the line that reports the
// reading, or over the diagram that follows a table.
func (model *Model) readDiagramAnswer(answered diagramMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	held := &connection.Overlay
	opening := held.Kind == app.OverlayMessage && held.Title == " diagram "
	following := held.Kind == app.OverlayDiagram && held.Following == answered.Title
	// A user who closed the card while the reads ran is not shown it again.
	if !opening && !following {
		return model, nil
	}
	if answered.Problem != "" {
		if following {
			held.Following, held.Notice = "", answered.Problem
			return model, nil
		}
		connection.Open(app.Overlay{
			Kind: app.OverlayMessage, Title: " diagram failed ", Body: answered.Problem,
		})
		return model, nil
	}
	trail := []app.DiagramStep{}
	if following {
		trail = append(held.Trail, app.DiagramStep{
			Title: held.Title, Diagram: held.Diagram, Field: held.Field, List: held.List,
		})
	}
	drawn := present.RenderErDiagram(answered.Root, answered.Related, present.DiagramMarks{
		Primary: model.icons.Icon(cfg.IconPrimaryKey),
		Foreign: model.icons.Icon(cfg.IconForeignKey),
	})
	connection.Open(app.Overlay{
		Kind: app.OverlayDiagram, Title: " diagram · " + answered.Title + " ",
		Diagram: drawn, Field: drawn.Root, Trail: trail,
		Notice: describeDiagramLinks(drawn),
	})
	return model, nil
}

// describeDiagramLinks returns the readout of a diagram with no foreign key, or nothing.
func describeDiagramLinks(drawn present.ErDiagram) string {
	if len(drawn.Links) == 0 {
		return "no foreign keys"
	}
	return ""
}

// runDiagramAction runs a key of the diagram that is not a scroll. It reports whether the
// action belonged to the diagram.
func (model *Model) runDiagramAction(
	connection *app.Connection, overlay *app.Overlay, match Match,
) (bool, tea.Model, tea.Cmd) {
	switch match.Action {
	case ActionPreviousTable:
		model.focusDiagramBox(overlay, overlay.Field-1)
	case ActionNextTable:
		model.focusDiagramBox(overlay, overlay.Field+1)
	case ActionFollowTable:
		return true, model, model.followDiagramBox(connection, overlay)
	case ActionCopyValue:
		lines := make([]string, 0, len(overlay.Diagram.Lines))
		for _, line := range overlay.Diagram.Lines {
			lines = append(lines, strings.TrimRight(line, " "))
		}
		overlay.Notice = "copied"
		return true, model, model.keepOnClipboard(strings.Join(lines, "\n") + "\n")
	case ActionClose:
		if len(overlay.Trail) == 0 {
			return false, model, nil
		}
		step := overlay.Trail[len(overlay.Trail)-1]
		overlay.Trail = overlay.Trail[:len(overlay.Trail)-1]
		overlay.Title, overlay.Diagram = step.Title, step.Diagram
		overlay.Field, overlay.List = step.Field, step.List
		overlay.Following, overlay.Notice = "", describeDiagramLinks(step.Diagram)
	default:
		return false, model, nil
	}
	return true, model, nil
}

// focusDiagramBox moves the focus to that table of the diagram, wrapping at both ends, and
// pans the diagram until the whole box is on screen.
func (model *Model) focusDiagramBox(overlay *app.Overlay, index int) {
	boxes := overlay.Diagram.Boxes
	if len(boxes) == 0 {
		return
	}
	overlay.Field = wrap(index, len(boxes))
	box := boxes[overlay.Field]
	rows, columns := model.layout.cardBody, model.layout.cardRoom
	height := len(overlay.Diagram.Lines)
	width := overlay.Diagram.Width
	overlay.List.Cursor = scrollTo(box.Y+box.Height-1, overlay.List.Cursor, rows, height)
	overlay.List.Cursor = scrollTo(box.Y, overlay.List.Cursor, rows, height)
	overlay.List.Offset = scrollTo(box.X+box.Width-1, overlay.List.Offset, columns, width)
	overlay.List.Offset = scrollTo(box.X, overlay.List.Offset, columns, width)
}

// followDiagramBox reads the diagram of the focused table. The diagram on show stays until
// the reads answer.
func (model *Model) followDiagramBox(
	connection *app.Connection, overlay *app.Overlay,
) tea.Cmd {
	if overlay.Field < 0 || overlay.Field >= len(overlay.Diagram.Boxes) ||
		overlay.Field == overlay.Diagram.Root {
		return nil
	}
	box := overlay.Diagram.Boxes[overlay.Field]
	table, known := connection.Catalog.FindTable(box.Schema, box.Name)
	if !known {
		overlay.Notice = "table not found in the catalog: " + box.Name
		return nil
	}
	overlay.Following = present.QualifyDiagramTable(table.Schema, table.Name)
	overlay.Notice = "reading " + overlay.Following + "…"
	return readDiagram(model.ActiveID(), connection.Session, table, connection.Catalog.Tables)
}

// openDiagramBox opens the focused table of the diagram in a tab of its own.
func (model *Model) openDiagramBox(
	connection *app.Connection, overlay *app.Overlay,
) (tea.Model, tea.Cmd) {
	if overlay.Field < 0 || overlay.Field >= len(overlay.Diagram.Boxes) {
		return model, nil
	}
	box := overlay.Diagram.Boxes[overlay.Field]
	table, known := connection.Catalog.FindTable(box.Schema, box.Name)
	if !known {
		connection.Show("table not found in the catalog: " + box.Name)
		return model, nil
	}
	connection.CloseEveryOverlay()
	preview := connection.Session.Composer().ComposeRelationRead(
		table, core.ReadRewrite{}).Display
	return model.runTabRead(connection, connection.OpenTable(table, preview))
}
