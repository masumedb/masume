package cfg

import (
	"os"
	"slices"
	"strconv"
)

// The config file carries the version of the client that wrote it. A file below the version
// of this client is brought up by the steps below, one step per version, and each step runs
// once. A step adds what the client gained; it never changes a setting the user made, so a
// table the user took out stays out.

// ConfigVersion is the version of the config file this client writes.
const ConfigVersion = 1

// configSection is the table that carries the version.
const configSection = "masume"

// configMigration is one step: the version the file reaches, and the tables it adds.
type configMigration struct {
	to int
	// build returns the tables the step writes, and none where the file holds them
	// already.
	build func(document Table) []TableUpdate
}

// configMigrations are the steps, in order. Adding a step means adding one here and raising
// ConfigVersion to its version.
var configMigrations = []configMigration{
	{to: 1, build: buildAiTables},
}

// buildAiTables returns the providers and the agents of the file the first run writes that
// the config file has not got.
func buildAiTables(document Table) []TableUpdate {
	updates := buildStarterTables(document, []string{"ai", "providers"},
		[]string{"model", "api_key_env", "base_url", "base_url_env"},
		// The model of a provider was in the client before it was in the file, so a
		// table that names none takes the model it had.
		starterFill{keys: []string{"model"}})
	return append(updates, buildStarterTables(document, []string{"ai", "agents"},
		[]string{"command", "args", "env", "model"},
		// The command of an agent was in the client as well. A table that names one of
		// its own keeps it, and the arguments that belong to it.
		starterFill{keys: []string{"command", "args", "env"}, unless: "command"})...)
}

// starterFill is what a step adds to a table the config file already holds: the keys it can
// add, and the key that stops it adding any.
type starterFill struct {
	keys   []string
	unless string
}

// buildStarterTables returns the tables of the file the first run writes under this header
// that the config file has not got. A table it has is left as the user wrote it, and a table
// the user took out after the step ran is not written again, because a step runs once.
func buildStarterTables(
	document Table, header, order []string, fill starterFill,
) []TableUpdate {
	held, _ := findTablePath(document, header)
	starter, err := DecodeDocument(string(StarterConfig()))
	if err != nil {
		return nil
	}
	tables, found := findTablePath(starter, header)
	if !found {
		return nil
	}

	updates := make([]TableUpdate, 0, len(tables))
	for _, name := range sortedKeys(tables) {
		table, isTable := FindTable(tables[name])
		if !isTable {
			continue
		}
		written, has := FindTable(held[name])
		if has {
			// A table of the file keeps every key it names, and takes the keys the
			// client held for it before the file did.
			table = keepMissingKeys(table, written, fill)
			if len(table) == 0 {
				continue
			}
		}
		update := TableUpdate{
			Header: append(append([]string{}, header...), name),
			Values: map[string]any{},
		}
		for _, key := range listTableKeys(table, order) {
			if value, held := readTomlValue(table, key); held {
				update.Order = append(update.Order, key)
				update.Values[key] = value
			}
		}
		updates = append(updates, update)
	}
	return updates
}

// keepMissingKeys returns the keys of the starter that the table of the file has not got,
// and none where the file named the key that stops them.
func keepMissingKeys(starter, written Table, fill starterFill) Table {
	if fill.unless != "" {
		if _, held := written[fill.unless]; held {
			return nil
		}
	}
	kept := Table{}
	for _, key := range fill.keys {
		if _, held := written[key]; held {
			continue
		}
		if value, named := starter[key]; named {
			kept[key] = value
		}
	}
	return kept
}

// findTablePath returns the table at this header, walking one part at a time.
func findTablePath(document Table, header []string) (Table, bool) {
	held := document
	for _, part := range header {
		table, found := FindTable(held[part])
		if !found {
			return nil, false
		}
		held = table
	}
	return held, true
}

// listTableKeys returns the keys of a table, the ones of this order first and the rest after
// them, so a table is written in the order it reads best.
func listTableKeys(table Table, order []string) []string {
	kept := make([]string, 0, len(table))
	for _, key := range order {
		if _, held := table[key]; held {
			kept = append(kept, key)
		}
	}
	for _, key := range sortedKeys(table) {
		if !slices.Contains(order, key) {
			kept = append(kept, key)
		}
	}
	return kept
}

// readTomlValue returns a value of a table in a form the writer takes, and false for one it
// does not write.
func readTomlValue(table Table, key string) (any, bool) {
	if written, held := FindString(table, key); held {
		return written, written != ""
	}
	if written, held := FindStringList(table, key); held {
		return written, len(written) > 0
	}
	if written, held := FindInteger(table, key); held {
		return written, true
	}
	if written, held := FindBool(table, key); held {
		return written, true
	}
	return nil, false
}

// ReadConfigVersion returns the version a config file was written at. A file with no version
// was written before the client had one.
func ReadConfigVersion(document Table) int {
	section, held := FindSection(document, configSection)
	if !held {
		return 0
	}
	version, _ := FindInteger(section, "version")
	return version
}

// MigrateConfigFile brings the config file up to the version of this client and returns the
// version it reached. A file at that version already, or above it, is left as it is: a
// client that does not know a version must not write over the file that carries it.
func MigrateConfigFile(path string) (int, error) {
	document, err := ReadDocument(path)
	if err != nil {
		return 0, err
	}
	version := ReadConfigVersion(document)
	if version >= ConfigVersion {
		return version, nil
	}

	updates := []TableUpdate{}
	for _, step := range configMigrations {
		if step.to > version {
			updates = append(updates, step.build(document)...)
		}
	}
	// The file is hand written and full of comments, so a copy is kept before the first
	// change of a run. The copy carries the version it was taken at, so one migration does
	// not write over the copy of another.
	if len(updates) > 0 {
		if err := copyConfigFile(
			path, path+".bak."+strconv.Itoa(version)); err != nil {
			return version, err
		}
	}

	updates = append(updates, TableUpdate{
		Header: []string{configSection}, Order: []string{"version"},
		Values: map[string]any{"version": ConfigVersion},
	})
	if err := SaveTables(path, updates); err != nil {
		return version, err
	}
	return ConfigVersion, nil
}

// copyConfigFile writes the config file to a second path, for the copy a migration keeps.
func copyConfigFile(from, to string) error {
	written, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, written, 0o600)
}
