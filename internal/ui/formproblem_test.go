package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
)

func TestAFailedSaveMovesTheCaretToTheFieldItIsAbout(t *testing.T) {
	useNoKeyring(t)
	t.Setenv("MASUME_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	model := buildOfflineModel(t, 160, 48)
	model.form = NewFormState(cfg.Profile{}, false, nil)
	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "name", "shop")
	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "host", "db.example")
	model.form.openField()
	model.screen = ScreenEditingConnection

	model.saveForm()
	form := model.form
	if form.BadField != "database" {
		t.Fatalf("the failure is about %q, wanted the database field", form.BadField)
	}
	if shown := form.Shown(); shown[form.Cursor].Key != "database" {
		t.Errorf("the caret is on %q, wanted the database field", shown[form.Cursor].Key)
	}
	if !strings.Contains(stripEscapes(model.renderForm()), "the database name is missing") {
		t.Error("the form does not show the failure")
	}
}

func TestAFailedSaveNamesTheFieldInTheWordsOfTheForm(t *testing.T) {
	fields := cfg.BuildFormFields(cfg.Profile{}, false, nil)
	for key, value := range map[string]string{
		"name": "shop", "host": "db.example", "database": "shop", "user": "ada",
		"auth": string(cfg.AuthCommand),
	} {
		fields = cfg.ApplyFieldChange(fields, key, value)
	}
	_, err := cfg.BuildProfileFromFields(fields, cfg.Profile{}, false)
	problem, isFormError := err.(cfg.FormError)
	if !isFormError || problem.Field != "passwordCommand" ||
		problem.Reason != "the password command is missing" {
		t.Errorf("the save answered %#v, wanted the password command field", err)
	}
}
