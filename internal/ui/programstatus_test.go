package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

func TestProgramStatusIsIdleWithNothingToDo(t *testing.T) {
	model := buildOfflineModel(t, 100, 30)
	status := model.resolveProgramStatus()
	if status == nil || status.State != tea.ProgramStateIdle || status.App != "masume" ||
		status.Title != "offline" {
		t.Errorf("the status is %+v", status)
	}
}

func TestProgramStatusIsWorkingWhileAStatementRuns(t *testing.T) {
	model := buildOfflineModel(t, 100, 30)
	tab := model.Active().Active()
	tab.Results.Start([]string{"select 1"}, 100)
	status := model.resolveProgramStatus()
	if status.State != tea.ProgramStateWorking || status.Message != "running "+tab.Label() {
		t.Errorf("the status is %+v", status)
	}
}

func TestProgramStatusIsBlockedOnAWriteConfirmation(t *testing.T) {
	model := buildOfflineModel(t, 100, 30)
	model.Active().Open(app.Overlay{Kind: app.OverlayConfirm, Title: " run the write "})
	status := model.resolveProgramStatus()
	if status.State != tea.ProgramStateBlocked || status.Kind != tea.ProgramStatusKindPermission ||
		status.Message != "run the write" {
		t.Errorf("the status is %+v", status)
	}
}

func TestProgramStatusKeepsTheOutcomeUntilAKeyPress(t *testing.T) {
	model := buildOfflineModel(t, 100, 30)
	connection := model.Active()
	tab := connection.Active()
	tab.Results.Start([]string{"select 1"}, 100)
	tab.Results.Results()[0].StartedAt = time.Now().Add(-2 * time.Second)
	tab.Results.Succeed(0, db.ComposedRead{}, db.QueryResult{})
	model.keepRunOutcome(connection, tab, runSucceeded)

	if status := model.resolveProgramStatus(); status.State != tea.ProgramStateDone {
		t.Fatalf("the status is %+v", status)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if status := model.resolveProgramStatus(); status.State != tea.ProgramStateIdle {
		t.Errorf("after a key press the status is %+v", status)
	}
}

func TestProgramStatusIsSentThroughTmuxOnlyWhenItChanges(t *testing.T) {
	inside := insideTmux
	insideTmux = true
	t.Cleanup(func() { insideTmux = inside })

	model := buildOfflineModel(t, 100, 30)
	if model.reportProgramStatus() == nil {
		t.Fatal("the first status was not sent")
	}
	if model.reportProgramStatus() != nil {
		t.Error("an unchanged status was sent again")
	}
	model.quitting = true
	if model.reportProgramStatus() == nil || model.reported != nil {
		t.Error("quitting did not clear the record")
	}
}
