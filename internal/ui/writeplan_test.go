package ui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/writeplan"
)

// undoingSession records the statements the undo applied.
type undoingSession struct {
	*offlineSession
	applied []db.Change
	problem error
}

func (session *undoingSession) ApplyChanges(_ context.Context, changes []db.Change) error {
	if session.problem != nil {
		return session.problem
	}
	session.applied = append(session.applied, changes...)
	return nil
}

// buildPlannedModel answers a model whose connection measures a write before it runs.
func buildPlannedModel(t *testing.T) (*Model, *app.Connection, *undoingSession) {
	t.Helper()
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	session := &undoingSession{offlineSession: connection.Session.(*offlineSession)}
	session.profile.WritePlan = cfg.PlanUndo
	session.profile.UndoRows = cfg.DefaultUndoRows
	session.capabilities = core.Capabilities{SortsRead: true, PlansWrites: true}
	connection.Session = session
	return model, connection, session
}

// buildTestUndo answers an undo of one row of orders.
func buildTestUndo() writeplan.Undo {
	return writeplan.Undo{
		Table: db.TableRef{Schema: "public", Name: "orders"},
		Rows:  1,
		Changes: []db.Change{{
			Description: "put status of one row of orders back",
			Display:     `update "public"."orders" set "status" = $1 where "id" = $2`,
		}},
		Display: []string{`update "public"."orders" set "status" = 'open' where "id" = 1`},
	}
}

// buildTestPlan answers a measured write over a quarter of a relation.
func buildTestPlan() writeplan.Plan {
	return writeplan.Plan{
		SQL:     "update orders set status = 'sent' where status = 'open'",
		Kind:    "update",
		Table:   db.TableRef{Schema: "public", Name: "orders"},
		Columns: []string{"status"},
		Rows:    3, HasRows: true, Total: 12, HasTotal: true,
		Cascades: []writeplan.Cascade{{Reason: "trigger t_order_audit", Table: "orders"}},
		Undo: writeplan.UndoPlan{
			Kept: true, Rows: 1, Table: db.TableRef{Schema: "public", Name: "orders"},
		},
	}
}

func TestAWriteThatCannotBeMeasuredKeepsThePlainQuestion(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayMessage, Title: measuringPlanTitle, Body: "measuring…",
	}
	held, _ := model.Update(writePlanBuiltMsg{
		ConnectionID: model.ActiveID(), TabID: connection.Active().ID,
		Written: "delete from orders using lines where lines.id = orders.line",
		SQL:     "delete from orders using lines where lines.id = orders.line",
	})
	model = held.(*Model)

	if model.Active().Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the card is %q, wanted the plain question",
			model.Active().Overlay.Kind)
	}
}

// An answer that lands after the reader closed the card is dropped.
func TestThePlanIsDrawnOverTheLineThatSaidItWasMeasured(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	tab := connection.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayMessage, Title: measuringPlanTitle, Body: "measuring…",
	}

	answered := writePlanBuiltMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID,
		Written: buildTestPlan().SQL, SQL: buildTestPlan().SQL,
		Plan: buildTestPlan(), Measured: true,
	}
	held, _ := model.Update(answered)
	model = held.(*Model)
	if model.Active().Overlay.Kind != app.OverlayWritePlan {
		t.Fatalf("the card is %q", model.Active().Overlay.Kind)
	}

	model.Active().Overlay = app.Overlay{}
	held, _ = model.Update(answered)
	model = held.(*Model)
	if model.Active().Overlay.IsOpen() {
		t.Errorf("a closed card was drawn again as %q", model.Active().Overlay.Kind)
	}
}

func TestThePlanCardSaysWhatTheWriteDoes(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayWritePlan, Title: " write plan ", Plan: buildTestPlan(),
	}

	drawn := stripStyles(model.renderWritePlan(connection.Overlay, 100))
	for _, said := range []string{
		"3 of 12 in orders", "status", "t_order_audit", "1 row to capture", "run", "cancel",
	} {
		if !strings.Contains(drawn, said) {
			t.Errorf("the card says nothing of %q:\n%s", said, drawn)
		}
	}
}

