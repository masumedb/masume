package present_test

import (
	"testing"

	"github.com/masumedb/masume/internal/present"
)

func TestMatchesTextIgnoresLetterCase(t *testing.T) {
	if !present.MatchesText("CustomerName", "name") {
		t.Error("a name that holds the typed text was missed")
	}
	if present.MatchesText("CustomerName", "xyz") {
		t.Error("a name that does not hold the typed text matched")
	}
	if !present.MatchesText("anything", "") {
		t.Error("an empty needle missed")
	}
}

func TestFindTextSpanReturnsTheFirstMatch(t *testing.T) {
	for _, held := range []struct {
		candidate, needle string
		from, to          int
		found             bool
	}{
		{candidate: "Run every statement", needle: "EVERY", from: 4, to: 9, found: true},
		{candidate: "Größe größe", needle: "grö", from: 0, to: 4, found: true},
		{candidate: "Run", needle: "xyz"},
		{candidate: "Run", needle: ""},
		{candidate: "Ru", needle: "Run"},
	} {
		from, to, found := present.FindTextSpan(held.candidate, held.needle)
		if from != held.from || to != held.to || found != held.found {
			t.Errorf("FindTextSpan(%q, %q) = %d, %d, %v", held.candidate, held.needle,
				from, to, found)
		}
	}
}

func TestFindMaskedColumnsHidesSecretNames(t *testing.T) {
	masked := present.FindMaskedColumns(
		[]string{"id", "password_hash", "shipping_pin", "api_key"},
		present.DefaultMasking())
	if !masked[1] || !masked[3] {
		t.Errorf("secret columns were not masked: %v", masked)
	}
	if masked[0] {
		t.Error("id was masked")
	}
	if masked[2] {
		t.Error("shipping_pin was masked, and pin is left out of the default names")
	}
}

func TestFindMaskedColumnsDoesNothingWhenOff(t *testing.T) {
	masked := present.FindMaskedColumns(
		[]string{"password"},
		present.MaskingRules{Enabled: false, Names: []string{"password"}})
	if len(masked) != 0 {
		t.Errorf("masking that is off still hid columns: %v", masked)
	}
}

func TestScoreCommandMatchFindsEachTypedWord(t *testing.T) {
	for _, held := range []struct {
		label, detail, term string
		matched             bool
	}{
		{"Open the connection picker", "", "connections", true},
		{"Export the result as CSV", "", "exp csv", true},
		{"Export the result as CSV", "", "csv exp", true},
		{"Close the connection", "closes all its tabs", "tabs", true},
		{"Explain plan", "", "xpl", true},
		{"Explain plan", "", "export", false},
	} {
		if _, matched := present.ScoreCommandMatch(held.label, held.detail, held.term); matched != held.matched {
			t.Errorf("%q against %q matched %v, wanted %v", held.term, held.label, matched, held.matched)
		}
	}
}

func TestScoreCommandMatchRanksALabelThatStartsWithTheWordFirst(t *testing.T) {
	lead, _ := present.ScoreCommandMatch("Explain plan", "", "exp")
	inside, _ := present.ScoreCommandMatch("Ask AI: explain this query", "", "exp")
	detail, _ := present.ScoreCommandMatch("Run every statement", "one result per explained", "exp")
	if lead >= inside || inside >= detail {
		t.Errorf("the scores are %d, %d and %d, wanted them rising", lead, inside, detail)
	}
}
