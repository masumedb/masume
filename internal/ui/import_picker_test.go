package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/load"
	"github.com/masumedb/masume/internal/query"
)

// The picker offers the files this client can read and no others, so a file it would refuse
// is never chosen by mistake.
func TestBuildFilePickerOffersTheFilesAnImportReads(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	picker := model.buildFilePicker(load.ListFileExtensions())

	for _, extension := range []string{".csv", ".tsv", ".json", ".jsonl", ".ndjson"} {
		if !slices.Contains(picker.Extensions, extension) {
			t.Errorf("the picker refuses %s, which an import reads", extension)
		}
	}
	if slices.Contains(picker.Extensions, ".md") {
		t.Error("the picker offers a file no import reads")
	}
}

// buildPickerDirectory writes a directory with a subdirectory, a hidden file, a file an import
// reads and one it does not.
func buildPickerDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "archive"), 0o700); err != nil {
		t.Fatalf("the directory cannot be made: %v", err)
	}
	for name, text := range map[string]string{
		"orders.csv": "id\n1\n", "notes.md": "# notes\n", ".hidden.csv": "id\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(text), 0o600); err != nil {
			t.Fatalf("the file cannot be written: %v", err)
		}
	}
	return directory
}

// readPickerListing runs the command that lists the directory and applies the listing.
func readPickerListing(t *testing.T, picker filePicker, command tea.Cmd) filePicker {
	t.Helper()
	if command == nil {
		t.Fatal("no directory is read")
	}
	picker, _, _ = picker.Update(command())
	return picker
}

func TestTheFilePickerListsTheParentThenDirectoriesThenImportableFiles(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	picker := newFilePicker(buildPickerDirectory(t), load.ListFileExtensions())
	picker = readPickerListing(t, picker, picker.Init())

	lines := model.buildPickerLines(&picker, 60)
	shown := []string{}
	for _, line := range lines[2:] {
		if text := strings.TrimSpace(stripEscapes(line)); text != "" {
			shown = append(shown, text)
		}
	}
	wanted := []string{"../", "archive/", "5B orders.csv"}
	if !slices.EqualFunc(shown, wanted, func(line, want string) bool {
		return strings.HasSuffix(line, want)
	}) {
		t.Errorf("the picker lists %q, wanted %q", shown, wanted)
	}
}

func TestTheFilePickerEntersAndLeavesADirectory(t *testing.T) {
	directory := buildPickerDirectory(t)
	picker := newFilePicker(directory, load.ListFileExtensions())
	picker = readPickerListing(t, picker, picker.Init())

	picker, _, _ = picker.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	picker, command, chosen := picker.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if chosen != "" || picker.Directory != filepath.Join(directory, "archive") {
		t.Fatalf("Enter on a directory chose %q and stands in %q", chosen, picker.Directory)
	}
	picker = readPickerListing(t, picker, command)
	if row, _ := picker.findCursorRow(); !row.parent {
		t.Errorf("an empty directory does not offer its parent: %+v", picker.rows)
	}

	picker, command, _ = picker.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	picker = readPickerListing(t, picker, command)
	if row, _ := picker.findCursorRow(); picker.Directory != directory || row.name != "archive" {
		t.Errorf("leaving stands in %q on %q", picker.Directory, row.name)
	}

	picker, _, _ = picker.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	_, _, chosen = picker.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if chosen != filepath.Join(directory, "orders.csv") {
		t.Errorf("Enter on a file chose %q", chosen)
	}
}

// The file the user picked is written into the row of the form the picker fills in, and the
// file is read at once: the form asked for a path and now it has one.
func TestReadPickedFileWritesThePathAndReadsTheFile(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	path := filepath.Join(t.TempDir(), "orders.tsv")
	if err := os.WriteFile(path, []byte("a\tb\n1\t2\n"), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}

	connection := buildImportConnection()
	held, command, taken := model.readPickedFile(connection, 1, path)
	if !taken || command == nil {
		t.Fatalf("the file was picked and nothing read it: %v, %v", taken, command)
	}
	_ = held

	overlay := connection.Overlay
	if overlay.Import.Plan.Path != path {
		t.Errorf("the path reads %q, wanted %q", overlay.Import.Plan.Path, path)
	}
	if overlay.Import.Stage != app.ImportFile {
		t.Errorf("the stage is %q, wanted the one that reads the file",
			overlay.Import.Stage)
	}
	if !overlay.Import.Running {
		t.Error("the card does not say that the file is being read")
	}
	// The row of the form reads the path as well, so the form shows what was picked.
	if overlay.Field != importFileField || overlay.Draft.Text != path {
		t.Errorf("the form holds %q on row %d, wanted the path on the file row",
			overlay.Draft.Text, overlay.Field)
	}
	// The name of the file marks it as separated by tabs, which the picker fills in too.
	if overlay.Import.Plan.Options.Delimiter != "\t" {
		t.Errorf("the delimiter reads %q, wanted a tab",
			overlay.Import.Plan.Options.Delimiter)
	}
}

// buildImportConnection returns a connection with an import open on the picker.
func buildImportConnection() *app.Connection {
	connection := &app.Connection{}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayImport,
		Import: app.ImportRequest{
			Stage: app.ImportPick,
			Plan:  load.Plan{Options: load.DefaultReadOptions()},
		},
		Draft: app.NewEditorBuffer("", 0),
	}
	return connection
}

// A picker belongs to the connection its import is open on, so an import on one connection
// does not read the directory of another.
func TestFilePickersBelongToTheirConnection(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)

	if model.findFilePicker(1) != nil {
		t.Error("a connection with no import holds a picker")
	}
	model.openFilePicker(1, load.ListFileExtensions())
	model.openFilePicker(2, load.ListFileExtensions())

	first, second := model.findFilePicker(1), model.findFilePicker(2)
	if first == nil || second == nil {
		t.Fatal("a picker is missing")
	}
	if first == second {
		t.Error("two imports share one picker")
	}
}

func TestTheImportPickerTitleNamesTheTargetTable(t *testing.T) {
	into := app.ImportRequest{Plan: load.Plan{Table: query.QualifiedName{Schema: "public", Name: "orders"}}}
	if title := buildImportTitle(into); title != " import into public.orders " {
		t.Errorf("the title reads %q", title)
	}
	into.Plan.CreatesTable = true
	if title := buildImportTitle(into); title != " import into a new table in public " {
		t.Errorf("the title of a new table reads %q", title)
	}
}
