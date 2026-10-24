package clickhouse

import (
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// bindParams writes each value into the statement in place of a `?` placeholder, as a
// literal. A `?` in a string, a quoted name or a comment stays as it is, and `\?` is a plain
// question mark.
func bindParams(statement string, params []any) (string, error) {
	if len(params) == 0 {
		return statement, nil
	}
	var bound strings.Builder
	state := quoteState{}
	next := 0
	for at := 0; at < len(statement); at++ {
		char := statement[at]
		if char != '?' || state.inNameOrComment() {
			at = state.step(statement, at, &bound)
			continue
		}
		if state.inString {
			bound.WriteByte(char)
			continue
		}
		if at > 0 && statement[at-1] == '\\' {
			written := bound.String()
			bound.Reset()
			bound.WriteString(written[:len(written)-1])
			bound.WriteByte('?')
			continue
		}
		if next >= len(params) {
			return "", fmt.Errorf("the statement has more placeholders than the %d values", len(params))
		}
		literal, err := formatLiteral(params[next])
		if err != nil {
			return "", err
		}
		bound.WriteString(literal)
		next++
	}
	return bound.String(), nil
}

// quoteState is the place of a scan in a statement: inside a string, a quoted name or a
// comment, or in plain text.
type quoteState struct {
	inString, inBacktick, inDouble, inLineComment bool
	blockComments                                 int
}

// inNameOrComment is true inside a quoted name or a comment.
func (state *quoteState) inNameOrComment() bool {
	return state.inBacktick || state.inDouble || state.inLineComment || state.blockComments > 0
}

// step copies the character at that offset, and the one after it where the two form one
// token, and returns the offset of the last character it copied.
func (state *quoteState) step(statement string, at int, bound *strings.Builder) int {
	char := statement[at]
	pairs := func(second byte) bool { return at+1 < len(statement) && statement[at+1] == second }
	copyPair := func() int {
		bound.WriteString(statement[at : at+2])
		return at + 1
	}
	switch {
	case state.inLineComment:
		state.inLineComment = char != '\n'
	case state.blockComments > 0:
		if char == '/' && pairs('*') {
			state.blockComments++
			return copyPair()
		}
		if char == '*' && pairs('/') {
			state.blockComments--
			return copyPair()
		}
	case state.inString || state.inBacktick || state.inDouble:
		quote, inside := byte('\''), &state.inString
		if state.inBacktick {
			quote, inside = '`', &state.inBacktick
		} else if state.inDouble {
			quote, inside = '"', &state.inDouble
		}
		if char == '\\' && at+1 < len(statement) {
			return copyPair()
		}
		if char == quote {
			if pairs(quote) {
				return copyPair()
			}
			*inside = false
		}
	case char == '\\' && at+1 < len(statement) && statement[at+1] != '?':
		return copyPair()
	case char == '\'':
		state.inString = true
	case char == '`':
		state.inBacktick = true
	case char == '"':
		state.inDouble = true
	case char == '#':
		state.inLineComment = true
	case char == '-' && pairs('-'):
		state.inLineComment = true
		return copyPair()
	case char == '/' && pairs('*'):
		state.blockComments++
		return copyPair()
	}
	bound.WriteByte(char)
	return at
}

// quoteLiteral writes a text as a string literal.
func quoteLiteral(text string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(text) + "'"
}

// formatLiteral writes one value as a literal of the server.
func formatLiteral(value any) (string, error) {
	if valuer, isValuer := value.(driver.Valuer); isValuer {
		inner, err := valuer.Value()
		if err != nil {
			return "", err
		}
		value = inner
	}
	switch held := value.(type) {
	case nil:
		return "NULL", nil
	case string:
		return quoteLiteral(held), nil
	case []byte:
		return "unhex('" + hex.EncodeToString(held) + "')", nil
	case time.Time:
		return formatTimeLiteral(held)
	case bool:
		if held {
			return "1", nil
		}
		return "0", nil
	case float32:
		return formatFloatLiteral(float64(held), "Float32"), nil
	case float64:
		return formatFloatLiteral(held, "Float64"), nil
	case fmt.Stringer:
		return quoteLiteral(held.String()), nil
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.String:
		return quoteLiteral(reflected.String()), nil
	case reflect.Slice, reflect.Array:
		elements := make([]string, 0, reflected.Len())
		for at := range reflected.Len() {
			element, err := formatLiteral(reflected.Index(at).Interface())
			if err != nil {
				return "", err
			}
			elements = append(elements, element)
		}
		return "[" + strings.Join(elements, ", ") + "]", nil
	case reflect.Map:
		pairs := make([]string, 0, reflected.Len())
		for _, key := range reflected.MapKeys() {
			name, err := formatLiteral(key.Interface())
			if err != nil {
				return "", err
			}
			element, err := formatLiteral(reflected.MapIndex(key).Interface())
			if err != nil {
				return "", err
			}
			pairs = append(pairs, name+", "+element)
		}
		return "map(" + strings.Join(pairs, ", ") + ")", nil
	case reflect.Pointer:
		if reflected.IsNil() {
			return "NULL", nil
		}
		return formatLiteral(reflected.Elem().Interface())
	}
	return fmt.Sprint(value), nil
}

// formatFloatLiteral writes a float as a cast to its type.
func formatFloatLiteral(value float64, typeName string) string {
	written := strconv.FormatFloat(value, 'g', -1, 64)
	if typeName == "Float32" {
		written = strconv.FormatFloat(value, 'g', -1, 32)
	}
	switch {
	case math.IsNaN(value):
		written = "'nan'"
	case math.IsInf(value, 1):
		written = "'inf'"
	case math.IsInf(value, -1):
		written = "'-inf'"
	}
	return "cast(" + written + ", '" + typeName + "')"
}

// formatTimeLiteral writes a moment with its time zone, and its fraction of a second where
// it holds one.
func formatTimeLiteral(moment time.Time) (string, error) {
	if moment.Location() == time.Local {
		moment = moment.UTC()
	}
	zone := moment.Location().String()
	if strings.ContainsAny(zone, `'\`) {
		return "", fmt.Errorf("invalid time zone %q", zone)
	}
	if moment.Nanosecond() == 0 {
		return fmt.Sprintf("toDateTime('%s', '%s')", moment.Format(time.DateTime), zone), nil
	}
	return fmt.Sprintf("toDateTime64('%s', 9, '%s')",
		moment.Format("2006-01-02 15:04:05.000000000"), zone), nil
}
