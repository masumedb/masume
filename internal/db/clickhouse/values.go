package clickhouse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/masumedb/masume/internal/core"
)

// columnType is a ClickHouse type, read out of the name the server writes for it.
type columnType struct {
	name string
	// The element types of a Nullable, LowCardinality, Array, Map or Tuple.
	inner []columnType
	// The element names of a named tuple.
	fields []string
	// The time zone of a DateTime or a DateTime64; empty for the zone of the server.
	zone string
}

// geoTypes are the geometry types, each written as the type it stands for.
var geoTypes = map[string]string{
	"Point": "Tuple(Float64, Float64)", "Ring": "Array(Point)", "LineString": "Array(Point)",
	"MultiLineString": "Array(Ring)", "Polygon": "Array(Ring)", "MultiPolygon": "Array(Polygon)",
}

// parseColumnType reads a type name such as `Array(Nullable(DateTime64(3, 'UTC')))`.
func parseColumnType(written string) columnType {
	written = strings.TrimSpace(written)
	if alias, isGeo := geoTypes[written]; isGeo {
		return parseColumnType(alias)
	}
	open := strings.IndexByte(written, '(')
	if open == -1 || !strings.HasSuffix(written, ")") {
		return columnType{name: written}
	}
	parsed := columnType{name: written[:open]}
	args := splitTypeArguments(written[open+1 : len(written)-1])
	switch parsed.name {
	case "Nullable", "LowCardinality", "Array":
		parsed.inner = []columnType{parseColumnType(args[0])}
	case "Map":
		if len(args) != 2 {
			return columnType{name: written}
		}
		parsed.inner = []columnType{parseColumnType(args[0]), parseColumnType(args[1])}
	case "Tuple", "Nested":
		for _, arg := range args {
			field, element := splitTupleElement(arg)
			parsed.fields = append(parsed.fields, field)
			parsed.inner = append(parsed.inner, parseColumnType(element))
		}
		if hasEmptyField(parsed.fields) {
			parsed.fields = nil
		}
	case "SimpleAggregateFunction":
		return parseColumnType(args[len(args)-1])
	case "DateTime":
		parsed.zone = readQuotedArgument(args, 0)
	case "DateTime64":
		parsed.zone = readQuotedArgument(args, 1)
	}
	return parsed
}

// hasEmptyField is true for a tuple without element names.
func hasEmptyField(fields []string) bool {
	for _, field := range fields {
		if field == "" {
			return true
		}
	}
	return false
}

// readQuotedArgument returns the text of a quoted type argument, or nothing.
func readQuotedArgument(args []string, at int) string {
	if at >= len(args) {
		return ""
	}
	held := strings.TrimSpace(args[at])
	if len(held) < 2 || held[0] != '\'' || held[len(held)-1] != '\'' {
		return ""
	}
	return unescapeText(held[1 : len(held)-1])
}

// splitTypeArguments splits the arguments of a type at the commas outside parentheses and
// quotes.
func splitTypeArguments(written string) []string {
	args := []string{}
	depth, start, quote := 0, 0, byte(0)
	for at := 0; at < len(written); at++ {
		char := written[at]
		switch {
		case quote != 0:
			if char == '\\' {
				at++
			} else if char == quote {
				quote = 0
			}
		case char == '\'' || char == '`' || char == '"':
			quote = char
		case char == '(':
			depth++
		case char == ')':
			depth--
		case char == ',' && depth == 0:
			args = append(args, strings.TrimSpace(written[start:at]))
			start = at + 1
		}
	}
	return append(args, strings.TrimSpace(written[start:]))
}

// splitTupleElement splits `name Type` into its name and its type. An element without a
// name returns an empty name.
func splitTupleElement(written string) (string, string) {
	if strings.HasPrefix(written, "`") {
		for at := 1; at < len(written); at++ {
			if written[at] == '\\' {
				at++
				continue
			}
			if written[at] == '`' {
				return unescapeText(written[1:at]), strings.TrimSpace(written[at+1:])
			}
		}
		return "", written
	}
	space := strings.IndexByte(written, ' ')
	open := strings.IndexByte(written, '(')
	if space == -1 || (open != -1 && open < space) {
		return "", written
	}
	return written[:space], strings.TrimSpace(written[space+1:])
}

// isComposite is true for a type the server writes as a bracketed list.
func (held columnType) isComposite() bool {
	switch held.name {
	case "Array", "Map", "Tuple", "Nested":
		return true
	}
	return false
}

// isByteArray is true for an Array(UInt8), which reads as bytes.
func (held columnType) isByteArray() bool {
	return held.name == "Array" && len(held.inner) == 1 && held.inner[0].unwrap().name == "UInt8"
}

// unwrap returns the type under a Nullable and a LowCardinality.
func (held columnType) unwrap() columnType {
	for (held.name == "Nullable" || held.name == "LowCardinality") && len(held.inner) == 1 {
		held = held.inner[0]
	}
	return held
}

// zoneCache holds every time zone read so far, by name.
var zoneCache sync.Map

