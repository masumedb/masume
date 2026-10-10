package datasearch

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
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
	// Filter is the WHERE text, or the MongoDB find filter, that reads the matched rows.
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
// in any case. `%` and `_` in the text match any text and any one character. On MongoDB it
// reads the string and array fields of the sampled documents of each collection.
func Search(ctx context.Context, session Session, schema, text string, rowLimit int) (Report, error) {
	dialect := session.Dialect()
	isMongo := core.ResolveEngineInfo(session.Describe().Profile.Engine).Family == core.FamilyMongo
	if dialect.MatchText == nil && !isMongo {
		return Report{}, db.NewUnsupportedError("search data")
	}
	tables, err := session.ListTables(ctx)
	if err != nil {
		return Report{}, err
	}
	read := func(table db.TableRef, columns []db.ColumnDetail) (string, string) {
		return buildRowsRead(dialect, table, columns, text)
	}
	if isMongo {
		read = func(table db.TableRef, columns []db.ColumnDetail) (string, string) {
			return buildDocumentsRead(session.Composer(), table, columns, text)
		}
	}
	report := Report{}
	for _, table := range tables {
		if table.Schema != schema || table.Kind != db.RelationTable {
			continue
		}
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		report.Searched++
		match, found, searchErr := searchTable(ctx, session, table, read, text, rowLimit)
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
	ctx context.Context, session Session, table db.TableRef,
	buildRead func(db.TableRef, []db.ColumnDetail) (string, string), text string, rowLimit int,
) (Match, bool, error) {
	detail, err := session.DescribeTable(ctx, table)
	if err != nil {
		return Match{}, false, err
	}
	read, filter := buildRead(table, detail.Columns)
	if filter == "" {
		return Match{}, false, nil
	}
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

// buildRowsRead returns the SELECT of the rows with the text in a column, and its WHERE text.
func buildRowsRead(
	dialect *query.Dialect, table db.TableRef, columns []db.ColumnDetail, text string,
) (string, string) {
	pattern := dialect.QuoteTextLiteral("%" + text + "%")
	tests := []string{}
	for _, column := range columns {
		if dialect.CanCompareType != nil && !dialect.CanCompareType(column.DataType) {
			continue
		}
		tests = append(tests, dialect.MatchText(dialect.QuoteIdentifier(column.Name), pattern))
	}
	if len(tests) == 0 {
		return "", ""
	}
	filter := strings.Join(tests, " or ")
	return "select * from " + dialect.BuildQualifiedName(table.Qualified()) + " where " + filter, filter
}

// documentTextTypes are the MongoDB field types a regular expression can match.
var documentTextTypes = []string{"string", "array", "mixed"}

// buildDocumentsRead returns the find of the documents with the text in a string field, and
// its filter.
func buildDocumentsRead(
	composer db.Composer, table db.TableRef, columns []db.ColumnDetail, text string,
) (string, string) {
	var pattern strings.Builder
	for _, character := range text {
		switch character {
		case '%':
			pattern.WriteString(".*")
		case '_':
			pattern.WriteString(".")
		default:
			pattern.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	quotedPattern, _ := json.Marshal(pattern.String())

	tests := []string{}
	for _, column := range columns {
		if !slices.Contains(documentTextTypes, column.DataType) {
			continue
		}
		field, _ := json.Marshal(column.Name)
		tests = append(tests, "{"+string(field)+`:{"$regex":`+string(quotedPattern)+`,"$options":"i"}}`)
	}
	filter := ""
	switch len(tests) {
	case 0:
		return "", ""
	case 1:
		filter = tests[0]
	default:
		filter = `{"$or":[` + strings.Join(tests, ",") + "]}"
	}
	step, _ := core.BuildRawFilter(filter)
	return composer.ComposeRelationRead(table, core.ReadRewrite{Filter: []core.FilterStep{step}}).Text, filter
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
