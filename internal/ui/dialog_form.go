package ui

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/masumedb/masume/internal/present"
)

// The rows a dialog form draws. The export form and the import form are both built from
// these, so a row is stepped through and typed into the same way in each.

// DialogField is one row of a form a dialog draws.
type DialogField struct {
	Key   string
	Label string
	Value string
	// Choices are the values the field steps through. A typed field has none.
	Choices []string
}

// describeFieldValue returns the value a row shows while the cursor is elsewhere. A value
// with no letter or digit, an empty one too, is quoted.
func describeFieldValue(field DialogField) string {
	if len(field.Choices) == 0 && !strings.ContainsFunc(field.Value, isLetterOrDigit) {
		return strconv.Quote(field.Value)
	}
	return field.Value
}

// truncateFieldValue returns the value a row shows while the cursor is elsewhere, cut to the
// width. A file path is cut from the start.
func truncateFieldValue(field DialogField, width int) string {
	if field.Key == "path" {
		return present.TruncatePath(describeFieldValue(field), width)
	}
	return present.TruncateText(describeFieldValue(field), width)
}

func isLetterOrDigit(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character)
}

// The two answers a yes-or-no field steps through.
var yesOrNo = []string{"yes", "no"}

// describeYesOrNo returns a flag as the word a form shows it with.
func describeYesOrNo(held bool) string {
	if held {
		return "yes"
	}
	return "no"
}
