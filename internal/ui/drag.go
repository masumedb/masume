package ui

import "github.com/masumedb/masume/internal/app"

type dragKind int

const (
	dragNothing dragKind = iota
	dragScrollbar
	dragEditorText
	dragSplitLine
	dragColumnEdge
	dragTreeEdge
)

type pointerDrag struct {
	kind dragKind
	// The bar being dragged, and where its thumb was taken hold of, in half cells. The
	// pointer may wander off the track and the drag holds, which is what a scroll bar does.
	bar  scrollHit
	grab int
	// True once a drag of a border has moved it at all, because a press that never moved
	// does something else: it hides the result, or it moves the keyboard to a pane.
	moved bool
	// Cells between the pointer and the divider: 1 on the second line of a double border.
	lineGrab int
	// Focus target of a press without motion. Empty on the editor foot: that press toggles
	// the result.
	pane app.Pane
	// The column whose border is being dragged, the width it had when the drag began, and
	// the cell the pointer stood on then.
	column      int
	columnWidth int
	columnFrom  int
}

func (drag pointerDrag) running() bool { return drag.kind != dragNothing }

func (drag pointerDrag) holds(kind dragKind) bool { return drag.kind == kind }

func (drag *pointerDrag) takeScrollbar(bar scrollHit, grab int) {
	*drag = pointerDrag{kind: dragScrollbar, bar: bar, grab: grab}
}

func (drag *pointerDrag) takeEditorText() {
	*drag = pointerDrag{kind: dragEditorText}
}

func (drag *pointerDrag) takeSplitLine(grab int, pane app.Pane) {
	*drag = pointerDrag{kind: dragSplitLine, lineGrab: grab, pane: pane}
}

func (drag *pointerDrag) takeTreeEdge(grab int, pane app.Pane) {
	*drag = pointerDrag{kind: dragTreeEdge, lineGrab: grab, pane: pane}
}

func (drag *pointerDrag) takeColumnEdge(column, width, from int) {
	*drag = pointerDrag{
		kind: dragColumnEdge, column: column, columnWidth: width, columnFrom: from,
	}
}

func (drag *pointerDrag) stop() { *drag = pointerDrag{} }
