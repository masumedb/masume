package ui

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// waitingWriteSession is a server whose writes and commits wait for a lock until their
// context ends.
type waitingWriteSession struct {
	offlineSession
	state    db.TransactionState
	deadline time.Time
}

func (session *waitingWriteSession) ReadTransactionState() db.TransactionState {
	return session.state
}

func (session *waitingWriteSession) ApplyChanges(ctx context.Context, _ []db.Change) error {
	<-ctx.Done()
	return ctx.Err()
}

func (session *waitingWriteSession) CommitTransaction(ctx context.Context) error {
	session.deadline, _ = ctx.Deadline()
	<-ctx.Done()
	return ctx.Err()
}

func buildWaitingWriteModel(t *testing.T) (*Model, *app.Connection, *waitingWriteSession) {
	t.Helper()
	model := buildOfflineModel(t, 100, 30)
	connection := model.Active()
	session := &waitingWriteSession{
		offlineSession: offlineSession{
			capabilities: core.Capabilities{HasTransactions: true, CancelsByContext: true},
		},
		state: db.TransactionNone,
	}
	connection.Session = session
	connection.Autocommit = true
	return model, connection, session
}

func TestGridSaveRunsUntilTheCancelKey(t *testing.T) {
	model, connection, _ := buildWaitingWriteModel(t)
	tab := connection.Active()

	command := applyChanges(connection, model.ActiveID(), tab, []db.Change{{}}, nil, "")
	if text, _ := model.describeStatus(connection, tab); text != "running "+saveWriteLabel {
		t.Errorf("the bar shows %q during the save", text)
	}
	answers := make(chan any, 1)
	go func() { answers <- command() }()

	_, cancel := model.cancelQuery(connection)
	if stopped := cancel().(cancelledMsg); !stopped.Stopped {
		t.Error("the cancel key found nothing to stop")
	}
	var answered changesAppliedMsg
	select {
	case held := <-answers:
		answered = held.(changesAppliedMsg)
	case <-time.After(5 * time.Second):
		t.Fatal("the save still waits after the cancel key")
	}
	if answered.Problem != "statement cancelled" {
		t.Errorf("the save answered %q", answered.Problem)
	}
	model.Update(answered)
	if label, writing := connection.FindWrite(); writing {
		t.Errorf("%q still runs after its answer", label)
	}
}

func TestTransactionStepRunsUntilItAnswers(t *testing.T) {
	model, connection, session := buildWaitingWriteModel(t)
	session.state = db.TransactionOpen

	_, command := model.runTransaction(connection, connection.Active(), ActionCommitTransaction)
	if label, _ := connection.FindWrite(); label != "commit" {
		t.Errorf("the running write is %q", label)
	}
	connection.StopRuns()
	answered := command().(transactionRanMsg)
	if answered.Problem != "statement cancelled" {
		t.Errorf("the commit answered %q", answered.Problem)
	}
	model.Update(answered)
	if label, writing := connection.FindWrite(); writing {
		t.Errorf("%q still runs after its answer", label)
	}
}

func TestCommitBeforeQuitHasATimeLimit(t *testing.T) {
	model, connection, session := buildWaitingWriteModel(t)
	session.state = db.TransactionOpen

	command := model.commitBeforeQuit(model.findOpenTransactions())
	answers := make(chan any, 1)
	go func() { answers <- command() }()
	// The quit path stops every run before it closes the sessions.
	connection.StopRuns()
	var answered quitCommittedMsg
	select {
	case held := <-answers:
		answered = held.(quitCommittedMsg)
	case <-time.After(5 * time.Second):
		t.Fatal("the commit still waits after the runs were stopped")
	}
	if session.deadline.IsZero() || session.deadline.After(time.Now().Add(quitCommitTimeout)) {
		t.Errorf("the commit ran with the deadline %v", session.deadline)
	}
	if answered.Problem == "" {
		t.Error("a commit that never ended was reported as done")
	}
	model.Update(answered)
	if label, writing := connection.FindWrite(); writing {
		t.Errorf("%q still runs after its answer", label)
	}
}
