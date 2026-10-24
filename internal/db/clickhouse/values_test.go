package clickhouse

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/core"
)

// A field keeps every digit of a wide number, and the type the grid formats it by.
func TestDecodeFieldKeepsEveryDigit(t *testing.T) {
	decoder := valueDecoder{zone: "UTC"}
	for _, held := range []struct {
		field    string
		dataType string
		want     any
	}{
		{"18446744073709551615", "UInt64", uint64(18446744073709551615)},
		{"-9223372036854775808", "Int64", int64(-9223372036854775808)},
		{"-128", "Int8", int8(-128)},
		{"340282366920938463463374607431768211455", "UInt128",
			json.Number("340282366920938463463374607431768211455")},
		{"12345678901234567890.123456789", "Decimal(38, 9)", "12345678901234567890.123456789"},
		{"12.5", "Nullable(Decimal(10, 2))", "12.5"},
		{"0.1", "Float64", 0.1},
		{"0.1", "Float32", float32(0.1)},
		{"true", "Bool", true},
		{`\N`, "Nullable(String)", nil},
		{`\\N`, "Nullable(String)", `\N`},
		{"x", "LowCardinality(Nullable(String))", "x"},
		{"61f0c404-5cb3-11e7-907b-a6006ad3dba0", "UUID", "61f0c404-5cb3-11e7-907b-a6006ad3dba0"},
	} {
		value := decoder.decodeField([]byte(held.field), parseColumnType(held.dataType))
		if value != held.want {
			t.Errorf("%s %q reads as %#v, wanted %#v", held.dataType, held.field, value, held.want)
		}
	}
}

// A field of text reads back the bytes the server escaped.
func TestDecodeFieldReadsTheEscapesOfTheServer(t *testing.T) {
	value := valueDecoder{}.decodeField([]byte(`tab\tnew\nline \\ it\'s \0\xff`),
		parseColumnType("String"))
	if value != "tab\tnew\nline \\ it's \x00\xff" {
		t.Errorf("the text reads as %q", value)
	}
}

// A moment keeps its fraction and the time zone of its column, or of the server where the
// column names none.
func TestDecodeFieldReadsAMomentInItsZone(t *testing.T) {
	decoder := valueDecoder{zone: "Europe/Berlin"}
	for _, held := range []struct {
		field    string
		dataType string
		want     string
	}{
		{"1790132400", "DateTime('Asia/Tokyo')", "2026-09-23 12:00:00+09:00"},
		{"1790132400.125", "DateTime64(3, 'Asia/Tokyo')", "2026-09-23 12:00:00.125+09:00"},
		{"1790132400", "DateTime", "2026-09-23 05:00:00+02:00"},
		{"-2208988799.5", "DateTime64(1, 'UTC')", "1900-01-01 00:00:00.5+00:00"},
		{"10413791999.999999", "DateTime64(6, 'UTC')", "2299-12-31 23:59:59.999999+00:00"},
	} {
		value := decoder.decodeField([]byte(held.field), parseColumnType(held.dataType))
		moment, isTime := value.(time.Time)
		if !isTime {
			t.Errorf("%s %q reads as %#v", held.dataType, held.field, value)
			continue
		}
		if written := core.FormatZonedTime(moment); written != held.want {
			t.Errorf("%s %q reads as %s, wanted %s", held.dataType, held.field, written, held.want)
		}
	}
	date := valueDecoder{}.decodeField([]byte("2026-01-02"), parseColumnType("Date"))
	if written := core.FormatCell(date, "Date"); written != "2026-01-02" {
		t.Errorf("the date reads as %q", written)
	}
}

// A list reads as the JSON the grid shows: an array, a map, a tuple, and an array of bytes.
func TestDecodeFieldReadsAList(t *testing.T) {
	decoder := valueDecoder{zone: "UTC"}
	for _, held := range []struct {
		field    string
		dataType string
		want     string
	}{
		{`['a','b\'c\t',NULL]`, "Array(Nullable(String))", `["a","b'c\t",null]`},
		{`[1.5,-2]`, "Array(Decimal(9, 2))", `["1.5","-2"]`},
		{`[[1],[2,3]]`, "Array(Array(UInt8))", `[[1],[2,3]]`},
		{`{'z':1,'a':2}`, "Map(String, UInt64)", `{"a":2,"z":1}`},
		{`{1:'x'}`, "Map(UInt8, String)", `{"1":"x"}`},
		{`(1,'a')`, "Tuple(UInt8, String)", `[1,"a"]`},
		{`(1,'a')`, "Tuple(id UInt8, name String)", `{"id":1,"name":"a"}`},
		{`(1.5,2.5)`, "Point", `[1.5,2.5]`},
		{`['1767225600']`, "Array(DateTime('Asia/Tokyo'))", `["2026-01-01T09:00:00+09:00"]`},
		{`[5]`, "Array(Int128)", `[5]`},
		{`{'k':[1,2]}`, "Map(String, Array(UInt8))", `{"k":[1,2]}`},
		{`[1,2]`, "Array(UInt8)", `\x0102`},
		{`{"a":[1,2],"b":2}`, "JSON", `{"a":[1,2],"b":2}`},
	} {
		value := decoder.decodeField([]byte(held.field), parseColumnType(held.dataType))
		if written := core.FormatCell(value, held.dataType); written != held.want {
			t.Errorf("%s %q reads as %s, wanted %s", held.dataType, held.field, written, held.want)
		}
	}
}