func TestTheUndoIsOfferedAndRun(t *testing.T) {
	model, connection, session := buildPlannedModel(t)
	connection.KeepUndo(buildTestUndo(), "update orders set status = 'sent'", time.Now())

	held, _ := model.undoLastWrite(connection)
	model = held.(*Model)
	overlay := model.Active().Overlay
	if overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the card is %q, wanted the question", overlay.Kind)
	}
	if !strings.Contains(overlay.Body, "orders") {
		t.Errorf("the question reads:\n%s", overlay.Body)
	}

	command := overlay.Answers.Answer(true)
	if command == nil {
		t.Fatal("a yes started nothing")
	}
	answer, is := command().(undoWrittenMsg)
	if !is {
		t.Fatalf("the answer is %T", command())
	}
	if answer.Problem != "" {
		t.Fatalf("the undo answered %q", answer.Problem)
	}
	if len(session.applied) != 1 {
		t.Fatalf("the undo applied %d changes", len(session.applied))
	}

	held, _ = model.Update(answer)
	model = held.(*Model)
	if model.Active().Undo != nil {
		t.Error("the undo was kept after it ran")
	}
}

func TestNothingIsUndoneWithoutAWriteThatKeptOne(t *testing.T) {
	model, connection, session := buildPlannedModel(t)
	held, _ := model.undoLastWrite(connection)
	model = held.(*Model)

	if model.Active().Overlay.IsOpen() {
		t.Error("a card was opened with no undo to run")
	}
	if len(session.applied) != 0 {
		t.Error("statements were applied with no undo to run")
	}
}

// A write the server refused reads no undo, so nothing is kept for it. The undo is read
// inside the transaction of the write and answered with it.
func TestAFailedWriteKeepsNoUndo(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	tab := connection.Active()

	held, _ := model.Update(queryRanMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, RunID: 1,
		Read:    db.ComposedRead{Display: "update orders set status = 'sent'"},
		Problem: "the server refused it",
	})
	model = held.(*Model)

	if model.Active().Undo != nil {
		t.Error("a write that failed left an undo")
	}
}

// buildBlockedPlan returns a delete that rows of public.order_items block.
func buildBlockedPlan() writeplan.Plan {
	plan := buildTestPlan()
	plan.Kind = "delete"
	plan.SQL = "delete from orders where status = 'open'"
	plan.Blockers = []writeplan.Cascade{{
		Reason: "on delete no action", Table: "public.order_items", Rows: 3000, HasRows: true,
		Relation:    db.TableRef{Schema: "public", Name: "order_items"},
		Referencing: `"order_id" in (select "id" from "public"."orders")`,
	}}
	return plan
}

// readCardButtons returns the actions of the buttons on this frame row, left to right.
func readCardButtons(model *Model, row int) []ActionID {
	actions := []ActionID{}
	for _, held := range model.layout.buttons {
		if held.row == row {
			actions = append(actions, held.action)
		}
	}
	return actions
}

func TestABlockedWriteLeadsWithTheBlockingRows(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayWritePlan, Title: " write plan ", Plan: buildBlockedPlan(),
	}
	frame := strings.Split(model.render(), "\n")
	drawn := stripStyles(strings.Join(frame[:model.layout.hintRow], "\n"))

	for _, said := range []string{
		"This delete will fail",
		"3,000 rows in public.order_items still reference these rows (on delete no action).",
		"show the blocking rows", "run anyway",
	} {
		if !strings.Contains(drawn, said) {
			t.Errorf("the card says nothing of %q:\n%s", said, drawn)
		}
	}
	if strings.Count(drawn, "cancel") != 1 {
		t.Errorf("the card shows cancel %d times:\n%s", strings.Count(drawn, "cancel"), drawn)
	}

	show, found := findCardButton(model, ActionChooseRow)
	if !found {
		t.Fatal("the card has no button that shows the blocking rows")
	}
	got := readCardButtons(model, show.row)
	wanted := []ActionID{ActionChooseRow, ActionAnswerYes, ActionClose}
	if !slices.Equal(got, wanted) {
		t.Errorf("the buttons are %v, wanted %v", got, wanted)
	}
}

