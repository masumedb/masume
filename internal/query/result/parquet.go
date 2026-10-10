package result

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/masumedb/masume/internal/query"
)

// The physical types of a Parquet column this writer uses.
const (
	parquetBoolean   = 0
	parquetInt64     = 2
	parquetDouble    = 5
	parquetByteArray = 6
)

// The Parquet constants of the file metadata.
const (
	parquetMagic         = "PAR1"
	parquetOptional      = 1
	parquetUTF8          = 0
	parquetPlain         = 0
	parquetRLE           = 3
	parquetDataPage      = 0
	parquetUncompressed  = 0
	parquetFormatVersion = 1
	parquetCreatedBy     = "masume"
)

// parquetColumn is one column chunk of the file metadata.
type parquetColumn struct {
	offset int64
	size   int64
	values int64
}

// parquetRowGroup is one written batch.
type parquetRowGroup struct {
	columns []parquetColumn
	rows    int64
}

// parquetRowWriter writes one row group per batch, and the metadata at the end. The type of
// each column is taken from the first values the writer sees.
type parquetRowWriter struct {
	out     io.Writer
	written int64
	names   []string
	types   []int32
	groups  []parquetRowGroup
	began   bool
}

func newParquetRowWriter(out io.Writer) *parquetRowWriter {
	return &parquetRowWriter{out: out}
}

func (writer *parquetRowWriter) write(data []byte) error {
	count, err := writer.out.Write(data)
	writer.written += int64(count)
	return err
}

func (writer *parquetRowWriter) begin(rows [][]any, columns []query.ResultColumn) error {
	if writer.began {
		return nil
	}
	writer.began = true
	writer.names = BuildRecordKeys(columns)
	writer.types = make([]int32, len(columns))
	for index := range columns {
		writer.types[index] = resolveParquetType(rows, index)
	}
	return writer.write([]byte(parquetMagic))
}

// resolveParquetType returns the type of the first value of the column that is not null:
// a whole number, a fraction, a flag, or text for anything else.
func resolveParquetType(rows [][]any, index int) int32 {
	for _, row := range rows {
		if index >= len(row) || row[index] == nil {
			continue
		}
		switch row[index].(type) {
		case bool:
			return parquetBoolean
		case int, int8, int16, int32, int64, uint8, uint16, uint32:
			return parquetInt64
		case float32, float64:
			return parquetDouble
		}
		return parquetByteArray
	}
	return parquetByteArray
}

func (writer *parquetRowWriter) WriteRows(rows [][]any, columns []query.ResultColumn) error {
	if err := writer.begin(rows, columns); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	group := parquetRowGroup{rows: int64(len(rows))}
	for index, column := range columns {
		page, err := buildParquetPage(rows, index, writer.types[index], column)
		if err != nil {
			return err
		}
		chunk := parquetColumn{offset: writer.written, size: int64(len(page)), values: int64(len(rows))}
		if err := writer.write(page); err != nil {
			return err
		}
		group.columns = append(group.columns, chunk)
	}
	writer.groups = append(writer.groups, group)
	return nil
}

func (writer *parquetRowWriter) Close(columns []query.ResultColumn) error {
	if err := writer.begin(nil, columns); err != nil {
		return err
	}
	metadata := writer.buildFileMetadata()
	if err := writer.write(metadata); err != nil {
		return err
	}
	footer := binary.LittleEndian.AppendUint32(nil, uint32(len(metadata)))
	return writer.write(append(footer, parquetMagic...))
}

