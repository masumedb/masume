package cassandra

import (
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/language"
	"github.com/masumedb/masume/internal/query/syntax"
)

// Dialect writes CQL the way a Cassandra reads it. A keyspace is the schema, and a name in
// double quotes keeps its case.
var Dialect = &query.Dialect{
	Engine: core.EngineCassandra, Syntax: syntax.FlavourStandard, SchemaWord: "keyspace",
	StatementLanguage: "CQL", FenceTag: "sql", StatementHint: "select … from …",
	QuoteIdentifier: func(name string) string {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	},
	BuildPlaceholder: func(int) string { return "?" },
	CountExpression:  "count(*)",
	// The server takes no row lock a statement can ask for.
	RowLockClause: "",
	// CQL reads bytes as a hexadecimal literal.
	RenderBytes: func(hex string) string { return "0x" + hex },
	QuoteTextLiteral: func(text string) string {
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	},
	// A WHERE on a column outside the key needs an index or ALLOW FILTERING, which the
	// composed read does not write, so only a key column compares.
	CanCompareType: func(string) bool { return true },
	ColumnTypes: map[core.ColumnKind]string{
		core.KindText: "text", core.KindInteger: "bigint", core.KindNumber: "decimal",
		core.KindBoolean: "boolean", core.KindTimestamp: "timestamp",
	},
	// A new table takes a partition key of its own, and the server makes no value for it.
	IdentityColumn: "id uuid primary key",
	DropSchema: func(dialect *query.Dialect, schema string) string {
		return "drop keyspace " + dialect.QuoteIdentifier(schema) + ";"
	},
	DropTrigger: func(dialect *query.Dialect, schema, name, table string) string {
		target := dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: table})
		return "drop trigger " + dialect.QuoteIdentifier(name) + " on " + target + ";"
	},
	DropRoutine: func(dialect *query.Dialect, schema, name, _ string) string {
		return "drop function " +
			dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: name}) + ";"
	},
}

// Support is everything known about a Cassandra before a connection exists.
var Support = BuildSupport(core.EngineCassandra)

// BuildSupport combines engine metadata with the CQL dialect and language.
func BuildSupport(engine core.Engine) db.EngineSupport {
	return db.EngineSupport{
		EngineInfo: core.ResolveEngineInfo(engine),
		Dialect:    Dialect,
		Language:   language.SQL,
		Compose:    db.NewSQLComposer(Dialect),
	}
}
