package clickhouse

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// buildTestClient returns a client of a test server that writes that answer.
func buildTestClient(t *testing.T, status int, header http.Header, body string) *httpClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			for name, values := range header {
				writer.Header()[name] = values
			}
			writer.WriteHeader(status)
			_, _ = io.WriteString(writer, body)
		}))
	t.Cleanup(server.Close)
	return &httpClient{client: server.Client(), endpoint: server.URL + "/", settings: url.Values{}}
}

// An error answer reads as the error of the server, with its code.
func TestSendReadsTheErrorOfTheServer(t *testing.T) {
	client := buildTestClient(t, http.StatusNotFound, nil,
		"Code: 60. DB::Exception: Unknown table expression identifier 'nope'. "+
			"(UNKNOWN_TABLE) (version 25.8.33.6 (official build))\n")
	_, err := client.send(context.Background(), "select * from nope", nil)
	if code, reported := findServerCode(err); !reported || code != 60 {
		t.Fatalf("the error is %v", err)
	}
	if written := describeFailure(err); written != "Unknown table expression identifier 'nope'" {
		t.Errorf("the error reads %q", written)
	}
}

// An error the server writes after some rows ends the read with that error.
func TestReadRowReadsAnErrorAfterTheRows(t *testing.T) {
	client := buildTestClient(t, http.StatusOK, nil,
		"number\nUInt64\n1\n2\n__exception__\r\n"+
			"Code: 395. DB::Exception: Value passed to 'throwIf' function is non-zero. "+
			"(FUNCTION_THROW_IF_VALUE_IS_NON_ZERO) (version 25.8.33.6 (official build))\n")
	response, err := client.send(context.Background(), "select", nil)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := openResult(response)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.close()
	_, err = reader.readRows(-1)
	if code, reported := findServerCode(err); !reported || code != 395 {
		t.Errorf("the read answered %v", err)
	}
}

// A server before 24.11 writes the error after the rows without a marker. The bodies are
// the answers of ClickHouse 24.8.
func TestReadRowReadsAPlainErrorAfterTheRows(t *testing.T) {
	plainError := "Code: 395. DB::Exception: Value passed to 'throwIf' function is non-zero: " +
		"while executing 'FUNCTION throwIf(equals(__table1.number, 5_UInt8) :: 0) -> " +
		"throwIf(equals(__table1.number, 5_UInt8)) UInt8 : 2'. " +
		"(FUNCTION_THROW_IF_VALUE_IS_NON_ZERO) (version 24.8.14.39 (official build))\n"
	for _, body := range []string{
		"number\tfiller\nUInt64\tString\n0\txx\n1\txx\n" + plainError,
		"number\tfiller\nUInt64\tString\n" + plainError,
		"number\nUInt64\n0\n1\n" + plainError,
	} {
		client := buildTestClient(t, http.StatusOK, nil, body)
		response, err := client.send(context.Background(), "select", nil)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := openResult(response)
		if err != nil {
			t.Fatal(err)
		}
		_, err = reader.readRows(-1)
		reader.close()
		if code, reported := findServerCode(err); !reported || code != 395 {
			t.Errorf("%q answered %v", body, err)
		}
	}
}

// A row with the text of an error is a row where more rows follow it.
func TestReadRowReadsErrorTextBeforeMoreRowsAsARow(t *testing.T) {
	client := buildTestClient(t, http.StatusOK, nil,
		"exception\nString\nCode: 60. DB::Exception: Unknown table. (UNKNOWN_TABLE)\nok\n")
	response, err := client.send(context.Background(), "select", nil)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := openResult(response)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.close()
	rows, err := reader.readRows(-1)
	if err != nil || len(rows) != 2 {
		t.Errorf("the read has %d rows: %v", len(rows), err)
	}
}

// The rows of an answer read with their column names and types.
func TestReadRowsReadsTheColumnsAndTheRows(t *testing.T) {
	client := buildTestClient(t, http.StatusOK,
		http.Header{"X-Clickhouse-Timezone": {"Asia/Tokyo"}},
		"id\tat\t\nUInt8\tDateTime\tNullable(String)\n1\t1790132400\t\\N\n")
	response, err := client.send(context.Background(), "select", nil)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := openResult(response)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.close()
	rows, err := reader.readRows(-1)
	if err != nil {
		t.Fatal(err)
	}
	if reader.columns[2].Name != "column3" || !reader.columns[1].Zoned {
		t.Errorf("the columns read as %+v", reader.columns)
	}
	if len(rows) != 1 || rows[0][0] != uint8(1) || rows[0][2] != nil {
		t.Fatalf("the rows read as %#v", rows)
	}
	if written := core.FormatCell(rows[0][1], ""); written != "2026-09-23 03:00:00" {
		t.Errorf("the moment reads as %q", written)
	}
}

// A profile on a port of the native protocol gets an error that names the HTTP port.
func TestConnectRefusesTheNativePort(t *testing.T) {
	for _, port := range []int{9000, 9440} {
		profile := cfg.Profile{
			Engine: core.EngineClickhouse, Host: "127.0.0.1", Port: port, User: "default",
		}
		_, err := NewAdapter(Support).Connect(context.Background(), profile, "")
		if err == nil {
			t.Fatalf("port %d connected", port)
		}
		if written := db.DescribeError(err); !strings.Contains(written, "the HTTP port is 8123") {
			t.Errorf("port %d answered %q", port, written)
		}
	}
}

// A TLS profile speaks HTTPS, and verifies the certificate against the host of the profile
// while it dials the address of a tunnel.
func TestBuildClientVerifiesTheHostThroughATunnel(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(writer, "one\nUInt8\n1\n")
		}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	rootPath := filepath.Join(t.TempDir(), "root.pem")
	written := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(rootPath, written, 0o600); err != nil {
		t.Fatal(err)
	}
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(address.Port())
	if err != nil {
		t.Fatal(err)
	}

	for _, held := range []struct {
		mode core.SSLMode
		host string
		ok   bool
	}{
		{core.SSLRequire, "db.internal", true},
		{core.SSLVerifyFull, "example.com", true},
		{core.SSLVerifyFull, "db.internal", false},
	} {
		profile := cfg.Profile{
			Host: held.host, Port: 8443, SSLMode: held.mode, SSLRootCert: rootPath,
			DialHost: "127.0.0.1", DialPort: port,
		}
		client, err := buildClient(profile, "")
		if err != nil {
			t.Fatal(err)
		}
		rows, err := readAllRows(context.Background(), client, "select 1 as one", nil)
		client.close()
		if held.ok && (err != nil || len(rows) != 1) {
			t.Errorf("%s to %s read %v: %v", held.mode, held.host, rows, err)
		}
		if !held.ok && err == nil {
			t.Errorf("%s to %s took a certificate of another host", held.mode, held.host)
		}
	}
}

// A statement with a FORMAT clause of its own is refused.
func TestOpenResultRefusesAnotherFormat(t *testing.T) {
	client := buildTestClient(t, http.StatusOK,
		http.Header{"X-Clickhouse-Format": {"JSON"}}, "{}")
	response, err := client.send(context.Background(), "select 1 format JSON", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openResult(response); err == nil || !errors.Is(err, db.ErrDatabase) {
		t.Errorf("the answer in another format read as %v", err)
	}
}
