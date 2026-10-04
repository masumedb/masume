package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/writeplan"
)

// startRunOn puts a running statement on the tab of an engine with these capabilities.
func startRunOn(t *testing.T, cancels bool) *Model {
	t.Helper()
	model := buildOfflineModel(t, 120, 30)
	connection := model.Active()
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, CancelsRunningQuery: cancels,
	}
	tab := connection.Active()
	tab.Editor = app.NewEditorBuffer("select 1", 0)
	tab.Results.Start([]string{"select 1"}, 200)
	return model
}

// CockroachDB, PlanetScale and MongoDB take no cancel. The wheel of the
// run shows the note, where the engines that take one show the key.
func TestARunOnAnEngineWithoutCancelShowsTheNote(t *testing.T) {
	frame := stripEscapes(startRunOn(t, false).render())

	if !strings.Contains(frame, noCancelNote) {
		t.Errorf("the frame of the run drew no note for an engine without cancel:\n%s", frame)
	}
	if strings.Contains(frame, "^X stop") {
		t.Errorf("the frame of the run drew a key that stops it:\n%s", frame)
	}
}

// An engine that takes a cancel shows the key beside the wheel, and no note.
func TestARunOnAnEngineWithCancelShowsTheKey(t *testing.T) {
	frame := stripEscapes(startRunOn(t, true).render())

	if !strings.Contains(frame, "^X stop") {
		t.Errorf("the frame of the run drew no key that stops it:\n%s", frame)
	}
	if strings.Contains(frame, noCancelNote) {
		t.Errorf("the frame of the run drew the note beside the key:\n%s", frame)
	}
}

// The bar under the pane shows the cancel key only where the engine takes one.
func TestTheBarShowsTheCancelKeyOnlyWhereTheEngineTakesOne(t *testing.T) {
	if bar := readStatusBar(t, startRunOn(t, true)); !strings.Contains(bar, "stop") {
		t.Errorf("the status bar drew %q, wanted the cancel key", bar)
	}
	if bar := readStatusBar(t, startRunOn(t, false)); strings.Contains(bar, "stop") {
		t.Errorf("the status bar drew %q, wanted no cancel key", bar)
	}
}

func TestCancelOnAContextEngineStopsTheRunContext(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	connection := model.Active()
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, CancelsByContext: true,
	}
	connection.Active().Results.Start([]string{"select 1"}, 200)
	running := connection.RunContext()

	_, command := model.cancelQuery(connection)

	if running.Err() != context.Canceled {
		t.Errorf("the run context reads %v, wanted it cancelled", running.Err())
	}
	if connection.RunContext().Err() != nil {
		t.Error("the next run starts with a cancelled context")
	}
	if answered := command().(cancelledMsg); !answered.Stopped || answered.Problem != "" {
		t.Errorf("the cancel answered %+v, wanted a stop", answered)
	}
	if bar := readStatusBar(t, model); !strings.Contains(bar, "stop") {
		t.Errorf("the status bar drew %q, wanted the cancel key", bar)
	}
}

func TestStopRunsCancelsARunningSqliteStatement(t *testing.T) {
	session := openTransactionSession(t)
	connection := &app.Connection{}
	command := runStatements(connection.RunContext(), 1, 1, 1, 0, session,
		[]db.ComposedRead{{Text: "with recursive n(i) as (select 1 union all " +
			"select i + 1 from n where i < 1000000000) select count(*) from n"}},
		100, writeplan.UndoPlan{}, nil, "test", true)

	answered := make(chan queryRanMsg, 1)
	go func() { answered <- command().(queryRanMsg) }()
	time.Sleep(200 * time.Millisecond)
	connection.StopRuns()

	select {
	case held := <-answered:
		if held.Problem != "statement cancelled" {
			t.Errorf("the run answered %q, wanted the cancel", held.Problem)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the statement still ran 10 seconds after the cancel")
	}
	next := runTransactionStatement(t, session, "select count(*) from entries", true)
	if next.Problem != "" {
		t.Errorf("the next statement answered %q", next.Problem)
	}
}
