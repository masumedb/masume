package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

// openStagedReview opens the review of one staged change of orders.
func openStagedReview(t *testing.T) (*Model, *app.Connection) {
	t.Helper()
	model := buildOfflineModel(t, 160, 40)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayChanges,
		Changes: []db.Change{{
			Description: "update orders set total = 500 where id = 1",
			Display:     `update "public"."orders" set "total" = $1 where "id" = $2`,
			Params:      []any{"500", int64(1)},
		}},
	}
	return model, connection
}

func TestTheStagedReviewShowsTheStatementOnce(t *testing.T) {
	model, _ := openStagedReview(t)
	screen := stripEscapes(model.render())
	if !strings.Contains(screen, "update orders set total = 500 where id = 1") {
		t.Errorf("the review does not show the change:\n%s", screen)
	}
	if strings.Contains(screen, "parameters:") || strings.Contains(screen, `"public"."orders"`) {
		t.Errorf("the review shows the statement twice:\n%s", screen)
	}
}

func TestTheStagedReviewShowsTheStatementsOnS(t *testing.T) {
	model, _ := openStagedReview(t)
	model.render()
	model.readKey(tea.Key{Code: 's', Text: "s"})
	screen := stripEscapes(model.render())
	if !strings.Contains(screen, `parameters: "500", 1`) || !strings.Contains(screen, "hide SQL") {
		t.Errorf("s did not show the statements:\n%s", screen)
	}
}

func TestTheStagedReviewHasButtons(t *testing.T) {
	model, _ := openStagedReview(t)
	model.render()
	apply, found := findCardButton(model, ActionApplyChanges)
	if !found {
		t.Fatal("the review has no apply button")
	}
	wanted := []ActionID{ActionApplyChanges, ActionDiscardChanges, ActionToggleStatements, ActionClose}
	if got := readCardButtons(model, apply.row); !slices.Equal(got, wanted) {
		t.Errorf("the buttons are %v, wanted %v", got, wanted)
	}
}
