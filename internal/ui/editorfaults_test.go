package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query/editor"
)

// buildScannedModel answers a model whose editor holds a statement over a catalog that was
// read, which is what the scanner checks a buffer against.
func buildScannedModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
		{Schema: "public", Name: "customers", Kind: db.RelationTable},
	}
	connection.Catalog.ReadAt = time.Unix(1_700_000_000, 0)
	connection.Catalog.Details = map[string]present.TableDetailState{
		present.BuildTableID(connection.Catalog.Tables[0]): {
			Kind: present.DetailReady,
			Detail: db.TableDetail{
				Table: connection.Catalog.Tables[0],
				Columns: []db.ColumnDetail{
					{Name: "id", DataType: "integer", IsPrimaryKey: true},
					{Name: "placed_at", DataType: "timestamp"},
				},
			},
		},
	}
	tab.Editor = app.NewEditorBuffer("select o.id, o.placed_at from public.orders as o", 0)
	return model, connection, tab
}

// markedFault is laid over the faults that were kept. A scan of the buffer writes what the
// scanner found, so the mark is gone wherever the buffer was scanned again.
const markedFault = "marked"

// markKeptFaults lays the mark over the faults the caches hold.
func markKeptFaults(model *Model, key tabKey) {
	held, found := model.caches.readFaults(key)
	if !found {
		return
	}
	held.faults = []editor.Diagnostic{{Message: markedFault}}
	model.caches.keepFaults(key, held)
}

// holdsMarkedFaults is true while the faults the caches hold are still the marked ones.
func holdsMarkedFaults(model *Model, key tabKey) bool {
	held, found := model.caches.readFaults(key)
	return found && len(held.faults) == 1 && held.faults[0].Message == markedFault
}

// The fault row, the marks over the text and the gutter each read the faults, so one frame
// scanned the buffer three times. It is scanned once and kept.
func TestEditorFaultsAreKeptWhileNothingChanges(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	model.findDiagnostics(connection, tab)
	key := model.buildTabKey(connection, tab)
	markKeptFaults(model, key)

	for range 20 {
		model.findDiagnostics(connection, tab)
	}
	if !holdsMarkedFaults(model, key) {
		t.Error("the buffer was scanned again although nothing changed")
	}
}

// A whole frame reads the faults from three places. The first read keeps them, so the two
// after it read what was kept rather than scanning the buffer again.
func TestOneFrameKeepsTheFaultsItScanned(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	model.View()

	key := model.buildTabKey(connection, tab)
	held, kept := model.caches.readFaults(key)
	if !kept || !held.found || held.text != tab.Editor.Text {
		t.Fatal("the frame kept no faults, so every reader of them scanned the buffer")
	}
	markKeptFaults(model, key)
	model.View()
	if !holdsMarkedFaults(model, key) {
		t.Error("a frame scanned the buffer again although the faults were kept")
	}
}

// One test per input the faults are found from. A cache that misses one of these reports a
// fault against a catalog the client no longer holds, or hides one it does.
func TestEditorFaultsAreFoundAgainForEveryChange(t *testing.T) {
	cases := []struct {
		name   string
		change func(connection *app.Connection, tab *app.Tab)
	}{
		{"the buffer changed", func(_ *app.Connection, tab *app.Tab) {
			tab.Editor = app.NewEditorBuffer("select * from public.customers", 0)
		}},
		{"the catalog was read again", func(connection *app.Connection, _ *app.Tab) {
			connection.Catalog.ReadAt = connection.Catalog.ReadAt.Add(time.Minute)
		}},
		{"a relation was added to the catalog", func(
			connection *app.Connection, _ *app.Tab,
		) {
			connection.Catalog.Tables = append(connection.Catalog.Tables,
				db.TableRef{Schema: "public", Name: "roles", Kind: db.RelationTable})
		}},
		{"the detail of a relation was asked for", func(
			connection *app.Connection, _ *app.Tab,
		) {
			id := present.BuildTableID(connection.Catalog.Tables[1])
			connection.Catalog.Details[id] = present.TableDetailState{
				Kind: present.DetailLoading,
			}
		}},
		// The count of the details does not move when one of them turns from loading to
		// read, and the count of the columns does not move when one is renamed. A key
		// built from counts alone would miss both.
		{"the detail of a relation turned from loading to read", func(
			connection *app.Connection, _ *app.Tab,
		) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Kind = present.DetailLoading
			connection.Catalog.Details[id] = held
		}},
		{"a column of a relation was renamed", func(
			connection *app.Connection, _ *app.Tab,
		) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Detail.Columns[1].Name = "created_at"
			connection.Catalog.Details[id] = held
		}},
		{"the type of a column changed", func(connection *app.Connection, _ *app.Tab) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Detail.Columns[1].DataType = "date"
			connection.Catalog.Details[id] = held
		}},
		{"a column became a key", func(connection *app.Connection, _ *app.Tab) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Detail.Columns[1].IsPrimaryKey = true
			connection.Catalog.Details[id] = held
		}},
		{"the read of a relation failed", func(connection *app.Connection, _ *app.Tab) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Kind, held.Message = present.DetailFailed, "no rights on it"
			connection.Catalog.Details[id] = held
		}},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildScannedModel(t)
			model.findDiagnostics(connection, tab)
			key := model.buildTabKey(connection, tab)
			markKeptFaults(model, key)

			held.change(connection, tab)
			model.findDiagnostics(connection, tab)
			if holdsMarkedFaults(model, key) {
				t.Error("the faults were kept although " + held.name)
			}
		})
	}
}

