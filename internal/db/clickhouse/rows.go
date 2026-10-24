package clickhouse

import (
	"strings"

	"github.com/masumedb/masume/internal/db"
)

// readCommands is the set of commands that answer with rows.
var readCommands = map[string]bool{
	"select": true, "with": true, "show": true, "describe": true, "desc": true,
	"explain": true, "exists": true, "check": true,
}

// plannedCommands is the set of commands the server plans without running.
var plannedCommands = map[string]bool{"select": true, "with": true}

// ReadsRows is true for a command whose answer is a result set.
func ReadsRows(command string) bool {
	return readCommands[strings.ToLower(command)]
}

// PlansStatement is true for a command the server plans, which is how a statement is
// checked without running it.
func PlansStatement(command string) bool {
	return plannedCommands[strings.ToLower(command)]
}

// relationKindByEngine reads the engine `system.tables` names for a view.
var relationKindByEngine = map[string]db.RelationKind{
	"view": db.RelationView, "materializedview": db.RelationMaterializedView,
	"livewview": db.RelationView, "windowview": db.RelationView,
}

// MapRelationKind reads the kind of a relation out of the engine it runs on. Every other
// engine holds the rows of a table itself.
func MapRelationKind(engine any) db.RelationKind {
	kind, known := relationKindByEngine[strings.ToLower(db.ReadAnyText(engine))]
	if !known {
		return db.RelationTable
	}
	return kind
}

// nullablePrefix opens the type of a column that takes a null.
const nullablePrefix = "nullable("

// ReadsNull is true for a column type that takes a null.
func ReadsNull(dataType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(dataType)), nullablePrefix)
}
