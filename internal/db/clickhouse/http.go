package clickhouse

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/masumedb/masume/internal/db"
)

// resultFormat is the format of every result this client reads.
const resultFormat = "TabSeparatedWithNamesAndTypes"

// exceptionMarker opens an error the server writes after the rows it already sent.
const exceptionMarker = "__exception__"

// plainErrorStart and plainErrorName open an error that a server before 24.11 writes after
// the rows it already sent, without the marker.
const (
	plainErrorStart = "Code: "
	plainErrorName  = "DB::Exception"
)

// errorBodyLimit is the most bytes of an error answer this client reads.
const errorBodyLimit = 1 << 20

// exceptionTagLine matches the line `<length> <tag>` that closes an error of a newer server.
var exceptionTagLine = regexp.MustCompile(`^\d+ \S+$`)

// httpClient sends statements to the HTTP interface of one server.
type httpClient struct {
	client   *http.Client
	endpoint string
	user     string
	password string
	// The settings of every request: the options of the profile and the fixed ones.
	settings url.Values
}

// send posts one statement and returns the answer of the server. An answer other than 200
// is returned as the error of the server.
func (client *httpClient) send(
	ctx context.Context, statement string, params url.Values,
) (*http.Response, error) {
	query := url.Values{}
	for name, values := range client.settings {
		query[name] = values
	}
	for name, values := range params {
		query[name] = values
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		client.endpoint+"?"+query.Encode(), strings.NewReader(statement))
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-ClickHouse-User", client.user)
	request.Header.Set("X-ClickHouse-Key", client.password)
	request.Header.Set("User-Agent", clientName)
	response, err := client.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if response.StatusCode == http.StatusOK {
		return response, nil
	}
	defer func() { _ = response.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, errorBodyLimit))
	if readErr != nil {
		return nil, readErr
	}
	if reported, parsed := parseServerError(string(body)); parsed {
		return nil, reported
	}
	return nil, fmt.Errorf("the server answered %s: %s",
		response.Status, strings.TrimSpace(string(body)))
}

// close drops the idle connections of the client.
func (client *httpClient) close() {
	client.client.CloseIdleConnections()
}

// drainResult reads an answer to its end and returns the error the server wrote in it.
func drainResult(response *http.Response) error {
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if at := bytes.Index(body, []byte(exceptionMarker)); at != -1 {
		return readTrailingError(body[at+len(exceptionMarker):])
	}
	return err
}

// readTrailingError reads the error the server wrote after its marker.
func readTrailingError(rest []byte) error {
	text := strings.TrimSpace(string(rest))
	text = strings.TrimSpace(strings.TrimSuffix(text, exceptionMarker))
	if at := strings.Index(text, "Code: "); at != -1 {
		text = text[at:]
	}
	if lines := strings.Split(text, "\n"); len(lines) > 1 &&
		exceptionTagLine.MatchString(strings.TrimSpace(lines[len(lines)-1])) {
		text = strings.TrimSpace(strings.Join(lines[:len(lines)-1], "\n"))
	}
	if reported, parsed := parseServerError(text); parsed {
		return reported
	}
	return errors.New(text)
}

// resultReader reads the rows of one answer as they arrive.
type resultReader struct {
	body    io.ReadCloser
	lines   *bufio.Reader
	columns []db.ResultColumn
	types   []columnType
	decoder valueDecoder
}

// openResult reads the column names and types an answer opens with. An answer without
// columns returns a reader with none.
func openResult(response *http.Response) (*resultReader, error) {
	reader := &resultReader{
		body: response.Body, lines: bufio.NewReaderSize(response.Body, 64<<10),
		decoder: valueDecoder{zone: response.Header.Get("X-ClickHouse-Timezone")},
	}
	if format := response.Header.Get("X-ClickHouse-Format"); format != "" &&
		format != resultFormat {
		reader.close()
		return nil, db.NewDatabaseError("a statement with a FORMAT clause is not supported")
	}
	names, err := reader.readLine()
	if errors.Is(err, io.EOF) {
		return reader, nil
	}
	if err != nil {
		reader.close()
		return nil, err
	}
	types, err := reader.readLine()
	if err != nil {
		reader.close()
		return nil, err
	}
	nameFields, typeFields := bytes.Split(names, []byte{'\t'}), bytes.Split(types, []byte{'\t'})
	if len(nameFields) != len(typeFields) {
		reader.close()
		return nil, fmt.Errorf("the server sent %d column names and %d types",
			len(nameFields), len(typeFields))
	}
	for at, field := range nameFields {
		name := unescapeText(string(field))
		if name == "" {
			// The server names no column of a computed expression.
			name = fmt.Sprintf("column%d", at+1)
		}
		dataType := unescapeText(string(typeFields[at]))
		reader.columns = append(reader.columns, db.ResultColumn{
			Name: name, DataType: dataType, Zoned: strings.Contains(dataType, "DateTime"),
		})
		reader.types = append(reader.types, parseColumnType(dataType))
	}
	return reader, nil
}

// readLine reads one line without its line break. A line that opens an error of the server
// returns that error.
func (reader *resultReader) readLine() ([]byte, error) {
	line, err := reader.lines.ReadBytes('\n')
	if errors.Is(err, io.EOF) && len(line) == 0 {
		return nil, io.EOF
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	line = bytes.TrimSuffix(line, []byte{'\n'})
	if string(bytes.TrimSuffix(line, []byte{'\r'})) == exceptionMarker {
		// The server ends the answer without the closing chunk after the error.
		rest, _ := io.ReadAll(io.LimitReader(reader.lines, errorBodyLimit))
		return nil, readTrailingError(rest)
	}
	return line, nil
}

// readRow reads the next row as plain values, and returns io.EOF after the last one.
func (reader *resultReader) readRow() ([]any, error) {
	line, err := reader.readLine()
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(line, []byte{'\t'})
	if len(fields) == len(reader.types) && bytes.HasPrefix(line, []byte(plainErrorStart)) &&
		bytes.Contains(line, []byte(plainErrorName)) {
		if _, peekErr := reader.lines.Peek(1); errors.Is(peekErr, io.EOF) {
			return nil, readTrailingError(line)
		}
	}
	if len(fields) != len(reader.types) {
		if at := bytes.Index(line, []byte(exceptionMarker)); at != -1 {
			rest, _ := io.ReadAll(io.LimitReader(reader.lines, errorBodyLimit))
			return nil, readTrailingError(append(line[at+len(exceptionMarker):], rest...))
		}
		if at := bytes.Index(line, []byte(plainErrorStart)); at != -1 &&
			bytes.Contains(line[at:], []byte(plainErrorName)) {
			rest, _ := io.ReadAll(io.LimitReader(reader.lines, errorBodyLimit))
			return nil, readTrailingError(append(line[at:], rest...))
		}
		return nil, fmt.Errorf("the server sent a row of %d fields for %d columns",
			len(fields), len(reader.types))
	}
	values := make([]any, len(fields))
	for at, field := range fields {
		values[at] = reader.decoder.decodeField(field, reader.types[at])
	}
	return values, nil
}

// readRows reads up to that many rows; a negative count reads every row.
func (reader *resultReader) readRows(count int) ([][]any, error) {
	read := [][]any{}
	for count < 0 || len(read) < count {
		values, err := reader.readRow()
		if errors.Is(err, io.EOF) {
			return read, nil
		}
		if err != nil {
			return nil, err
		}
		read = append(read, values)
	}
	return read, nil
}

// close drops the rest of the answer.
func (reader *resultReader) close() {
	_ = reader.body.Close()
}
