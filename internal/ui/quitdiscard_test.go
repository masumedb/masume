package ui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

// Staged changes are lost with the client, so the press that quits must ask first.
func TestQuittingAsksAboutStagedChanges(t *testing.T) {
	model, _, tab := buildTableTabModel(t)
	stageCellEdits(tab, 2)

	model.readKey(pressCtrlC())

	if model.confirm == nil {
		t.Fatal("the client ended without asking about the staged changes")
	}
	if model.quitting {
		t.Error("the client ended before the question was answered")
	}
	if !strings.Contains(model.confirm.Body, "2 changes staged") {
		t.Errorf("the question does not count the changes: %q", model.confirm.Body)
	}
	if !model.confirm.Destructive {
		t.Error("the question is not drawn as one that cannot be taken back")
	}

	model.confirm.Answer(false)
	if model.quitting {
		t.Error("the client ended after the answer that keeps the work")
	}
}

// A notebook transaction is rolled back by the server when the connection closes.
func TestQuittingAsksAboutAnOpenTransaction(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	tab := model.Active().Active()
	tab.Notebook = &app.Notebook{HoldsTransaction: true}

	model.readKey(pressCtrlC())

	if model.confirm == nil {
		t.Fatal("the client ended without asking about the open transaction")
	}
	if !strings.Contains(model.confirm.Body, "1 open transaction") {
		t.Errorf("the question does not name the transaction: %q", model.confirm.Body)
	}
}

// The answer that discards the work leads to the question about the connections that are
// in no config file, and the client ends after both.
func TestDiscardingStagedChangesAsksToSaveTheConnection(t *testing.T) {
	model, _, tab := buildTableTabModel(t)
	stageCellEdits(tab, 1)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))

	model.readKey(pressCtrlC())
	if model.confirm == nil {
		t.Fatal("the client ended without asking about the staged changes")
	}
	held := model.confirm
	model.confirm = nil
	held.Answer(true)

	if model.confirm == nil {
		t.Fatal("the client ended without offering to save the connection")
	}
	if model.quitting {
		t.Error("the client ended before the save question was answered")
	}
}

// A client without staged work and without an unsaved connection ends on the press.
func TestQuittingAsksNothingWithoutUnwrittenWork(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)

	model.readKey(pressCtrlC())

	if model.confirm != nil {
		t.Fatalf("the client asked %q with nothing to lose", model.confirm.Body)
	}
	if !model.quitting {
		t.Error("the client did not end")
	}
}

// committingSession holds an open transaction and records its commit.
type committingSession struct {
	*offlineSession
	committed bool
}

func (session *committingSession) ReadTransactionState() db.TransactionState {
	if session.committed {
		return db.TransactionNone
	}
	return db.TransactionOpen
}

func (session *committingSession) CommitTransaction(context.Context) error {
	session.committed = true
	return nil
}

func TestQuittingOffersToCommitAnOpenTransaction(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	session := &committingSession{offlineSession: connection.Session.(*offlineSession)}
	connection.Session = session

	model.readKey(pressCtrlC())
	if model.confirm == nil || model.quitting {
		t.Fatal("the client ended without asking about the open transaction")
	}
	if !strings.Contains(model.confirm.Body, "1 open transaction") {
		t.Errorf("the question does not name the transaction: %q", model.confirm.Body)
	}
	labels := []string{}
	for _, button := range model.buildConfirmButtons(model.confirm) {
		labels = append(labels, button.label)
	}
	if !slices.Equal(labels, []string{"commit and quit", "roll back and quit", "keep working"}) {
		t.Errorf("the question offers %v", labels)
	}

	_, command := model.readKey(tea.Key{Code: 'l', Mod: uv.ModCtrl})
	if command == nil {
		t.Fatal("the commit key sent nothing")
	}
	model.Update(command())
	if !session.committed || !model.quitting {
		t.Errorf("the commit ran %v and the client quits %v", session.committed, model.quitting)
	}
}
