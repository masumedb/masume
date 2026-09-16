package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// holdClipboard puts a clipboard of the test in place of the clipboard of the machine.
func holdClipboard(t *testing.T, text string, err error) *string {
	t.Helper()
	written := text
	oldHas, oldRead, oldWrite := hasClipboard, readClipboard, writeClipboard
	hasClipboard = func() bool { return true }
	readClipboard = func() (string, error) { return written, err }
	writeClipboard = func(text string) error {
		written = text
		return nil
	}
	t.Cleanup(func() {
		hasClipboard, readClipboard, writeClipboard = oldHas, oldRead, oldWrite
	})
	return &written
}

// A copy reaches the clipboard of the machine, so another program can paste it.
func TestACopyReachesTheSystemClipboard(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	held := holdClipboard(t, "", nil)

	batch, isBatch := model.keepOnClipboard("select id")().(tea.BatchMsg)
	if !isBatch {
		t.Fatal("the copy returned no batch of commands")
	}
	for _, command := range batch {
		command()
	}
	if *held != "select id" {
		t.Errorf("the system clipboard holds %q", *held)
	}
	if model.clipboard != "select id" {
		t.Errorf("the client kept %q", model.clipboard)
	}
}

// The paste key writes what another program copied, and its line breaks are the breaks of
// the buffer.
func TestThePasteKeyWritesTheSystemClipboard(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "", 0)
	holdClipboard(t, "select id\r\nfrom orders", nil)

	model.pasteIntoEditor(connection, tab)
	if tab.Editor.Text != "select id\nfrom orders" {
		t.Errorf("the paste answers %q", tab.Editor.Text)
	}
}

// A machine with no clipboard tool pastes what this client last copied.
func TestThePasteKeyFallsBackToTheTextThisClientCopied(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "", 0)
	holdClipboard(t, "", errors.New("no clipboard tool"))
	model.clipboard = "select id"

	model.pasteIntoEditor(connection, tab)
	if tab.Editor.Text != "select id" {
		t.Errorf("the paste answers %q", tab.Editor.Text)
	}
}
