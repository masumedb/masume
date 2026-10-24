package ui

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// pointerState is the input state of the pointer: clicks, drags and the shape on show.
type pointerState struct {
	// The clicks in a row on one target, because a terminal reports no double click.
	clicks clickCounter
	// What the pointer was dragged over, which `Ctrl+C` copies.
	selection screenSelection
	drag      pointerDrag
	// The pointer shape last sent to the terminal, or nothing before the first.
	shape string
	// The effect of a drag that runs between frames.
	animation animationState
}

// The pointer shapes the client asks the terminal for, by the names of the CSS cursors.
const (
	pointerDefault  = "default"
	pointerHand     = "pointer"
	pointerText     = "text"
	pointerColumns  = "ew-resize"
	pointerRows     = "ns-resize"
	pointerResizing = "col-resize"
	pointerGrabbing = "grabbing"
)

// resolvePointerShape returns the pointer shape for the cell under the pointer.
func (model *Model) resolvePointerShape(x, y int) string {
	switch model.pointer.drag.kind {
	case dragColumnEdge:
		return pointerResizing
	case dragTreeEdge:
		return pointerColumns
	case dragSplitLine:
		return pointerRows
	case dragSideDivider:
		if model.layout.divider.stacked {
			return pointerRows
		}
		return pointerColumns
	case dragEditorText:
		return pointerText
	case dragScrollbar:
		return pointerDefault
	case dragTab, dragCell:
		return pointerGrabbing
	case dragColumnHeader:
		if model.pointer.drag.moved {
			return pointerGrabbing
		}
		return pointerHand
	}
	if model.frame.isArmed {
		return pointerHand
	}
	if _, onBar := findScrollbar(model.layout.scrollbars, x, y); onBar {
		return pointerDefault
	}
	if _, _, _, onKey := findButton(model.layout.buttons, x, y); onKey {
		return pointerHand
	}
	if model.screen != ScreenWorking || model.confirm != nil {
		return pointerDefault
	}
	connection := model.Active()
	if connection == nil || connection.Overlay.IsOpen() {
		return pointerDefault
	}
	layout := model.layout
	if _, onDivider := layout.divider.holds(x, y); onDivider {
		if layout.divider.stacked {
			return pointerRows
		}
		return pointerColumns
	}
	if _, _, onLine := model.findSplitLine(x, y); onLine {
		return pointerRows
	}
	if _, _, onEdge := model.findTreeEdge(x, y); onEdge {
		return pointerColumns
	}
	if _, onEdge := findColumnEdge(layout.columnEdges, x, y, layout.gridHeaderRow); onEdge {
		return pointerResizing
	}
	if layout.editorTextRows > 0 && x >= layout.editorTextLeft &&
		x < layout.editorTextLeft+layout.editorTextWidth && y >= layout.editorTextTop &&
		y < layout.editorTextTop+layout.editorTextRows && !model.isOnAside(x, y) {
		return pointerText
	}
	return pointerDefault
}

// followPointerShape returns the command that sets the pointer shape, and nothing where the
// shape is the one the terminal already shows.
func (model *Model) followPointerShape() tea.Cmd {
	shape := model.resolvePointerShape(model.frame.pointerX, model.frame.pointerY)
	if shape == model.pointer.shape {
		return nil
	}
	if model.pointer.shape == "" && shape == pointerDefault {
		return nil
	}
	model.pointer.shape = shape
	return tea.Raw(writePointerShape(shape))
}

// writePointerShape returns the OSC 22 sequence that sets the pointer shape. Inside tmux the
// sequence is wrapped for passthrough to the outer terminal.
func writePointerShape(shape string) string {
	sequence := ansi.SetPointerShape(shape)
	if os.Getenv("TMUX") != "" {
		return ansi.TmuxPassthrough(sequence)
	}
	return sequence
}

// ResetPointerShape returns the sequence that puts the default pointer shape back.
func ResetPointerShape() string {
	return writePointerShape(pointerDefault)
}
