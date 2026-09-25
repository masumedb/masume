package present_test

import (
	"testing"

	"github.com/masumedb/masume/internal/present"
)

func TestAlignDecimalPadsWithSpacesAndKeepsTheDigits(t *testing.T) {
	for _, held := range []struct {
		text     string
		fraction int
		want     string
	}{
		{"51395.99", 2, "51395.99"},
		{"51140.3", 2, "51140.3 "},
		{"496", 2, "496   "},
		{"103.62094758064515", 14, "103.62094758064515"},
		{"-1.5", 3, "-1.5  "},
		{"1e10", 2, "1e10"},
		{"∅", 2, "∅"},
		{"12", 0, "12"},
	} {
		if got := present.AlignDecimal(held.text, held.fraction); got != held.want {
			t.Errorf("AlignDecimal(%q, %d) = %q, wanted %q", held.text, held.fraction, got, held.want)
		}
	}
}
