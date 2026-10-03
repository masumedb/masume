package ui

import (
	"strings"

	"github.com/masumedb/masume/internal/app"
)

// dropTint is the weight a drop zone mixes its ground toward the accent with.
const dropTint = 0.18

// planDropZones returns the cells of the two sides a dragged tab can land on: the sides of
// the split view, or the two halves of the panes where the split view is closed. It reports
// whether the panes have room for two sides.
func (model *Model) planDropZones() ([2]sideRect, sideArrangement, bool) {
	layout := model.layout
	if layout.arrangement != arrangeSingle {
		return layout.sideRects, layout.arrangement, true
	}
	area := layout.paneArea
	across := model.width >= 2*narrowestPaneWidth
	stacked := area.height >= 2*sideFloorRows
	arrangement := arrangeSingle
	switch {
	case model.split.stacked && stacked:
		arrangement = arrangeStacked
	case across:
		arrangement = arrangeAcross
	case stacked:
		arrangement = arrangeStacked
	}
	if arrangement == arrangeSingle || area.width < 1 {
		return [2]sideRect{}, arrangeSingle, false
	}
	return model.planSides(arrangement, area.left, area.top, area.width, area.height),
		arrangement, true
}

// findDropSide returns the side a tab dragged to that cell lands on, and whether the cell is
// in a drop zone.
func (model *Model) findDropSide(x, y int) (int, bool) {
	zones, _, room := model.planDropZones()
	if !room {
		return 0, false
	}
	for side, zone := range zones {
		if x >= zone.left && x < zone.left+zone.width && y >= zone.top &&
			y < zone.top+zone.height {
			return side, true
		}
	}
	return 0, false
}

// describeDropSide returns the name of a side: left or right, or top or bottom.
func (model *Model) describeDropSide(side int) string {
	_, arrangement, _ := model.planDropZones()
	names := [2]string{"left", "right"}
	if arrangement == arrangeStacked {
		names = [2]string{"top", "bottom"}
	}
	return names[side]
}

// describeTabDrop returns the bar text while a tab is dragged.
func (model *Model) describeTabDrop() string {
	if !model.drag.dropping {
		return "release to drop the tab here"
	}
	side := model.describeDropSide(model.drag.dropSide)
	if model.drag.splitBefore.open {
		return "release to show the tab on the " + side
	}
	return "release to split, with the tab on the " + side
}

// paintDropZone tints the side a dragged tab would land on.
func (model *Model) paintDropZone(frame string) string {
	if !model.drag.holds(dragTab) || !model.drag.dropping {
		return frame
	}
	zones, _, room := model.planDropZones()
	if !room {
		return frame
	}
	zone := zones[model.drag.dropSide]
	rows := strings.Split(frame, "\n")
	for row := zone.top; row < zone.top+zone.height && row < len(rows); row++ {
		cells := mapCells(rows[row])
		model.tintRowCells(cells, zone.left, zone.left+zone.width-1,
			model.styles.Theme.Accent, dropTint)
		rows[row] = writeCells(cells)
	}
	return strings.Join(rows, "\n")
}

// placeTabOnSide shows a tab on one side of the split view, which takes the focus. The other
// side keeps the tab it showed before, or takes the tab of this side where it showed the
// moved tab. A closed split view opens with the partner tab on the other side.
func (model *Model) placeTabOnSide(
	connection *app.Connection, moved tabKey, side int, before splitView, partner tabKey,
	hasPartner bool,
) {
	sides := before.sides
	if before.open {
		if sides[1-side] == moved {
			sides[1-side] = before.sides[side]
		}
	} else {
		if !hasPartner || partner == moved {
			partner = model.findPartnerTab(connection, moved)
		}
		sides[1-side] = partner
	}
	sides[side] = moved
	model.split.open, model.split.sides, model.split.focused = true, sides, side
	model.activateKey(moved)
}

// findPartnerTab returns the tab beside the moved one, or a new query tab where the
// connection holds no other.
func (model *Model) findPartnerTab(connection *app.Connection, moved tabKey) tabKey {
	index := connection.IndexOfTab(moved.tab)
	switch {
	case index+1 < len(connection.Tabs):
		return model.buildTabKey(connection, connection.Tabs[index+1])
	case index > 0:
		return model.buildTabKey(connection, connection.Tabs[index-1])
	}
	opened := connection.OpenQueryTab("")
	opened.Focus = app.PaneEditor
	return model.buildTabKey(connection, opened)
}

// dropTab places the dragged tab on the side it was released over.
func (model *Model) dropTab(held pointerDrag) {
	connection := model.Active()
	if connection == nil {
		return
	}
	model.placeTabOnSide(connection, held.dragged, held.dropSide, held.splitBefore,
		held.previous, held.hasPrevious)
}

// moveTabToOtherSide moves the tab with the focus to the other side of the split view.
func (model *Model) moveTabToOtherSide(connection *app.Connection) {
	if !model.split.open {
		connection.Show("no split view")
		return
	}
	moved, found := model.findActiveKey()
	if !found {
		return
	}
	model.placeTabOnSide(connection, moved, model.split.resolveAside(), model.split,
		tabKey{}, false)
}
