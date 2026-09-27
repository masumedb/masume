package ui

import (
	"slices"
	"testing"

	"github.com/masumedb/masume/internal/app"
)

// readMenuIDs returns the ids of the entries of a menu.
func readMenuIDs(actions []app.MenuAction) []string {
	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		ids = append(ids, action.ID)
	}
	return ids
}

func TestTheGridMenuOffersToFollowOnlyAForeignKey(t *testing.T) {
	model, connection, tab := openOrdersRowCard(t, false)
	connection.CloseEveryOverlay()

	tab.GridColumn = 0
	onID := readMenuIDs(model.buildGridMenu(connection, tab, model.buildGridShape(connection, tab)))
	if slices.Contains(onID, string(ActionFollowForeignKey)) {
		t.Errorf("the menu of id offers to follow a key: %v", onID)
	}
	tab.GridColumn = 1
	onKey := readMenuIDs(model.buildGridMenu(connection, tab, model.buildGridShape(connection, tab)))
	if !slices.Contains(onKey, string(ActionFollowForeignKey)) {
		t.Errorf("the menu of customer_id does not offer to follow its key: %v", onKey)
	}
}

func TestAMenuCountsItsActionsWithoutTheDivider(t *testing.T) {
	actions := []app.MenuAction{{ID: "a"}, {Divider: true}, {ID: "b"}}
	if count := countMenuActions(actions); count != 2 {
		t.Errorf("the menu counts %d actions", count)
	}
}
