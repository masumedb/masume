package db

import (
	"strings"

	"github.com/masumedb/masume/internal/query"
)

// describeColumnSource returns what a CREATE TABLE writes after the type of a column: the
// numbering of the server, the expression it computes the column from, or the default value.
func describeColumnSource(column ColumnDetail, dialect *query.Dialect) string {
	if column.IsIdentityAlways {
		return dialect.IdentityClause
	}
	if column.IsGenerated {
		if !column.HasDefault || dialect.RenderGeneratedColumn == nil {
			return ""
		}
		return dialect.RenderGeneratedColumn(column.DefaultValue)
	}
	if column.HasDefault {
		return "DEFAULT " + column.DefaultValue
	}
	return ""
}

// RenderColumnDefinition returns the column as a CREATE TABLE writes it: name, type,
// default or identity, and NOT NULL.
func RenderColumnDefinition(column ColumnDetail, dialect *query.Dialect, quotesName bool) string {
	quote := dialect.QuoteIdentifierIfNeeded
	if quotesName {
		quote = dialect.QuoteIdentifier
	}
	parts := []string{quote(column.Name) + " " + column.DataType}
	if written := describeColumnSource(column, dialect); written != "" {
		parts = append(parts, written)
	}
	if !column.Nullable {
		parts = append(parts, "NOT NULL")
	}
	return strings.Join(parts, " ")
}

// RenderTableDDL builds CREATE TABLE SQL from catalog metadata, with keywords in capitals.
// quotesEveryName quotes each name; otherwise a name is quoted only where needed.
func RenderTableDDL(
	detail TableDetail, indexes []IndexDetail, constraints []ConstraintDetail,
	dialect *query.Dialect, quotesEveryName bool,
) []string {
	quote, qualify := dialect.QuoteIdentifierIfNeeded, dialect.BuildQualifiedNameIfNeeded
	if quotesEveryName {
		quote, qualify = dialect.QuoteIdentifier, dialect.BuildQualifiedName
	}
	lines := []string{"CREATE TABLE " + qualify(detail.Table.Qualified()) + " ("}

	body := make([]string, 0, len(detail.Columns)+len(constraints))
	for _, column := range detail.Columns {
		body = append(body, "    "+RenderColumnDefinition(column, dialect, quotesEveryName))
	}
	for _, constraint := range constraints {
		body = append(body, "    CONSTRAINT "+quote(constraint.Name)+" "+
			constraint.Definition)
	}

	for at, line := range body {
		if at == len(body)-1 {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, line+",")
	}
	lines = append(lines, ");")

	secondary := make([]IndexDetail, 0, len(indexes))
	for _, index := range indexes {
		if !index.IsPrimary {
			secondary = append(secondary, index)
		}
	}
	if len(secondary) > 0 {
		lines = append(lines, "")
		for _, index := range secondary {
			lines = append(lines, index.Definition+";")
		}
	}
	return lines
}
