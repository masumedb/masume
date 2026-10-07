package cfg_test

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
)

func TestBuildProfileFromTargetKeepsTheOptionsOfAURL(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"postgres://ada@db/shop?sslmode=require&connect_timeout=5&application_name=etl%20job")
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]string{"connect_timeout": "5", "application_name": "etl job"}
	if !maps.Equal(built.Options, wanted) {
		t.Errorf("the options are %v, wanted %v", built.Options, wanted)
	}
}

func TestBuildProfileFromTargetKeepsTheMongoFieldsOutOfTheOptions(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"mongodb://ada@db/shop?authSource=admin&replicaSet=rs0&retryWrites=false")
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(built.Options, map[string]string{"retryWrites": "false"}) || built.ReplicaSet != "rs0" {
		t.Errorf("the options are %v and the replica set %q", built.Options, built.ReplicaSet)
	}
}

func TestBuildProfileFromTargetKeepsTheUnknownKeywordsAsOptions(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget("host=db dbname=shop user=ada target_session_attrs=read-write")
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(built.Options, map[string]string{"target_session_attrs": "read-write"}) {
		t.Errorf("the options are %v", built.Options)
	}
}

func TestSaveProfileWritesTheOptionsAsAnInlineTable(t *testing.T) {
	profile, err := cfg.BuildProfileFromTarget("sqlserver://sa@db/shop?app%20name=etl&dial%20timeout=5")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := cfg.SaveProfileToFile(profile, "", path); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `options = { "app name" = "etl", "dial timeout" = "5" }`) {
		t.Errorf("the file reads:\n%s", written)
	}
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 || len(loaded.Profiles) != 1 ||
		!maps.Equal(loaded.Profiles[0].Options, profile.Options) {
		t.Errorf("the profile read back as %+v with %v", loaded.Profiles, loaded.Problems)
	}
}

func TestLoadConfigRefusesOptionsThatAreNotATable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	text := "[profile.shop]\nengine = \"postgres\"\nhost = \"db\"\nuser = \"ada\"\ndatabase = \"shop\"\noptions = \"connect_timeout=5\"\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 1 || !strings.Contains(loaded.Problems[0].Reason, `"options" must be a table`) {
		t.Errorf("the problems are %v", loaded.Problems)
	}
}

func TestFormFieldsCarryTheOptions(t *testing.T) {
	profile, err := cfg.BuildProfileFromTarget("postgres://ada@db/shop?connect_timeout=5")
	if err != nil {
		t.Fatal(err)
	}
	fields := cfg.BuildFormFields(profile, true, nil)
	if written := cfg.ReadField(fields, "options"); written != "connect_timeout=5" {
		t.Fatalf("the options field holds %q", written)
	}
	for at := range fields {
		if fields[at].Key == "options" {
			fields[at].Value = "a=1&b=x%20y"
		}
	}
	built, err := cfg.BuildProfileFromFields(fields, profile, true)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(built.Options, map[string]string{"a": "1", "b": "x y"}) {
		t.Errorf("the options are %v", built.Options)
	}
}
