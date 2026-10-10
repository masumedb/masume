package result_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/result"
)

// sheetCell is one cell of a written sheet, as the XML holds it.
type sheetCell struct {
	Reference string `xml:"r,attr"`
	Type      string `xml:"t,attr"`
	Value     string `xml:"v"`
	Text      string `xml:"is>t"`
}

type sheetRow struct {
	Cells []sheetCell `xml:"c"`
}

type sheet struct {
	Rows []sheetRow `xml:"sheetData>row"`
}

func writeXlsx(t *testing.T, columns []query.ResultColumn, batches ...[][]any) (*zip.Reader, sheet) {
	t.Helper()
	var out bytes.Buffer
	writer := result.CreateRowWriter(result.ExportXLSX, result.DefaultCSVOptions(), &out)
	for _, rows := range batches {
		if err := writer.WriteRows(rows, columns); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(columns); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		text, _ := io.ReadAll(opened)
		var read sheet
		if err := xml.Unmarshal(text, &read); err != nil {
			t.Fatalf("the sheet is no XML: %v\n%s", err, text)
		}
		return archive, read
	}
	t.Fatal("the workbook has no sheet")
	return nil, sheet{}
}

func TestXlsxExportWritesTheHeaderAndTypedCells(t *testing.T) {
	columns := []query.ResultColumn{{Name: "id"}, {Name: "name"}, {Name: "paid"}, {Name: "total"}}
	_, read := writeXlsx(t, columns,
		[][]any{{int64(1), "Ada & <co>", true, 2.5}},
		[][]any{{int64(1234567890123456789), nil, false, nil}})

	if len(read.Rows) != 3 {
		t.Fatalf("the sheet holds %d rows", len(read.Rows))
	}
	header := read.Rows[0].Cells
	if len(header) != 4 || header[0].Text != "id" || header[3].Reference != "D1" {
		t.Errorf("the header is %+v", header)
	}
	first := read.Rows[1].Cells
	if first[0].Value != "1" || first[0].Type != "" || first[1].Text != "Ada & <co>" ||
		first[2].Type != "b" || first[2].Value != "1" || first[3].Value != "2.5" {
		t.Errorf("the first row is %+v", first)
	}
	second := read.Rows[2].Cells
	if len(second) != 2 || second[0].Text != "1234567890123456789" || second[1].Reference != "C3" {
		t.Errorf("the second row is %+v", second)
	}
}

func TestXlsxExportOfNoRowsHoldsTheHeader(t *testing.T) {
	archive, read := writeXlsx(t, []query.ResultColumn{{Name: "id"}})
	names := []string{}
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	if len(read.Rows) != 1 || !strings.Contains(strings.Join(names, " "), "[Content_Types].xml") {
		t.Errorf("the workbook holds %v with %d rows", names, len(read.Rows))
	}
}

func TestXlsxExportRefusesACellLongerThanExcelHolds(t *testing.T) {
	var out bytes.Buffer
	writer := result.CreateRowWriter(result.ExportXLSX, result.DefaultCSVOptions(), &out)
	err := writer.WriteRows([][]any{{strings.Repeat("x", 32768)}}, []query.ResultColumn{{Name: "note"}})
	if err == nil || !strings.Contains(err.Error(), "32767") {
		t.Errorf("the write answered %v", err)
	}
}
