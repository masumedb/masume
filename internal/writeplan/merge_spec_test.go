package writeplan_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/writeplan"
)

// transactingSession is a planning session with transactions, and records each step of one.
type transactingSession struct {
	*planningSession
	steps []string
}

func (session *transactingSession) Capabilities() core.Capabilities {
	return core.Capabilities{PlansWrites: true, HasTransactions: true}
}

func (session *transactingSession) ReadTransactionState() db.TransactionState {
	return db.TransactionNone
}

func (session *transactingSession) BeginTransaction(context.Context) error {
	session.steps = append(session.steps, "begin")
	return nil
}

func (session *transactingSession) CommitTransaction(context.Context) error {
	session.steps = append(session.steps, "commit")
	return nil
}

func (session *transactingSession) RollbackTransaction(context.Context) error {
	session.steps = append(session.steps, "rollback")
	return nil
}

func TestMergePlansAddsTheRowsOfWritesOfOneKind(t *testing.T) {
	session := buildOrdersSession()
	first := buildPlan(t, session, "update orders set status = 'sent' where id = 1", cfg.PlanUndo)
	second := buildPlan(t, session, "update orders set total = 1 where id = 2", cfg.PlanUndo)

	merged, is := writeplan.MergePlans([]writeplan.Plan{first, second})
	if !is {
		t.Fatal("two updates of orders did not merge")
	}
	if merged.Rows != first.Rows+second.Rows || merged.Undo.Rows != first.Undo.Rows+second.Undo.Rows {
		t.Errorf("the merged plan counts %d rows and %d to capture", merged.Rows, merged.Undo.Rows)
	}
	if !slices.Equal(merged.Columns, []string{"status", "total"}) {
		t.Errorf("the merged plan assigns %v", merged.Columns)
	}

	removal := buildPlan(t, session, "delete from orders where id = 3", cfg.PlanUndo)
	if _, is := writeplan.MergePlans([]writeplan.Plan{first, removal}); is {
		t.Error("an update and a delete merged into one plan")
	}
}

func TestApplyWithUndoReadsEveryUndoBeforeTheWritesInOneTransaction(t *testing.T) {
	session := &transactingSession{planningSession: buildOrdersSession()}
	first := buildPlan(t, session.planningSession,
		"update orders set status = 'sent' where id = 1", cfg.PlanUndo)
	second := buildPlan(t, session.planningSession,
		"update orders set status = 'sent' where id = 2", cfg.PlanUndo)
	session.asked = nil

	undo, err := writeplan.ApplyWithUndo(context.Background(), session,
		[]writeplan.UndoPlan{first.Undo, second.Undo}, func(context.Context) error {
			session.steps = append(session.steps, "apply")
			return nil
		})
	if err != nil {
		t.Fatalf("the apply answered %v", err)
	}
	if !slices.Equal(session.steps, []string{"begin", "apply", "commit"}) || len(session.asked) != 2 {
		t.Errorf("the steps were %v after %d undo reads", session.steps, len(session.asked))
	}
	if undo.Rows != 6 || !undo.IsHeld() {
		t.Errorf("the undo holds %d rows", undo.Rows)
	}

	session.steps = nil
	_, err = writeplan.ApplyWithUndo(context.Background(), session,
		[]writeplan.UndoPlan{first.Undo}, func(context.Context) error {
			return errors.New("refused")
		})
	if err == nil || !slices.Equal(session.steps, []string{"begin", "rollback"}) {
		t.Errorf("a refused write answered %v after %v", err, session.steps)
	}
}
