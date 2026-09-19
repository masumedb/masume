package load

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// Import column types come from the sampled values.

// timestampLayouts are the supported timestamp formats, with the most precise first.
var timestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	slashLayout,
}

// slashLayout is a date written with slashes, day first. A file written month first holds
// the same shape, so a column of these is a date only where one value of it has a first
// number over twelve.
const slashLayout = "02/01/2006"

// booleanWords are the supported boolean strings and their values.
var booleanWords = map[string]bool{
	"true": true, "false": false, "t": true, "f": false,
	"yes": true, "no": false, "y": true, "n": false, "1": true, "0": false,
}

// ReadValueKind returns the type of an import value, or false for null.
func ReadValueKind(value any) (core.ColumnKind, bool) {
	switch held := value.(type) {
	case nil:
		return "", false
	case bool:
		return core.KindBoolean, true
	case json.Number:
		if _, err := strconv.ParseInt(held.String(), 10, 64); err == nil {
			return core.KindInteger, true
		}
		if holdsWholeDigits(held.String()) {
			return core.KindText, true
		}
		return core.KindNumber, true
	case string:
		return readTextKind(held), true
	}
	return core.KindText, true
}

// readTextKind infers a field type. Numbers with leading zeros remain text.
func readTextKind(written string) core.ColumnKind {
	trimmed := strings.TrimSpace(written)
	if trimmed == "" {
		return core.KindText
	}
	if _, known := booleanWords[strings.ToLower(trimmed)]; known && !holdsDigitOnly(trimmed) {
		return core.KindBoolean
	}
	if !holdsLeadingZero(trimmed) {
		if _, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return core.KindInteger
		}
		if holdsWholeDigits(trimmed) {
			return core.KindText
		}
		if _, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return core.KindNumber
		}
	}
	if _, read := ReadTimestamp(trimmed); read && !holdsSlashDate(trimmed) {
		return core.KindTimestamp
	}
	return core.KindText
}

// holdsSlashDate is true for a date written with slashes.
func holdsSlashDate(written string) bool {
	_, err := time.Parse(slashLayout, written)
	return err == nil
}

// provesDayFirst is true for a slash date whose first number is over twelve, which no month
// is, so the file writes the day first.
func provesDayFirst(written string) bool {
	if !holdsSlashDate(written) {
		return false
	}
	day, err := strconv.Atoi(written[:strings.Index(written, "/")])
	return err == nil && day > 12
}

// holdsDigitOnly is true for non-empty text containing only digits.
func holdsDigitOnly(written string) bool {
	for _, held := range written {
		if held < '0' || held > '9' {
			return false
		}
	}
	return written != ""
}

// holdsLeadingZero is true for a signed or unsigned number with a leading zero before another non-decimal character.
func holdsLeadingZero(written string) bool {
	held := strings.TrimPrefix(strings.TrimPrefix(written, "-"), "+")
	return len(held) > 1 && held[0] == '0' && held[1] != '.'
}

// ReadTimestamp parses a timestamp written in any of the forms a data file uses.
func ReadTimestamp(written string) (time.Time, bool) {
	for _, layout := range timestampLayouts {
		if held, err := time.Parse(layout, written); err == nil {
			return held, true
		}
	}
	return time.Time{}, false
}

// ResolveColumnKind returns a type for all column values. An all-null column uses text.
func ResolveColumnKind(values []any) core.ColumnKind {
	if holdsDayFirstDates(values) {
		return core.KindTimestamp
	}
	resolved := core.ColumnKind("")
	for _, value := range values {
		kind, holds := ReadValueKind(value)
		if !holds {
			continue
		}
		resolved = core.ResolveWiderKind(resolved, kind)
		if resolved == core.KindText {
			return core.KindText
		}
	}
	if resolved == "" {
		return core.KindText
	}
	return resolved
}

// holdsDayFirstDates is true for a column of slash dates where one of them proves that the
// file writes the day first. A column of dates that could be read either way is text, so no
// date is written the wrong way round.
func holdsDayFirstDates(values []any) bool {
	proven := false
	held := 0
	for _, value := range values {
		written, isText := value.(string)
		if value == nil || (isText && strings.TrimSpace(written) == "") {
			continue
		}
		if !isText || !holdsSlashDate(strings.TrimSpace(written)) {
			return false
		}
		held++
		if provesDayFirst(strings.TrimSpace(written)) {
			proven = true
		}
	}
	return proven && held > 0
}

// ValueError is an import value conversion error.
type ValueError struct{ Reason string }

func (err ValueError) Error() string { return err.Reason }

// CastValue converts an import value to the target column type.
func CastValue(value any, kind core.ColumnKind) (any, error) {
	if value == nil {
		return nil, nil
	}

	written := ""
	switch held := value.(type) {
	case string:
		// A text column takes the value as the file wrote it, with the blanks it holds.
		if kind == core.KindText {
			return held, nil
		}
		written = strings.TrimSpace(held)
	case json.Number:
		written = held.String()
	case bool:
		if kind == core.KindBoolean || kind == core.KindText {
			return castBoolean(held, kind), nil
		}
		return nil, failValue("cannot convert %v to %s", held, kind)
	default:
		return value, nil
	}

	if written == "" {
		return nil, nil
	}
	return castText(written, kind)
}

// castBoolean returns a boolean as its own kind, or as text.
func castBoolean(held bool, kind core.ColumnKind) any {
	if kind == core.KindText {
		return strconv.FormatBool(held)
	}
	return held
}

func castText(written string, kind core.ColumnKind) (any, error) {
	switch kind {
	case core.KindText:
		return written, nil
	case core.KindInteger:
		held, err := strconv.ParseInt(written, 10, 64)
		if err != nil {
			return nil, failValue("invalid integer %q", written)
		}
		return held, nil
	case core.KindNumber:
		held, err := strconv.ParseFloat(written, 64)
		if err != nil {
			return nil, failValue("invalid number %q", written)
		}
		return held, nil
	case core.KindBoolean:
		held, known := booleanWords[strings.ToLower(written)]
		if !known {
			return nil, failValue("invalid boolean %q", written)
		}
		return held, nil
	case core.KindTimestamp:
		held, read := ReadTimestamp(written)
		if !read {
			return nil, failValue("invalid date or time %q", written)
		}
		return held, nil
	}
	return written, nil
}

func failValue(format string, parts ...any) error {
	return ValueError{Reason: fmt.Sprintf(format, parts...)}
}

// holdsWholeDigits is true for a signed or unsigned integer string.
func holdsWholeDigits(written string) bool {
	digits := strings.TrimPrefix(strings.TrimPrefix(written, "-"), "+")
	if digits == "" {
		return false
	}
	for _, letter := range digits {
		if letter < '0' || letter > '9' {
			return false
		}
	}
	return true
}
