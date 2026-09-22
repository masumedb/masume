package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
)

// A connection is saved under the name of its block in the config file, so two connections
// of one name would write into one block. The form refuses the name the second one takes.
func TestTheFormRefusesANameAnotherConnectionHolds(t *testing.T) {
	useNoKeyring(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("MASUME_CONFIG", path)
	written := "[profile.alpha]\nengine = \"postgres\"\nhost = \"alpha.example\"\n" +
		"port = 5432\ndatabase = \"shop\"\nuser = \"ada\"\n"
	if err := os.WriteFile(path, []byte(written), 0o600); err != nil {
		t.Fatal(err)
	}

	model := buildOfflineModel(t, 160, 48)
	model.profiles = []cfg.Profile{
		buildPromptingProfile("alpha"), buildPromptingProfile("beta"),
	}
	model.form = NewFormState(buildPromptingProfile("beta"), true, nil)
	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "name", "alpha")
	model.form.openField()
	model.screen = ScreenEditingConnection

	model.saveForm()
	if model.form == nil {
		t.Fatal("the form closed on a name another connection holds")
	}
	if model.form.Test != TestFailed {
		t.Errorf("the form reports %q", model.form.Test)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != written {
		t.Errorf("the config file was written:\n%s", after)
	}
}

// A new connection takes the name of no connection in the file either, so the block of that
// connection keeps the values it holds.
func TestANewConnectionRefusesANameTheFileHolds(t *testing.T) {
	useNoKeyring(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("MASUME_CONFIG", path)
	written := "[profile.alpha]\nengine = \"postgres\"\nhost = \"alpha.example\"\n" +
		"port = 5432\ndatabase = \"shop\"\nuser = \"ada\"\n"
	if err := os.WriteFile(path, []byte(written), 0o600); err != nil {
		t.Fatal(err)
	}

	model := buildOfflineModel(t, 160, 48)
	model.profiles = []cfg.Profile{buildPromptingProfile("alpha")}
	model.form = NewFormState(cfg.Profile{}, false, nil)
	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "name", "alpha")
	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "host", "beta.example")
	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "database", "other")
	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "user", "grace")
	model.form.openField()
	model.screen = ScreenEditingConnection

	model.saveForm()
	if model.form == nil || model.form.Test != TestFailed {
		t.Fatal("the form saved a connection under a name the file holds")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != written {
		t.Errorf("the config file was written:\n%s", after)
	}
}
