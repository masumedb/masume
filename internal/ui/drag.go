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
	dragSideDivider
	dragTab
	dragColumnHeader
	dragCell
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
	// True for a press on a column name with Shift, which adds the column to the sort on a
	// release that never moved.
	addsSort bool
	// True once the pointer moved while it drags a tab or a column name, which then draws
	// lifted.
	lifted bool
	// The tab being dragged, the tab that had the focus before the press, and the split view
	// as it stood then. A drop on a side of the panes is read against them.
	dragged     tabKey
	previous    tabKey
	hasPrevious bool
	splitBefore splitView
	// True while the pointer stands on a side a dropped tab lands on, and that side.
	dropping bool
	dropSide int
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

func (drag *pointerDrag) takeSideDivider(grab int) {
	*drag = pointerDrag{kind: dragSideDivider, lineGrab: grab}
}

func (drag *pointerDrag) takeColumnEdge(column, width, from int) {
	*drag = pointerDrag{
		kind: dragColumnEdge, column: column, columnWidth: width, columnFrom: from,
	}
}

func (drag *pointerDrag) takeTab(dragged, previous tabKey, hasPrevious bool, before splitView) {
	*drag = pointerDrag{
		kind: dragTab, dragged: dragged, previous: previous, hasPrevious: hasPrevious,
		splitBefore: before,
	}
}

func (drag *pointerDrag) takeColumnHeader(column int, addsSort bool) {
	*drag = pointerDrag{kind: dragColumnHeader, column: column, addsSort: addsSort}
}

func (drag *pointerDrag) takeCell() {
	*drag = pointerDrag{kind: dragCell}
}

func (drag *pointerDrag) stop() { *drag = pointerDrag{} }
