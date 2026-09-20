package db

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
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
		return "default " + column.DefaultValue
	}
	return ""
}

// RenderTableDDL builds CREATE TABLE SQL from catalog metadata.
func RenderTableDDL(
	detail TableDetail, indexes []IndexDetail, constraints []ConstraintDetail,
	dialect *query.Dialect,
) []string {
	lines := []string{"create table " + dialect.BuildQualifiedName(detail.Table.Qualified()) + " ("}

	body := make([]string, 0, len(detail.Columns)+len(constraints))
	for _, column := range detail.Columns {
		parts := []string{"    " + dialect.QuoteIdentifier(column.Name) + " " + column.DataType}
		if written := describeColumnSource(column, dialect); written != "" {
			parts = append(parts, written)
		}
		if !column.Nullable {
			parts = append(parts, "not null")
		}
		body = append(body, strings.Join(parts, " "))
	}
	for _, constraint := range constraints {
		body = append(body, "    constraint "+dialect.QuoteIdentifier(constraint.Name)+" "+
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
