package present_test

import (
	"bytes"
	"testing"

	"github.com/masumedb/masume/internal/present"
)

// A long binary value is drawn as its first 32 bytes in hex and its size.
func TestFormatRowCapsALongBinaryValue(t *testing.T) {
	value := bytes.Repeat([]byte{0xab}, 5<<20)

	written := present.FormatRow([]any{value}, present.RowFormat{DataTypes: []string{"bytea"}})

	wanted := `\x` + string(bytes.Repeat([]byte("ab"), 32)) + "… (5MB)"
	if written[0] != wanted {
		t.Errorf("the cell reads %q, wanted %q", written[0], wanted)
	}
}

// A binary value of 32 bytes or less is drawn in full.
func TestFormatRowKeepsAShortBinaryValue(t *testing.T) {
	value := bytes.Repeat([]byte{0x01}, 32)

	written := present.FormatRow([]any{value}, present.RowFormat{DataTypes: []string{"bytea"}})

	wanted := `\x` + string(bytes.Repeat([]byte("01"), 32))
	if written[0] != wanted {
		t.Errorf("the cell reads %q, wanted %q", written[0], wanted)
	}
}

// The viewer shows the whole value.
func TestFormatForViewerKeepsTheWholeBinaryValue(t *testing.T) {
	value := bytes.Repeat([]byte{0xff}, 100)

	written := present.FormatForViewer(value, "bytea")

	if len(written) != len(`\x`)+200 {
		t.Errorf("the viewer shows %d characters, wanted %d", len(written), len(`\x`)+200)
	}
}
