package clickhouse

import (
	"slices"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
)

// Composer reads a relation and a statement as the shared SQL composer does. A staged
// change is a mutation of the table, which the dialect writes.
type Composer struct {
	db.SQLComposer
}

// NewComposer returns the composer of a ClickHouse connection.
func NewComposer(dialect *query.Dialect) Composer {
	return Composer{SQLComposer: db.NewSQLComposer(dialect)}
}

// BuildChanges refuses an insert of a row into a view, which the server cannot write, and
// hands every other change to the shared builder.
func (composer Composer) BuildChanges(
	target db.ChangeTarget, staged core.PendingChanges,
) ([]db.Change, error) {
	if target.Table.Kind != db.RelationTable && len(staged.Inserts) > 0 {
		return nil, core.NewEditError("this server writes no row into a view")
	}
	target.Rows = buildByteArrayRows(target.Columns, target.Rows)
	return composer.SQLComposer.BuildChanges(target, staged)
}

// buildByteArrayRows returns the rows with each Array(UInt8) value as a list of numbers.
func buildByteArrayRows(columns []db.ResultColumn, rows [][]any) [][]any {
	byteArrays := []int{}
	for at, column := range columns {
		if parseColumnType(column.DataType).unwrap().isByteArray() {
			byteArrays = append(byteArrays, at)
		}
	}
	if len(byteArrays) == 0 {
		return rows
	}
	converted := make([][]any, len(rows))
	for at, row := range rows {
		converted[at] = slices.Clone(row)
		for _, column := range byteArrays {
			if column >= len(row) {
				continue
			}
			held, isBytes := row[column].([]byte)
			if !isBytes {
				continue
			}
			numbers := make([]any, len(held))
			for index, element := range held {
				numbers[index] = element
			}
			converted[at][column] = numbers
		}
	}
	return converted
}

// Compile-time Composer interface check.
var _ db.Composer = Composer{}
