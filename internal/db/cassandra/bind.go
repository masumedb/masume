package cassandra

import (
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"gopkg.in/inf.v0"

	"github.com/masumedb/masume/internal/db"
)

// The layouts a timestamp is written in, from the widest to the narrowest.
var timestampLayouts = []string{
	time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999", "2006-01-02 15:04:05", "2006-01-02",
}

// buildBinding returns the callback that binds these values to a prepared statement. The
// grid writes every edited cell as text, and the driver marshals a value by its Go type
// alone, so each value takes the type of the column it is bound to.
func buildBinding(params []any) func(*gocql.QueryInfo) ([]any, error) {
	return func(info *gocql.QueryInfo) ([]any, error) {
		bound := make([]any, 0, len(params))
		for at, value := range params {
			if at >= len(info.Args) {
				bound = append(bound, value)
				continue
			}
			held, err := castParameter(value, info.Args[at].TypeInfo)
			if err != nil {
				return nil, err
			}
			bound = append(bound, held)
		}
		return bound, nil
	}
}

// castParameter returns the value as the driver marshals that column type. A value that is
// not text is passed on as it is.
func castParameter(value any, held gocql.TypeInfo) (any, error) {
	written, isText := value.(string)
	if !isText || held == nil {
		return value, nil
	}
	switch held.Type() {
	case gocql.TypeText, gocql.TypeVarchar, gocql.TypeAscii:
		return written, nil
	case gocql.TypeInt, gocql.TypeSmallInt, gocql.TypeTinyInt, gocql.TypeBigInt,
		gocql.TypeCounter:
		return castInteger(written, held)
	case gocql.TypeFloat:
		read, err := strconv.ParseFloat(written, 32)
		return float32(read), wrapCast(err, written, held)
	case gocql.TypeDouble:
		read, err := strconv.ParseFloat(written, 64)
		return read, wrapCast(err, written, held)
	case gocql.TypeBoolean:
		read, err := strconv.ParseBool(written)
		return read, wrapCast(err, written, held)
	case gocql.TypeDecimal:
		read, set := new(inf.Dec).SetString(written)
		if !set {
			return nil, db.NewDatabaseError("%q is not a decimal", written)
		}
		return read, nil
	case gocql.TypeVarint:
		read, err := strconv.ParseInt(written, 10, 64)
		return read, wrapCast(err, written, held)
	case gocql.TypeUUID, gocql.TypeTimeUUID:
		read, err := gocql.ParseUUID(written)
		return read, wrapCast(err, written, held)
	case gocql.TypeTimestamp:
		return castTimestamp(written)
	case gocql.TypeBlob:
		return []byte(written), nil
	}
	return written, nil
}

// castInteger returns a whole number as the width of its column.
func castInteger(written string, held gocql.TypeInfo) (any, error) {
	read, err := strconv.ParseInt(strings.TrimSpace(written), 10, 64)
	if err != nil {
		return nil, wrapCast(err, written, held)
	}
	switch held.Type() {
	case gocql.TypeInt:
		return int(read), nil
	case gocql.TypeSmallInt:
		return int16(read), nil
	case gocql.TypeTinyInt:
		return int8(read), nil
	}
	return read, nil
}

// castTimestamp reads a written time in the layouts the grid and the server write.
func castTimestamp(written string) (any, error) {
	trimmed := strings.TrimSpace(written)
	for _, layout := range timestampLayouts {
		if read, err := time.Parse(layout, trimmed); err == nil {
			return read, nil
		}
	}
	return nil, db.NewDatabaseError("%q is not a timestamp", written)
}

// wrapCast names the value and the column type of a value the driver cannot take.
func wrapCast(err error, written string, held gocql.TypeInfo) error {
	if err == nil {
		return nil
	}
	return db.NewDatabaseError("%q is not a %s", written, held.Type().String())
}
