package result

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/masumedb/masume/internal/query"
)

// The limits of one sheet, in rows and in characters per cell.
const (
	xlsxMaxRows      = 1_048_576
	xlsxMaxCellChars = 32_767
)

// xlsxExactDigits is the integer size above which Excel rounds, so a larger integer is
// written as text.
const xlsxExactDigits = 1e15

// The fixed parts of a workbook of one sheet.
var xlsxParts = []struct{ name, text string }{
	{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
		`</Types>`},
	{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`},
	{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<sheets><sheet name="result" sheetId="1" r:id="rId1"/></sheets></workbook>`},
	{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`</Relationships>`},
	{"xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
		`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
		`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
		`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>` +
		`</styleSheet>`},
}

// xlsxRowWriter writes a workbook of one sheet. The header row is bold.
type xlsxRowWriter struct {
	archive *zip.Writer
	sheet   *bufio.Writer
	rows    int
	began   bool
}

func newXlsxRowWriter(out io.Writer) *xlsxRowWriter {
	return &xlsxRowWriter{archive: zip.NewWriter(out)}
}

func (writer *xlsxRowWriter) begin(columns []query.ResultColumn) error {
	if writer.began {
		return nil
	}
	writer.began = true
	for _, part := range xlsxParts {
		entry, err := writer.archive.Create(part.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(entry, part.text); err != nil {
			return err
		}
	}
	entry, err := writer.archive.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return err
	}
	writer.sheet = bufio.NewWriter(entry)
	if _, err := writer.sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`); err != nil {
		return err
	}
	header := make([]any, 0, len(columns))
	for _, column := range columns {
		header = append(header, column.Name)
	}
	return writer.writeRow(header, nil, true)
}

func (writer *xlsxRowWriter) WriteRows(rows [][]any, columns []query.ResultColumn) error {
	if err := writer.begin(columns); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.writeRow(row, columns, false); err != nil {
			return err
		}
	}
	return nil
}

func (writer *xlsxRowWriter) Close(columns []query.ResultColumn) error {
	if err := writer.begin(columns); err != nil {
		return err
	}
	if _, err := writer.sheet.WriteString(`</sheetData></worksheet>`); err != nil {
		return err
	}
	if err := writer.sheet.Flush(); err != nil {
		return err
	}
	return writer.archive.Close()
}

// writeRow writes one row. A null cell is left empty.
func (writer *xlsxRowWriter) writeRow(row []any, columns []query.ResultColumn, header bool) error {
	if writer.rows >= xlsxMaxRows {
		return fmt.Errorf("an xlsx sheet holds at most %d rows, header included", xlsxMaxRows)
	}
	writer.rows++
	number := strconv.Itoa(writer.rows)
	var line strings.Builder
	line.WriteString(`<row r="` + number + `">`)
	for index, value := range row {
		if value == nil {
			continue
		}
		reference := buildColumnLetters(index) + number
		column := query.ResultColumn{}
		if index < len(columns) {
			column = columns[index]
		}
		cell, err := buildXlsxCell(reference, value, column, header)
		if err != nil {
			return err
		}
		line.WriteString(cell)
	}
	line.WriteString(`</row>`)
	_, err := writer.sheet.WriteString(line.String())
	return err
}

// buildColumnLetters returns the letters of a column from zero: A, B, …, Z, AA, AB, ….
func buildColumnLetters(index int) string {
	letters := ""
	for index >= 0 {
		letters = string(rune('A'+index%26)) + letters
		index = index/26 - 1
	}
	return letters
}

// buildXlsxCell writes a cell: a number or a flag as its type, and anything else as text.
func buildXlsxCell(
	reference string, value any, column query.ResultColumn, header bool,
) (string, error) {
	switch held := value.(type) {
	case bool:
		flag := "0"
		if held {
			flag = "1"
		}
		return `<c r="` + reference + `" t="b"><v>` + flag + `</v></c>`, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		written := fmt.Sprint(held)
		if parsed, err := strconv.ParseFloat(written, 64); err == nil && math.Abs(parsed) < xlsxExactDigits {
			return `<c r="` + reference + `"><v>` + written + `</v></c>`, nil
		}
	case float32, float64:
		parsed, _ := readFloatValue(held)
		if !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
			return `<c r="` + reference + `"><v>` + strconv.FormatFloat(parsed, 'g', -1, 64) + `</v></c>`, nil
		}
	}
	text := buildExportText(value, column)
	if len([]rune(text)) > xlsxMaxCellChars {
		return "", fmt.Errorf("cell %s holds more than %d characters, the most an xlsx cell holds",
			reference, xlsxMaxCellChars)
	}
	style := ""
	if header {
		style = ` s="1"`
	}
	return `<c r="` + reference + `"` + style + ` t="inlineStr"><is><t xml:space="preserve">` +
		escapeXMLText(text) + `</t></is></c>`, nil
}

// escapeXMLText escapes the text and drops the control characters XML cannot hold.
func escapeXMLText(text string) string {
	cleaned := strings.Map(func(character rune) rune {
		if character == '\t' || character == '\n' || character == '\r' || character >= 0x20 {
			return character
		}
		return -1
	}, text)
	var written strings.Builder
	_ = xml.EscapeText(&written, []byte(cleaned))
	return written.String()
}
