package present_test

import (
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// A moment in a zoned column is written in the zone of the setting, with the offset. No zone
// keeps the zone of the value. A timestamp without a time zone is written in UTC with no
// offset, as before.
func TestFormatRowWritesAZonedMomentInTheZoneWithTheOffset(t *testing.T) {
	kolkata := time.FixedZone("IST", 5*60*60+30*60)
	moment := time.Date(2026, 9, 23, 18, 0, 0, 0, kolkata)
	berlin := time.FixedZone("CEST", 2*60*60)
	row := []any{moment, moment}
	format := present.RowFormat{
		DataTypes: []string{"timestamptz", "timestamp"},
		Zoned:     map[int]bool{0: true},
	}

	for _, held := range []struct {
		zone          *time.Location
		zoned, native string
	}{
		{nil, "2026-09-23 18:00:00.000 +05:30", "2026-09-23 12:30:00.000"},
		{time.UTC, "2026-09-23 12:30:00.000 +00:00", "2026-09-23 12:30:00.000"},
		{berlin, "2026-09-23 14:30:00.000 +02:00", "2026-09-23 12:30:00.000"},
	} {
		format.Zone = held.zone
		written := present.FormatRow(row, format)
		if written[0] != held.zoned {
			t.Errorf("the zoned cell reads %q, wanted %q", written[0], held.zoned)
		}
		if written[1] != held.native {
			t.Errorf("the timestamp cell reads %q, wanted %q", written[1], held.native)
		}
	}
}

// The viewer writes a moment of a zoned column as the grid writes it, and any other column
// as before.
func TestFormatColumnForViewerWritesAZonedMomentAsTheGrid(t *testing.T) {
	kolkata := time.FixedZone("IST", 5*60*60+30*60)
	moment := time.Date(2026, 9, 23, 18, 0, 0, 0, kolkata)
	zoned := db.ResultColumn{Name: "placed", DataType: "timestamptz", Zoned: true}
	native := db.ResultColumn{Name: "noted", DataType: "timestamp"}

	if written := present.FormatColumnForViewer(moment, zoned, nil); written != "2026-09-23 18:00:00.000 +05:30" {
		t.Errorf("the zoned value reads %q", written)
	}
	if written := present.FormatColumnForViewer(moment, zoned, time.UTC); written != "2026-09-23 12:30:00.000 +00:00" {
		t.Errorf("the zoned value in UTC reads %q", written)
	}
	if written := present.FormatColumnForViewer(moment, native, nil); written != "2026-09-23 12:30:00.000" {
		t.Errorf("the timestamp value reads %q", written)
	}
}
