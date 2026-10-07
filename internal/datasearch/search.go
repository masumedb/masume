package datasearch

import (
	"context"
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// Match is one table with rows that hold the text.
type Match struct {
	Table db.TableRef
	// Rows is the count of matched rows read, up to the row limit. Capped is true where the
	// table holds more.
	Rows   int
	Capped bool
	// Columns with the text in a matched row.
	Columns []string
	// Filter is the WHERE text that reads the matched rows.
	Filter string
}

// Report is the outcome of a search: the tables with matches, and the tables that failed.
type Report struct {
	Matches  []Match
	Searched int
	Failed   []string
}

// Session is the part of a session a search reads.
type Session interface {
	db.SessionInfo
	db.CatalogReader
	RunQuery(ctx context.Context, sql string, rowLimit int, params []any) (db.QueryResult, error)
}

// Search reads every table of the schema for rows with the text in any column, as text and
// in any case. `%` and `_` in the text match any text and any one character.
func Search(ctx context.Context, session Session, schema, text string, rowLimit int) (Report, error) {
	dialect := session.Dialect()
	if dialect.MatchText == nil {
		return Report{}, db.NewUnsupportedError("search data")
	}
	tables, err := session.ListTables(ctx)
	if err != nil {
		return Report{}, err
	}
	pattern := dialect.QuoteTextLiteral("%" + text + "%")
	report := Report{}
	for _, table := range tables {
		if table.Schema != schema || table.Kind != db.RelationTable {
			continue
		}
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		report.Searched++
		match, found, searchErr := searchTable(ctx, session, table, pattern, text, rowLimit)
		if searchErr != nil {
			report.Failed = append(report.Failed, table.Name)
			continue
		}
		if found {
			report.Matches = append(report.Matches, match)
		}
	}
	return report, nil
}

func searchTable(
	ctx context.Context, session Session, table db.TableRef, pattern, text string, rowLimit int,
) (Match, bool, error) {
	dialect := session.Dialect()
	detail, err := session.DescribeTable(ctx, table)
	if err != nil {
		return Match{}, false, err
	}
	tests := []string{}
	for _, column := range detail.Columns {
		if dialect.CanCompareType != nil && !dialect.CanCompareType(column.DataType) {
			continue
		}
		tests = append(tests, dialect.MatchText(dialect.QuoteIdentifier(column.Name), pattern))
	}
	if len(tests) == 0 {
		return Match{}, false, nil
	}
	filter := strings.Join(tests, " or ")
	read := "select * from " + dialect.BuildQualifiedName(table.Qualified()) + " where " + filter
	result, err := session.RunQuery(ctx, read, rowLimit, nil)
	if err != nil {
		return Match{}, false, err
	}
	if len(result.Rows) == 0 {
		return Match{}, false, nil
	}
	return Match{
		Table: table, Rows: len(result.Rows), Capped: result.Truncated,
		Columns: findMatchedColumns(result, text), Filter: filter,
	}, true, nil
}

// findMatchedColumns returns the columns whose text holds the search text in a read row. A
// search with a wildcard can match a column this misses.
func findMatchedColumns(result db.QueryResult, text string) []string {
	wanted := strings.ToLower(text)
	found := []string{}
	for at, column := range result.Columns {
		for _, row := range result.Rows {
			if at < len(row) && strings.Contains(strings.ToLower(core.FormatCell(row[at], "")), wanted) {
				found = append(found, column.Name)
				break
			}
		}
	}
	return found
}