// A cache that scans again and answers what it held before passes a test that only counts the
// scans, so the faults themselves are read: a column the relation does not hold is reported,
// and the report goes once the column is named right.
func TestEditorFaultsFollowTheBuffer(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	if faults := model.findDiagnostics(connection, tab); len(faults) > 0 {
		t.Fatalf("a statement over the columns of the relation was faulted: %v", faults)
	}

	tab.Editor = app.NewEditorBuffer("select o.nothing_here from public.orders as o", 0)
	found := model.findDiagnostics(connection, tab)
	if len(found) == 0 {
		t.Fatal("a column the relation does not hold was not reported")
	}
	if !strings.Contains(strings.ToLower(found[0].Message), "nothing_here") {
		t.Errorf("the fault reads %q, and does not name the column", found[0].Message)
	}

	tab.Editor = app.NewEditorBuffer("select o.placed_at from public.orders as o", 0)
	if faults := model.findDiagnostics(connection, tab); len(faults) > 0 {
		t.Errorf("the fault is still reported with the column named right: %v", faults)
	}
}

// The faults are kept per tab, so the buffer of one tab never answers for another.
func TestEditorFaultsAreKeptPerTab(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	model.findDiagnostics(connection, tab)

	other := connection.OpenQueryTab("select o.nothing_here from public.orders as o")
	if other == tab {
		t.Fatal("the second tab is the first one")
	}

	markKeptFaults(model, model.buildTabKey(connection, tab))
	found := model.findDiagnostics(connection, other)
	if len(found) == 1 && found[0].Message == markedFault {
		t.Error("the second tab answered with the faults of the first")
	}
	if len(found) == 0 {
		t.Error("the buffer of the second tab was not faulted")
	}
}

// The keys that step through the faults and the row that reports one read them in the order
// they stand in the statement, whatever order the server named them in.
func TestTheFaultsOfTheServerAreKeptInTheOrderOfTheStatement(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	tab.Editor = app.NewEditorBuffer("select 1;\nselect 2;\nselect 3;", 0)

	model.readChecked(checkedMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, SQL: tab.Editor.Text,
		Found: []editor.Diagnostic{
			{Start: 20, End: 21, Message: "third"},
			{Start: 0, End: 1, Message: "first"},
			{Start: 10, End: 11, Message: "second"},
		},
	})

	messages := []string{}
	for _, fault := range tab.Served.Found {
		messages = append(messages, fault.Message)
	}
	if strings.Join(messages, " ") != "first second third" {
		t.Errorf("the faults read %v", messages)
	}
}

// The next problem key reaches the fault after the caret, and wraps at the end.
func TestTheNextProblemKeyStepsThroughTheFaultsInOrder(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	tab.Editor = app.NewEditorBuffer("select 1;\nselect 2;\nselect 3;", 0)
	model.readChecked(checkedMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, SQL: tab.Editor.Text,
		Found: []editor.Diagnostic{
			{Start: 20, End: 21, Message: "third"},
			{Start: 10, End: 11, Message: "second"},
		},
	})

	model.stepProblem(connection, tab)
	if tab.Editor.Caret != 10 {
		t.Errorf("the caret stands at %d, wanted the first fault at 10", tab.Editor.Caret)
	}
	model.stepProblem(connection, tab)
	if tab.Editor.Caret != 20 {
		t.Errorf("the caret stands at %d, wanted the second fault at 20", tab.Editor.Caret)
	}
	if connection.Notice == nil || connection.Notice.Text != "2 of 2 errors" {
		t.Errorf("the notice reads %v, wanted 2 of 2 errors", connection.Notice)
	}
}

// The title counts the faults as errors.
func TestTheEditorTitleCountsTheErrors(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	for faults, wanted := range map[int]string{
		0: " query ", 1: " query · 1 error ", 2: " query · 2 errors ",
	} {
		if title := model.describeEditorTitle(tab, faults); title != wanted {
			t.Errorf("the title reads %q, wanted %q", title, wanted)
		}
	}
}

// typeInEditor writes the text into the editor one key at a time.
func typeInEditor(model *Model, text string) {
	for _, character := range text {
		model.readWorkspaceKey(tea.Key{Code: character, Text: string(character)})
	}
}

// The word under the caret is still being written, so a fault on it waits until the caret
// leaves the word or the typing stops.
func TestAFaultOnTheWordBeingTypedWaitsForTheTypingToStop(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	written := "select o.id from public.orders as o join cus"
	tab.Editor = app.NewEditorBuffer(written, len(written))

	typeInEditor(model, "t")
	if strings.Contains(model.View().Content, "unknown table") {
		t.Error("the fault on the word being typed was shown")
	}

	model.readCheckDue(checkDueMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, SQL: tab.Editor.Text,
	})
	if !strings.Contains(model.View().Content, "unknown table: cust") {
		t.Error("the fault was not shown after the typing stopped")
	}

	typeInEditor(model, "x")
	model.readWorkspaceKey(tea.Key{Code: tea.KeyLeft})
	if strings.Contains(model.View().Content, "unknown table") {
		t.Error("the fault was shown with the caret still in the word")
	}
	for range len("custx") {
		model.readWorkspaceKey(tea.Key{Code: tea.KeyLeft})
	}
	if !strings.Contains(model.View().Content, "unknown table: custx") {
		t.Error("the fault was not shown after the caret left the word")
	}
}

// A fault away from the caret is shown while the typing goes on.
func TestAFaultAwayFromTheCaretIsShownWhileTyping(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	written := "select o.nothing_here from public.orders as o where o.i"
	tab.Editor = app.NewEditorBuffer(written, len(written))

	typeInEditor(model, "d")
	if !strings.Contains(model.View().Content, "nothing_here") {
		t.Error("the fault away from the caret was held back")
	}
}
