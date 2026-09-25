package present

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MatchesSubsequence is true if every typed character is in the candidate in the same order,
// so `t88` finds `tenant_88231`.
func MatchesSubsequence(candidate, needle string) bool {
	if needle == "" {
		return true
	}
	haystack := strings.ToLower(candidate)
	wanted := strings.ToLower(needle)

	at := 0
	for _, character := range wanted {
		found := strings.IndexRune(haystack[at:], character)
		if found == -1 {
			return false
		}
		at += found + len(string(character))
	}
	return true
}

// MatchesText is true if the candidate contains the typed text, without case comparison. A
// filter over a list uses this, and the object tree uses a subsequence.
func MatchesText(candidate, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(candidate), strings.ToLower(needle))
}

// The scores of one typed word of a command search. A lower score ranks first.
const (
	scoreLeadWord   = -1
	scoreLabelWord  = 0
	scoreDetailWord = 2
	scoreLabelChars = 3
)

// ScoreCommandMatch ranks a command against the typed words. Each word must start a word of
// the label or of the detail, or be a subsequence of the label. A word also matches without
// its plural "s". It returns false where one word matches nothing.
func ScoreCommandMatch(label, detail, term string) (int, bool) {
	labelWords, detailWords := splitMatchWords(label), splitMatchWords(detail)
	typedWords := strings.Fields(strings.ToLower(term))
	score := 0
	if len(labelWords) > 0 && len(typedWords) > 0 &&
		startsAnyWord(labelWords[:1], typedWords[0]) {
		score = scoreLeadWord
	}
	for _, typed := range typedWords {
		switch {
		case startsAnyWord(labelWords, typed):
			score += scoreLabelWord
		case startsAnyWord(detailWords, typed):
			score += scoreDetailWord
		case MatchesSubsequence(label, typed):
			score += scoreLabelChars
		default:
			return 0, false
		}
	}
	return score, true
}

// splitMatchWords returns the lowercase words of a text, split at every character that is not
// a letter or a digit.
func splitMatchWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	})
}

// startsAnyWord is true where the typed word, or the typed word without a plural "s", starts
// one of the words.
func startsAnyWord(words []string, typed string) bool {
	singular := strings.TrimSuffix(typed, "s")
	for _, word := range words {
		if strings.HasPrefix(word, typed) || (len(singular) > 2 && strings.HasPrefix(word, singular)) {
			return true
		}
	}
	return false
}

// FindTextSpan returns the byte range of the first place the candidate contains the typed
// text, without case comparison.
func FindTextSpan(candidate, needle string) (int, int, bool) {
	count := utf8.RuneCountInString(needle)
	if count == 0 {
		return 0, 0, false
	}
	for from := range candidate {
		to, taken := from, 0
		for to < len(candidate) && taken < count {
			_, size := utf8.DecodeRuneInString(candidate[to:])
			to += size
			taken++
		}
		if taken < count {
			break
		}
		if strings.EqualFold(candidate[from:to], needle) {
			return from, to, true
		}
	}
	return 0, 0, false
}

// MaskedDisplay is drawn in place of a hidden value.
const MaskedDisplay = "••••••"

// DefaultMaskedNames match at any position in the name, without case comparison. `pin` is
// not in the list, because it is a part of `shipping`.
var DefaultMaskedNames = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey", "private_key",
	"credit_card", "card_number", "cvv", "iban", "ssn",
}

// MaskingRules is the masking setting: the on or off state, and the names that select a
// column.
type MaskingRules struct {
	Enabled bool
	Names   []string
}

// DefaultMasking hides the value of a column whose name indicates a secret. The value is
// still read, and a copy or an export contains it.
func DefaultMasking() MaskingRules {
	return MaskingRules{Enabled: true, Names: DefaultMaskedNames}
}

// FindMaskedColumns runs one time per result and not per cell, because the result is the
// same for every row.
func FindMaskedColumns(columnNames []string, rules MaskingRules) map[int]bool {
	masked := map[int]bool{}
	if !rules.Enabled {
		return masked
	}
	// The names are converted to lower case one time here and not for every column.
	markers := make([]string, 0, len(rules.Names))
	for _, marker := range rules.Names {
		if marker != "" {
			markers = append(markers, strings.ToLower(marker))
		}
	}

	for index, name := range columnNames {
		lowered := strings.ToLower(name)
		for _, marker := range markers {
			if strings.Contains(lowered, marker) {
				masked[index] = true
				break
			}
		}
	}
	return masked
}