// loadZone returns the time zone of that name, or UTC where the system holds no such zone.
func loadZone(name string) *time.Location {
	if name == "" || name == "UTC" {
		return time.UTC
	}
	if held, found := zoneCache.Load(name); found {
		return held.(*time.Location)
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		zone = time.UTC
	}
	zoneCache.Store(name, zone)
	return zone
}

// valueDecoder reads the values of one result. The zone is the zone of a DateTime column
// whose type names none.
type valueDecoder struct {
	zone string
}

// decodeField reads one field of a TabSeparated row.
func (decoder valueDecoder) decodeField(field []byte, held columnType) any {
	if string(field) == `\N` {
		return nil
	}
	base := held.unwrap()
	if !base.isComposite() {
		return decoder.decodeScalar(unescapeText(string(field)), base)
	}
	parser := &literalParser{text: string(field)}
	value, err := parser.readValue(decoder, base)
	if err != nil {
		return unescapeText(string(field))
	}
	if base.isByteArray() {
		return readByteArray(value)
	}
	return value
}

// readByteArray returns an array of UInt8 as bytes.
func readByteArray(value any) any {
	elements, isList := value.([]any)
	if !isList {
		return value
	}
	written := make([]byte, 0, len(elements))
	for _, element := range elements {
		held, isByte := element.(uint8)
		if !isByte {
			return value
		}
		written = append(written, held)
	}
	return written
}

// decodeScalar reads the text of one value of a type that is not a list. A text that does
// not read as its type is returned as it is.
func (decoder valueDecoder) decodeScalar(text string, held columnType) any {
	value, err := decoder.parseScalar(text, held)
	if err != nil {
		return text
	}
	return value
}

func (decoder valueDecoder) parseScalar(text string, held columnType) (any, error) {
	switch held.name {
	case "Nothing":
		return nil, nil
	case "Int8":
		value, err := strconv.ParseInt(text, 10, 8)
		return int8(value), err
	case "Int16":
		value, err := strconv.ParseInt(text, 10, 16)
		return int16(value), err
	case "Int32":
		value, err := strconv.ParseInt(text, 10, 32)
		return int32(value), err
	case "Int64":
		return strconv.ParseInt(text, 10, 64)
	case "UInt8":
		value, err := strconv.ParseUint(text, 10, 8)
		return uint8(value), err
	case "UInt16":
		value, err := strconv.ParseUint(text, 10, 16)
		return uint16(value), err
	case "UInt32":
		value, err := strconv.ParseUint(text, 10, 32)
		return uint32(value), err
	case "UInt64":
		return strconv.ParseUint(text, 10, 64)
	case "Int128", "Int256", "UInt128", "UInt256":
		return json.Number(text), nil
	case "Float32", "BFloat16":
		value, err := strconv.ParseFloat(text, 32)
		return float32(value), err
	case "Float64":
		return strconv.ParseFloat(text, 64)
	case "Bool":
		return strconv.ParseBool(text)
	case "Date", "Date32":
		return time.ParseInLocation(time.DateOnly, text, time.UTC)
	case "DateTime", "DateTime64":
		zone := held.zone
		if zone == "" {
			zone = decoder.zone
		}
		return parseUnixMoment(text, loadZone(zone))
	case "JSON", "Object":
		if json.Valid([]byte(text)) {
			return json.RawMessage(text), nil
		}
	}
	return text, nil
}