// buildParquetPage writes one data page of the column: the page header, the definition
// levels, and the values that are not null.
func buildParquetPage(
	rows [][]any, index int, kind int32, column query.ResultColumn,
) ([]byte, error) {
	defined := make([]bool, len(rows))
	values := []byte{}
	flags := []bool{}
	for at, row := range rows {
		if index >= len(row) || row[index] == nil {
			continue
		}
		defined[at] = true
		value := row[index]
		switch kind {
		case parquetBoolean:
			held, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("column %s holds a flag and then %T", column.Name, value)
			}
			flags = append(flags, held)
		case parquetInt64:
			held, ok := readParquetInteger(value)
			if !ok {
				return nil, fmt.Errorf("column %s holds a whole number and then %T", column.Name, value)
			}
			values = binary.LittleEndian.AppendUint64(values, uint64(held))
		case parquetDouble:
			held, ok := readFloatValue(value)
			if !ok {
				return nil, fmt.Errorf("column %s holds a fraction and then %T", column.Name, value)
			}
			values = binary.LittleEndian.AppendUint64(values, math.Float64bits(held))
		default:
			text := buildExportText(value, column.DataType)
			values = binary.LittleEndian.AppendUint32(values, uint32(len(text)))
			values = append(values, text...)
		}
	}
	if kind == parquetBoolean {
		values = packBits(flags)
	}

	levels := encodeDefinitionLevels(defined)
	body := binary.LittleEndian.AppendUint32(nil, uint32(len(levels)))
	body = append(body, levels...)
	body = append(body, values...)

	header := thriftWriter{}
	header.writeI32(1, parquetDataPage)
	header.writeI32(2, int32(len(body)))
	header.writeI32(3, int32(len(body)))
	header.beginStruct(5)
	header.writeI32(1, int32(len(rows)))
	header.writeI32(2, parquetPlain)
	header.writeI32(3, parquetRLE)
	header.writeI32(4, parquetRLE)
	header.endStruct()
	header.stop()
	return append(header.data, body...), nil
}

// readParquetInteger returns a whole number as int64.
func readParquetInteger(value any) (int64, bool) {
	switch held := value.(type) {
	case int:
		return int64(held), true
	case int8:
		return int64(held), true
	case int16:
		return int64(held), true
	case int32:
		return int64(held), true
	case int64:
		return held, true
	case uint8:
		return int64(held), true
	case uint16:
		return int64(held), true
	case uint32:
		return int64(held), true
	}
	return 0, false
}

// packBits packs flags eight to a byte, the first flag in the lowest bit.
func packBits(flags []bool) []byte {
	packed := make([]byte, (len(flags)+7)/8)
	for at, flag := range flags {
		if flag {
			packed[at/8] |= 1 << (at % 8)
		}
	}
	return packed
}

// encodeDefinitionLevels writes the levels as one bit-packed run of the RLE hybrid encoding:
// 1 for a value, 0 for a null.
func encodeDefinitionLevels(defined []bool) []byte {
	groups := (len(defined) + 7) / 8
	encoded := binary.AppendUvarint(nil, uint64(groups<<1|1))
	padded := make([]bool, groups*8)
	copy(padded, defined)
	return append(encoded, packBits(padded)...)
}

// buildFileMetadata writes the FileMetaData structure of the footer.
func (writer *parquetRowWriter) buildFileMetadata() []byte {
	var rows int64
	for _, group := range writer.groups {
		rows += group.rows
	}

	meta := thriftWriter{}
	meta.writeI32(1, parquetFormatVersion)
	meta.beginList(2, thriftStruct, len(writer.names)+1)
	meta.writeBinary(4, "schema")
	meta.writeI32(5, int32(len(writer.names)))
	meta.stop()
	for index, name := range writer.names {
		meta.writeI32(1, writer.types[index])
		meta.writeI32(3, parquetOptional)
		meta.writeBinary(4, name)
		if writer.types[index] == parquetByteArray {
			meta.writeI32(6, parquetUTF8)
		}
		meta.stop()
	}
	meta.endList()
	meta.writeI64(3, rows)
	meta.beginList(4, thriftStruct, len(writer.groups))
	for _, group := range writer.groups {
		var size int64
		meta.beginList(1, thriftStruct, len(group.columns))
		for index, column := range group.columns {
			size += column.size
			meta.writeI64(2, column.offset)
			meta.beginStruct(3)
			meta.writeI32(1, writer.types[index])
			meta.beginList(2, thriftI32, 2)
			meta.writeListI32(parquetPlain)
			meta.writeListI32(parquetRLE)
			meta.endList()
			meta.beginList(3, thriftBinary, 1)
			meta.writeListBinary(writer.names[index])
			meta.endList()
			meta.writeI32(4, parquetUncompressed)
			meta.writeI64(5, column.values)
			meta.writeI64(6, column.size)
			meta.writeI64(7, column.size)
			meta.writeI64(9, column.offset)
			meta.endStruct()
			meta.stop()
		}
		meta.endList()
		meta.writeI64(2, size)
		meta.writeI64(3, group.rows)
		meta.stop()
	}
	meta.endList()
	meta.writeBinary(6, parquetCreatedBy)
	meta.stop()
	return meta.data
}

