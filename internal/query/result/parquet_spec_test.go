package result_test

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/result"
)

func TestParquetExportFramesTheFile(t *testing.T) {
	var out bytes.Buffer
	columns := []query.ResultColumn{{Name: "id"}, {Name: "name"}}
	writer := result.CreateRowWriter(result.ExportParquet, result.DefaultCSVOptions(), &out)
	if err := writer.WriteRows([][]any{{int64(1), "ada"}, {int64(2), nil}}, columns); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(columns); err != nil {
		t.Fatal(err)
	}
	written := out.Bytes()
	if !bytes.HasPrefix(written, []byte("PAR1")) || !bytes.HasSuffix(written, []byte("PAR1")) {
		t.Fatalf("the file is not framed by PAR1: %q", written)
	}
	footer := int(binary.LittleEndian.Uint32(written[len(written)-8:]))
	if footer <= 0 || footer > len(written)-12 {
		t.Fatalf("the footer length %d does not fit a file of %d bytes", footer, len(written))
	}
	metadata := written[len(written)-8-footer : len(written)-8]
	for _, held := range []string{"schema", "id", "name", "masume"} {
		if !bytes.Contains(metadata, []byte(held)) {
			t.Errorf("the metadata has no %q", held)
		}
	}
}

func TestParquetExportRefusesAColumnThatChangesType(t *testing.T) {
	var out bytes.Buffer
	columns := []query.ResultColumn{{Name: "id"}}
	writer := result.CreateRowWriter(result.ExportParquet, result.DefaultCSVOptions(), &out)
	if err := writer.WriteRows([][]any{{int64(1)}}, columns); err != nil {
		t.Fatal(err)
	}
	err := writer.WriteRows([][]any{{"two"}}, columns)
	if err == nil || !strings.Contains(err.Error(), "column id holds a whole number and then string") {
		t.Errorf("the write answered %v", err)
	}
}
