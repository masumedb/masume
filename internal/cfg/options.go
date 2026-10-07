package cfg

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/masumedb/masume/internal/core"
)

// ParseOptions reads driver options written as a URL query: `key=value&key=value`.
func ParseOptions(text string) (map[string]string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, nil
	}
	query, err := url.ParseQuery(trimmed)
	if err != nil {
		return nil, failProfile("options are not key=value pairs joined by &: %v", err)
	}
	options := map[string]string{}
	for key, values := range query {
		if strings.TrimSpace(key) == "" {
			return nil, failProfile("an option has no name")
		}
		options[key] = values[0]
	}
	return options, nil
}

// WriteOptions returns the options as a URL query, sorted by key.
func WriteOptions(options map[string]string) string {
	query := url.Values{}
	for key, value := range options {
		query.Set(key, value)
	}
	return query.Encode()
}

// consumedURLKeys are the URL query keys the profile holds in fields of its own.
func consumedURLKeys(engine core.Engine) []string {
	keys := slices.Concat(sslKeys, sslRootCertKeys, sslCertKeys, sslKeyKeys)
	if isMongoEngine(engine) {
		keys = append(keys, directKey, "authSource", "replicaSet")
	}
	if engine == core.EngineTurso {
		keys = append(keys, "authToken")
	}
	return keys
}

// readURLOptions returns the URL query keys the profile has no field for. A key written
// twice keeps its first value.
func readURLOptions(parsed *url.URL, engine core.Engine) map[string]string {
	consumed := consumedURLKeys(engine)
	options := map[string]string{}
	for key, values := range parsed.Query() {
		if !slices.Contains(consumed, key) {
			options[key] = values[0]
		}
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

// readOptionsTable reads the `options` table of a profile. A number or a flag is kept as its
// text.
func readOptionsTable(source Table) (map[string]string, error) {
	value, present := source["options"]
	if !present {
		return nil, nil
	}
	table, isTable := FindTable(value)
	if !isTable {
		return nil, failProfile(`"options" must be a table, such as options = { connect_timeout = "5" }`)
	}
	options := map[string]string{}
	for key, held := range table {
		switch written := held.(type) {
		case string:
			options[key] = written
		case int64:
			options[key] = strconv.FormatInt(written, 10)
		case bool:
			options[key] = strconv.FormatBool(written)
		default:
			return nil, failProfile("option %q must be a string, a number or a flag", key)
		}
	}
	return options, nil
}

// writeTomlKey returns the key bare where TOML allows it, and quoted otherwise.
func writeTomlKey(key string) string {
	if key != "" && !strings.ContainsFunc(key, func(character rune) bool {
		return !(character < unicode.MaxASCII && (unicode.IsLetter(character) ||
			unicode.IsDigit(character) || character == '_' || character == '-'))
	}) {
		return key
	}
	return quoteTomlString(key)
}