// parseUnixMoment reads a moment written as seconds since 1970, with an optional fraction.
func parseUnixMoment(text string, zone *time.Location) (time.Time, error) {
	whole, fraction, _ := strings.Cut(text, ".")
	seconds, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	nanoseconds := int64(0)
	if fraction != "" {
		if len(fraction) > 9 {
			fraction = fraction[:9]
		}
		nanoseconds, err = strconv.ParseInt(fraction+strings.Repeat("0", 9-len(fraction)), 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		if strings.HasPrefix(whole, "-") {
			nanoseconds = -nanoseconds
		}
	}
	return time.Unix(seconds, nanoseconds).In(zone), nil
}

// literalParser reads a list value in the text form of the server: `[1,2]`, `{'k':1}`,
// `(1,'a')`, with each string in single quotes.
type literalParser struct {
	text string
	at   int
}

func (parser *literalParser) readValue(decoder valueDecoder, held columnType) (any, error) {
	base := held.unwrap()
	switch base.name {
	case "Array":
		return parser.readArray(decoder, base.inner[0])
	case "Nested":
		return parser.readArray(decoder, columnType{name: "Tuple", inner: base.inner, fields: base.fields})
	case "Map":
		return parser.readMap(decoder, base)
	case "Tuple":
		return parser.readTuple(decoder, base)
	}
	if strings.HasPrefix(parser.text[parser.at:], "NULL") {
		parser.at += len("NULL")
		return nil, nil
	}
	if parser.at < len(parser.text) && parser.text[parser.at] == '\'' {
		text, err := parser.readQuoted()
		if err != nil {
			return nil, err
		}
		return decoder.decodeScalar(text, base), nil
	}
	return decoder.decodeScalar(parser.readBare(), base), nil
}

// expect steps over one character that must stand at the cursor.
func (parser *literalParser) expect(char byte) error {
	if parser.at >= len(parser.text) || parser.text[parser.at] != char {
		return fmt.Errorf("expected %q at offset %d", char, parser.at)
	}
	parser.at++
	return nil
}

// readList reads the elements between two brackets, separated by commas.
func (parser *literalParser) readList(open, close byte, readElement func() error) error {
	if err := parser.expect(open); err != nil {
		return err
	}
	if parser.at < len(parser.text) && parser.text[parser.at] == close {
		parser.at++
		return nil
	}
	for {
		if err := readElement(); err != nil {
			return err
		}
		if parser.at < len(parser.text) && parser.text[parser.at] == ',' {
			parser.at++
			continue
		}
		return parser.expect(close)
	}
}

func (parser *literalParser) readArray(decoder valueDecoder, element columnType) (any, error) {
	values := []any{}
	err := parser.readList('[', ']', func() error {
		value, err := parser.readValue(decoder, element)
		values = append(values, value)
		return err
	})
	return values, err
}

func (parser *literalParser) readMap(decoder valueDecoder, held columnType) (any, error) {
	values := map[string]any{}
	err := parser.readList('{', '}', func() error {
		key, err := parser.readValue(decoder, held.inner[0])
		if err != nil {
			return err
		}
		if err := parser.expect(':'); err != nil {
			return err
		}
		value, err := parser.readValue(decoder, held.inner[1])
		values[formatMapKey(key)] = value
		return err
	})
	return values, err
}

// formatMapKey writes a map key as JSON writes the key of a map.
func formatMapKey(key any) string {
	if moment, isTime := key.(time.Time); isTime {
		return moment.Format(time.RFC3339Nano)
	}
	return core.FormatCell(key, "")
}

func (parser *literalParser) readTuple(decoder valueDecoder, held columnType) (any, error) {
	values := []any{}
	err := parser.readList('(', ')', func() error {
		element := columnType{name: "String"}
		if len(values) < len(held.inner) {
			element = held.inner[len(values)]
		}
		value, err := parser.readValue(decoder, element)
		values = append(values, value)
		return err
	})
	if err != nil || held.fields == nil {
		return values, err
	}
	named := map[string]any{}
	for at, value := range values {
		if at < len(held.fields) {
			named[held.fields[at]] = value
		}
	}
	return named, nil
}

// readQuoted reads a string in single quotes, with the escapes of the server.
func (parser *literalParser) readQuoted() (string, error) {
	start := parser.at + 1
	for at := start; at < len(parser.text); at++ {
		switch parser.text[at] {
		case '\\':
			at++
		case '\'':
			parser.at = at + 1
			return unescapeText(parser.text[start:at]), nil
		}
	}
	return "", fmt.Errorf("unterminated string at offset %d", parser.at)
}

// readBare reads a value without quotes up to the comma or the bracket that ends it. A JSON
// value inside the list keeps its own brackets and strings.
func (parser *literalParser) readBare() string {
	start, depth, inString := parser.at, 0, false
	for ; parser.at < len(parser.text); parser.at++ {
		char := parser.text[parser.at]
		switch {
		case inString:
			if char == '\\' {
				parser.at++
			} else if char == '"' {
				inString = false
			}
		case char == '"':
			inString = true
		case char == '[' || char == '{' || char == '(':
			depth++
		case char == ']' || char == '}' || char == ')':
			if depth == 0 {
				return parser.text[start:parser.at]
			}
			depth--
		case (char == ',' || char == ':') && depth == 0:
			return parser.text[start:parser.at]
		}
	}
	return parser.text[start:]
}

// unescapeText reads the backslash escapes the server writes in a field and in a quoted
// string.
func unescapeText(written string) string {
	if !strings.Contains(written, `\`) {
		return written
	}
	var plain strings.Builder
	plain.Grow(len(written))
	for at := 0; at < len(written); at++ {
		char := written[at]
		if char != '\\' || at+1 == len(written) {
			plain.WriteByte(char)
			continue
		}
		at++
		switch written[at] {
		case 'b':
			plain.WriteByte('\b')
		case 'f':
			plain.WriteByte('\f')
		case 'n':
			plain.WriteByte('\n')
		case 'r':
			plain.WriteByte('\r')
		case 't':
			plain.WriteByte('\t')
		case '0':
			plain.WriteByte(0)
		case 'a':
			plain.WriteByte('\a')
		case 'v':
			plain.WriteByte('\v')
		case 'e':
			plain.WriteByte(0x1b)
		case 'x':
			if at+2 < len(written) {
				if value, err := strconv.ParseUint(written[at+1:at+3], 16, 8); err == nil {
					plain.WriteByte(byte(value))
					at += 2
					continue
				}
			}
			plain.WriteByte('x')
		default:
			plain.WriteByte(written[at])
		}
	}
	return plain.String()
}