func TestAPlanWithoutABlockerLeadsWithRun(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayWritePlan, Title: " write plan ", Plan: buildTestPlan(),
	}
	model.render()

	run, found := findCardButton(model, ActionAnswerYes)
	if !found {
		t.Fatal("the card has no run button")
	}
	got := readCardButtons(model, run.row)
	if !slices.Equal(got, []ActionID{ActionAnswerYes, ActionClose}) {
		t.Errorf("the buttons are %v", got)
	}
}

func TestEnterOpensTheRowsThatBlockTheWrite(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
		{Schema: "public", Name: "order_items", Kind: db.RelationTable},
	}
	plan := buildBlockedPlan()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayWritePlan, Title: " write plan ", Plan: plan,
	}
	model.render()

	model.readKey(tea.Key{Code: tea.KeyEnter})

	if connection.Overlay.IsOpen() {
		t.Fatalf("the card %q is still open", connection.Overlay.Kind)
	}
	tab := connection.Active()
	if tab.Kind != app.TabTable || tab.Table.Name != "order_items" {
		t.Fatalf("the active tab is %q %q", tab.Kind, tab.Table.Name)
	}
	if len(tab.Filter) != 1 || tab.Filter[0].Text != plan.Blockers[0].Referencing {
		t.Errorf("the tab filters with %+v", tab.Filter)
	}
}

// buildStagedTab returns the tab of a planned model with one cell of orders staged.
func buildStagedTab(t *testing.T) (*Model, *app.Connection, *app.Tab, *undoingSession) {
	t.Helper()
	model, connection, session := buildPlannedModel(t)
	tab := connection.Active()
	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from orders", Display: "select * from orders"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"}, {Name: "status", DataType: "text"},
			},
			Rows: [][]any{{int64(1), "open"}},
		})
	tab.Target = app.EditTarget{
		Table:    db.TableRef{Schema: "public", Name: "orders"},
		Editable: true, KeyColumns: []string{"id"},
	}
	tab.StageChange(func(pending *core.PendingChanges) {
		pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
			RowIndex: 0, ColumnIndex: 1, Value: core.CellValue{Kind: core.CellText, Text: "sent"},
		}
	})
	return model, connection, tab, session
}

func TestStagedChangesOnAPlannedProfileAreMeasuredFirst(t *testing.T) {
	model, connection, tab, session := buildStagedTab(t)

	_, command := model.applyStagedChanges(connection, tab)
	if connection.Overlay.Kind != app.OverlayMessage ||
		connection.Overlay.Title != measuringPlanTitle || command == nil {
		t.Fatalf("the apply opened %q and measures %v", connection.Overlay.Kind, command != nil)
	}
	if tab.Applying || len(session.applied) != 0 {
		t.Error("the staged changes were written before the plan")
	}
}

func TestTheStagedPlanAppliesTheChangesOnYes(t *testing.T) {
	model, connection, tab, session := buildStagedTab(t)
	_, measure := model.applyStagedChanges(connection, tab)
	built, is := measure().(stagedPlanBuiltMsg)
	if !is {
		t.Fatalf("the measure answered %T", built)
	}
	built.Plans, built.Measured = []writeplan.Plan{buildTestPlan()}, true

	held, _ := model.Update(built)
	model = held.(*Model)
	if connection.Overlay.Kind != app.OverlayWritePlan {
		t.Fatalf("the card is %q, wanted the write plan", connection.Overlay.Kind)
	}
	command := connection.Overlay.Answers.Answer(true)
	if command == nil {
		t.Fatal("yes on the plan sent nothing")
	}
	model.Update(command())
	if len(session.applied) != 1 || core.CountChanges(tab.Pending) != 0 {
		t.Errorf("the plan applied %d changes and left %d staged",
			len(session.applied), core.CountChanges(tab.Pending))
	}
}

func TestStagedChangesThatCannotBeMeasuredAreApplied(t *testing.T) {
	model, connection, tab, session := buildStagedTab(t)
	_, measure := model.applyStagedChanges(connection, tab)
	built := measure().(stagedPlanBuiltMsg)
	built.Measured = false

	_, command := model.Update(built)
	if command == nil || !tab.Applying {
		t.Fatal("changes that cannot be measured were not applied")
	}
	model.Update(command())
	if len(session.applied) != 1 {
		t.Errorf("%d changes were applied", len(session.applied))
	}
}
