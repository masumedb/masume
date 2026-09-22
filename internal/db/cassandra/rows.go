package cassandra

import (
	"reflect"

	"github.com/gocql/gocql"
	"gopkg.in/inf.v0"

	"github.com/masumedb/masume/internal/db"
)

// readColumns returns the result columns of an iterator, with the CQL type of each.
func readColumns(held []gocql.ColumnInfo) []db.ResultColumn {
	columns := make([]db.ResultColumn, 0, len(held))
	for _, column := range held {
		columns = append(columns, db.ResultColumn{
			Name: column.Name, DataType: readTypeName(column.TypeInfo),
		})
	}
	return columns
}

// readTypeName returns the CQL name of a type.
func readTypeName(held gocql.TypeInfo) string {
	if held == nil {
		return ""
	}
	return held.Type().String()
}

// readRow returns the values of the next row, and false at the end of the result. Each
// value is scanned into a pointer to the holder the driver asks for, so a null column
// answers a nil pointer rather than the zero value of its type.
func readRow(iterator *gocql.Iter) ([]any, bool) {
	row, err := iterator.RowData()
	if err != nil {
		return nil, false
	}

	holders := make([]any, len(row.Values))
	for at, held := range row.Values {
		holders[at] = reflect.New(reflect.TypeOf(held)).Interface()
	}
	if !iterator.Scan(holders...) {
		return nil, false
	}

	values := make([]any, len(holders))
	for at, holder := range holders {
		held := reflect.ValueOf(holder).Elem()
		if held.IsNil() {
			continue
		}
		values[at] = readValue(held.Elem().Interface())
	}
	return values, true
}

// readValue returns a scanned value as the grid holds it.
func readValue(held any) any {
	switch value := held.(type) {
	case gocql.UUID:
		return value.String()
	case *inf.Dec:
		if value == nil {
			return nil
		}
		return value.String()
	case inf.Dec:
		return value.String()
	case int:
		return int64(value)
	case int8:
		return int64(value)
	case int16:
		return int64(value)
	case int32:
		return int64(value)
	case float32:
		return float64(value)
	}
	return held
}
