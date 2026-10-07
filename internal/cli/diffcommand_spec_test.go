package cli

import (
	"strings"
	"testing"
)

func TestParseDiffArgumentsReadsTheSidesInOrder(t *testing.T) {
	held, err := parseDiffArguments([]string{
		"-p", "staging", "postgres://you@host/shop", "-s", "public", "--target-schema=v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(held.sides) != 2 || held.sides[0].profileName != "staging" ||
		held.sides[1].target != "postgres://you@host/shop" {
		t.Fatalf("sides: %+v", held.sides)
	}
	if held.sourceSchema != "public" || held.targetSchema != "v2" {
		t.Errorf("schemas %q and %q", held.sourceSchema, held.targetSchema)
	}
}

func TestParseDiffArgumentsRefusesWhatItCannotRun(t *testing.T) {
	for _, held := range []struct {
		name   string
		argv   []string
		reason string
	}{
		{"one side", []string{"a.db"}, "requires two connections"},
		{"three sides", []string{"a.db", "b.db", "-p", "c"}, "received 3"},
		{"an unknown option", []string{"--table", "a.db", "b.db"}, "unknown"},
	} {
		t.Run(held.name, func(t *testing.T) {
			_, err := parseDiffArguments(held.argv)
			if err == nil || !strings.Contains(err.Error(), held.reason) {
				t.Errorf("the error is %v, wanted %q", err, held.reason)
			}
		})
	}
}
