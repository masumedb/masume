package cfg_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
)

func TestBuildProfileFromTargetReadsSeveralHosts(t *testing.T) {
	for _, one := range []struct {
		target string
		host   string
		port   int
		others []string
	}{
		{"postgres://ada@db1:5433,db2,[::1]:5434/shop", "db1", 5433, []string{"db2:5432", "[::1]:5434"}},
		{"mongodb://db1,db2:27018/shop?replicaSet=rs0", "db1", 27017, []string{"db2:27018"}},
		{"host=db1,db2 port=5433 dbname=shop user=ada", "db1", 5433, []string{"db2:5433"}},
		{"host=db1,db2 port=5433,5434 dbname=shop user=ada", "db1", 5433, []string{"db2:5434"}},
	} {
		built, err := cfg.BuildProfileFromTarget(one.target)
		if err != nil {
			t.Errorf("%s: %v", one.target, err)
			continue
		}
		if built.Host != one.host || built.Port != one.port || !slices.Equal(built.OtherHosts, one.others) {
			t.Errorf("%s reads %s:%d and %v", one.target, built.Host, built.Port, built.OtherHosts)
		}
	}
}

func TestBuildProfileFromTargetRefusesSeveralHostsItCannotUse(t *testing.T) {
	for target, wanted := range map[string]string{
		"mysql://ada@db1,db2/shop":                       "PostgreSQL and MongoDB families only",
		"postgres://ada@db1,,db2/shop":                   "empty entry",
		"host=db1,db2,db3 port=1,2 dbname=shop user=ada": "2 ports for 3 hosts",
	} {
		if _, err := cfg.BuildProfileFromTarget(target); err == nil || !strings.Contains(err.Error(), wanted) {
			t.Errorf("%s answered %v", target, err)
		}
	}
}

func TestSaveProfileWritesTheOtherHosts(t *testing.T) {
	profile, err := cfg.BuildProfileFromTarget("postgres://ada@db1,db2:5433/shop")
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
	if !strings.Contains(string(written), `other_hosts = ["db2:5433"]`) {
		t.Errorf("the file reads:\n%s", written)
	}
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 || len(loaded.Profiles) != 1 ||
		!slices.Equal(loaded.Profiles[0].OtherHosts, profile.OtherHosts) {
		t.Errorf("the profile read back as %+v with %v", loaded.Profiles, loaded.Problems)
	}
}

func TestLoadConfigRefusesOtherHostsWithATunnel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	text := "[profile.shop]\nengine = \"postgres\"\nhost = \"db\"\nuser = \"ada\"\ndatabase = \"shop\"\n" +
		"other_hosts = [\"db2:5432\"]\nssh_host = \"bastion\"\nssh_user = \"ada\"\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 1 || !strings.Contains(loaded.Problems[0].Reason, "ssh tunnel") {
		t.Errorf("the problems are %v", loaded.Problems)
	}
}

func TestParseConnectionURLReadsSeveralHosts(t *testing.T) {
	held, parsed := cfg.ParseConnectionURL("postgres://ada@db1,db2:5433/shop")
	if !parsed || held.Host != "db1" || held.Port != 5432 || !slices.Equal(held.OtherHosts, []string{"db2:5433"}) {
		t.Errorf("the URL reads as %+v, parsed %v", held, parsed)
	}
}

func TestFindShownFieldsShowsTheOptionsAndTheOtherHosts(t *testing.T) {
	for target, wanted := range map[string]map[string]bool{
		"postgres://ada@db/shop": {"options": true, "otherHosts": true},
		"mysql://ada@db/shop":    {"options": true, "otherHosts": false},
		"./notes.db":             {"options": false, "otherHosts": false},
	} {
		profile, err := cfg.BuildProfileFromTarget(target)
		if err != nil {
			t.Fatal(err)
		}
		shown := map[string]bool{}
		for _, field := range cfg.FindShownFields(cfg.BuildFormFields(profile, true, nil)) {
			shown[field.Key] = true
		}
		for key, visible := range wanted {
			if shown[key] != visible {
				t.Errorf("%s shows %s: %v", target, key, shown[key])
			}
		}
	}
}
