package sqlite

import (
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/language"
	"github.com/masumedb/masume/internal/query/syntax"
)

// Dialect is the SQLite syntax configuration. Schemas are attached databases; main is the primary database file.
var Dialect = &query.Dialect{
	Engine: core.EngineSqlite, Syntax: syntax.FlavourStandard, SchemaWord: "database",
	StatementLanguage: "SQL", FenceTag: "sql", StatementHint: "select … from …",
	QuoteIdentifier: func(name string) string {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	},
	BuildPlaceholder: func(int) string { return "?" },
	CountExpression:  "count(*)",
	// SQLite reads bytes as a hexadecimal literal.
	RenderBytes: func(hex string) string { return "x'" + hex + "'" },
	QuoteTextLiteral: func(text string) string {
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	},
	// SQLite compares any two values it stores, whatever the column type is.
	CanCompareType: func(string) bool { return true },
	BindLimit:      32766,
	ColumnTypes: map[core.ColumnKind]string{
		core.KindText: "text", core.KindInteger: "integer", core.KindNumber: "real",
		// SQLite booleans use integers; timestamps use text.
		core.KindBoolean: "integer", core.KindTimestamp: "text",
	},
	IdentityColumn: "id integer primary key autoincrement",
	// A SQLite database is a file, and DETACH is the only way to release one.
	DropSchema: func(dialect *query.Dialect, schema string) string {
		return "detach database " + dialect.QuoteIdentifier(schema) + ";"
	},
	DropTrigger: func(dialect *query.Dialect, schema, name, _ string) string {
		return "drop trigger " +
			dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: name}) + ";"
	},
	DropRoutine: func(*query.Dialect, string, string, string) string {
		return "-- SQLite stored routines are unsupported"
	},
}

// Support is everything known about a SQLite file before it is opened.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineSqlite),
	Dialect:    Dialect,
	Language:   language.SQL,
	Compose:    db.NewSQLComposer(Dialect),
}

// BuildSupport combines engine metadata with the SQLite dialect and language.
func BuildSupport(engine core.Engine) db.EngineSupport {
	return db.EngineSupport{
		EngineInfo: core.ResolveEngineInfo(engine),
		Dialect:    Dialect,
		Language:   language.SQL,
		Compose:    db.NewSQLComposer(Dialect),
	}
}