// The Thrift compact protocol types this writer uses.
const (
	thriftI32    = 5
	thriftI64    = 6
	thriftBinary = 8
	thriftList   = 9
	thriftStruct = 12
)

// thriftWriter writes the Thrift compact protocol. Each struct keeps the last field id, which
// the next field is written as a delta of.
type thriftWriter struct {
	data []byte
	last []int16
}

func (writer *thriftWriter) field(id int16, kind byte) {
	if len(writer.last) == 0 {
		writer.last = []int16{0}
	}
	top := &writer.last[len(writer.last)-1]
	delta := id - *top
	if delta > 0 && delta <= 15 {
		writer.data = append(writer.data, byte(delta)<<4|kind)
	} else {
		writer.data = append(writer.data, kind)
		writer.data = binary.AppendVarint(writer.data, int64(id))
	}
	*top = id
}

func (writer *thriftWriter) writeI32(id int16, value int32) {
	writer.field(id, thriftI32)
	writer.data = binary.AppendVarint(writer.data, int64(value))
}

func (writer *thriftWriter) writeI64(id int16, value int64) {
	writer.field(id, thriftI64)
	writer.data = binary.AppendVarint(writer.data, value)
}

func (writer *thriftWriter) writeBinary(id int16, text string) {
	writer.field(id, thriftBinary)
	writer.writeListBinary(text)
}

func (writer *thriftWriter) writeListBinary(text string) {
	writer.data = binary.AppendUvarint(writer.data, uint64(len(text)))
	writer.data = append(writer.data, text...)
}

func (writer *thriftWriter) writeListI32(value int32) {
	writer.data = binary.AppendVarint(writer.data, int64(value))
}

// beginStruct opens a struct field. Its fields count their ids from zero again.
func (writer *thriftWriter) beginStruct(id int16) {
	writer.field(id, thriftStruct)
	writer.last = append(writer.last, 0)
}

// endStruct closes a struct field with its stop byte.
func (writer *thriftWriter) endStruct() {
	writer.data = append(writer.data, 0)
	writer.last = writer.last[:len(writer.last)-1]
}

// beginList opens a list field. A list of structs opens a field id scope for each element,
// which stop closes and opens again.
func (writer *thriftWriter) beginList(id int16, kind byte, size int) {
	writer.field(id, thriftList)
	if size < 15 {
		writer.data = append(writer.data, byte(size)<<4|kind)
	} else {
		writer.data = append(writer.data, 0xF0|kind)
		writer.data = binary.AppendUvarint(writer.data, uint64(size))
	}
	writer.last = append(writer.last, 0)
}

func (writer *thriftWriter) endList() {
	writer.last = writer.last[:len(writer.last)-1]
}

// stop ends a struct with its stop byte, and resets the field ids for the next struct.
func (writer *thriftWriter) stop() {
	writer.data = append(writer.data, 0)
	if len(writer.last) > 0 {
		writer.last[len(writer.last)-1] = 0
	}
}
