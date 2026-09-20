package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// shownPathHead is how much of a directory path the card of the picker draws on every
// platform.
const shownPathHead = 24

// buildCertificateFormModel opens the connection form on a profile with the tls fields
// shown, with the caret on that field.
func buildCertificateFormModel(t *testing.T, field string) *Model {
	t.Helper()
	useNoKeyring(t)
	model := buildOfflineModel(t, 100, 40)
	profile := buildPromptingProfile("shop")
	profile.SSLRootCert = "/certs/ca.pem"
	model.form = NewFormState(profile, true, nil)
	model.screen = ScreenEditingConnection
	focusFormField(t, model.form, field)
	return model
}

// A certificate path is a file on this machine, so the field opens a picker rather than
// asking the user to type the whole path.
func TestTheFormOpensAFilePickerOnACertificateField(t *testing.T) {
	for _, field := range []string{"sslRootCert", "sslCert", "sslKey"} {
		model := buildCertificateFormModel(t, field)

		held, command := model.readFormKey(tea.Key{Code: tea.KeyEnter})
		model = held.(*Model)
		if command == nil {
			t.Errorf("%s opened the picker without reading a directory", field)
		}
		if model.formPicker == nil {
			t.Fatalf("%s did not open the picker", field)
		}
		if model.formPicker.field != field {
			t.Errorf("the picker fills %q, wanted %q", model.formPicker.field, field)
		}
	}
}

// Every other field steps on, as it did before the picker.
func TestTheFormStepsOnFromAFieldWithoutAFile(t *testing.T) {
	model := buildCertificateFormModel(t, "host")
	before := model.form.Cursor

	held, _ := model.readFormKey(tea.Key{Code: tea.KeyEnter})
	model = held.(*Model)
	if model.formPicker != nil {
		t.Fatal("the host field opened a file picker")
	}
	if model.form.Cursor == before {
		t.Error("the caret stayed on the host field")
	}
}

// The picker opens in the directory the field names, so a path that is already written is
// changed where it stands.
func TestTheFilePickerOpensInTheDirectoryOfTheField(t *testing.T) {
	directory := t.TempDir()
	model := buildCertificateFormModel(t, "sslRootCert")
	model.form.Fields = cfg.ApplyFieldChange(
		model.form.Fields, "sslRootCert", filepath.Join(directory, "ca.pem"))
	model.form.openField()

	held, _ := model.readFormKey(tea.Key{Code: tea.KeyEnter})
	model = held.(*Model)
	if model.formPicker == nil {
		t.Fatal("the field did not open the picker")
	}
	if model.formPicker.picker.CurrentDirectory != directory {
		t.Errorf("the picker stands in %q, wanted %q",
			model.formPicker.picker.CurrentDirectory, directory)
	}
}

// The card draws the directory it stands in, and the keys it reads.
func TestTheFilePickerCardDrawsTheDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(directory, "ca.pem"), []byte("held"), 0o600); err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
	model := buildCertificateFormModel(t, "sslRootCert")
	model.form.Fields = cfg.ApplyFieldChange(
		model.form.Fields, "sslRootCert", filepath.Join(directory, "ca.pem"))
	model.form.openField()

	held, command := model.readFormKey(tea.Key{Code: tea.KeyEnter})
	model = held.(*Model)
	// The picker reads the directory with a command of its own, and draws it after.
	if command != nil {
		next, _ := model.Update(command())
		model = next.(*Model)
	}

	drawn := stripEscapes(model.renderForm())
	if !strings.Contains(drawn, "ssl root cert") {
		t.Errorf("the card does not name the field:\n%s", drawn)
	}
	// macOS puts a temporary directory under a path longer than the card, and the card
	// cuts what does not fit, so the head of the path is what it draws.
	head := directory
	if len(head) > shownPathHead {
		head = head[:shownPathHead]
	}
	if !strings.Contains(drawn, head) {
		t.Errorf("the card does not name the directory:\n%s", drawn)
	}
	if !strings.Contains(drawn, "ca.pem") {
		t.Errorf("the card lists no file of the directory:\n%s", drawn)
	}
}

// Escape closes the picker and leaves the field as it was.
func TestTheFilePickerLeavesTheFieldOnEscape(t *testing.T) {
	model := buildCertificateFormModel(t, "sslRootCert")
	held, _ := model.readFormKey(tea.Key{Code: tea.KeyEnter})
	model = held.(*Model)

	held, _ = model.readFormKey(tea.Key{Code: tea.KeyEscape})
	model = held.(*Model)
	if model.formPicker != nil {
		t.Fatal("escape left the picker open")
	}
	if model.screen != ScreenEditingConnection {
		t.Errorf("escape closed the form, the screen is %q", model.screen)
	}
	if held := cfg.ReadField(model.form.Fields, "sslRootCert"); held != "/certs/ca.pem" {
		t.Errorf("the field reads %q, wanted the path it held", held)
	}
}

// The file the user chooses is written into the field the picker was opened on.
func TestTheFilePickerWritesTheChosenPathIntoTheField(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "root.pem")
	model := buildCertificateFormModel(t, "sslCert")

	model.writeFormFilePath("sslCert", path)
	if held := cfg.ReadField(model.form.Fields, "sslCert"); held != path {
		t.Errorf("the field reads %q, wanted %q", held, path)
	}
	if model.form.Draft.Text != path {
		t.Errorf("the caret holds %q, wanted %q", model.form.Draft.Text, path)
	}
}
