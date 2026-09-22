// Package dump writes a schema as SQL, and runs the statements of a SQL file back into a
// server.
package dump

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/build"
	"github.com/masumedb/masume/internal/query/result"
)

// Content is the part of a schema a dump holds.
type Content string

const (
	ContentAll    Content = "schema and rows"
	ContentSchema Content = "schema only"
	ContentRows   Content = "rows only"
)

// Contents lists the parts in the order the form steps through them.
var Contents = []Content{ContentAll, ContentSchema, ContentRows}

// FindContentNamed parses the text as a content.
func FindContentNamed(written string) (Content, bool) {
	return core.FindAllowed(Contents, strings.ToLower(strings.TrimSpace(written)))
}

// WritesSchema is true for a dump with the definitions in it.
func (content Content) WritesSchema() bool { return content != ContentRows }

// WritesRows is true for a dump with the rows of each table in it.
func (content Content) WritesRows() bool { return content != ContentSchema }

// BatchRows is how many rows one read of a table takes at a time.
const BatchRows = 500

// Options is the dump a caller asks for.
type Options struct {
	Schema string
	// Tables are the tables the dump holds. An empty list is the whole schema: its
	// objects, its tables and its views.
	Tables  []db.TableRef
	Content Content
	// DropsFirst puts a DROP statement in front of each definition.
	DropsFirst bool
	// OnProgress reports how far the dump has come, where a caller wants to follow it.
	OnProgress func(Progress)
}

// Progress is how far a dump has come. It is reported after each batch of rows and after
// each table.
type Progress struct {
	Tables   int
	OfTables int
	Rows     int64
	// Table is the table the dump is reading now.
	Table string
}

// Report is what a dump wrote.
type Report struct {
	Tables  int
	Views   int
	Objects int
	Rows    int64
}

// Describe counts what the dump wrote. A part the dump held none of is left out.
func (report Report) Describe() string {
	parts := []string{present.FormatCountOf(int64(report.Tables), "table", "tables")}
	if report.Views > 0 {
		parts = append(parts, present.FormatCountOf(int64(report.Views), "view", "views"))
	}
	if report.Objects > 0 {
		parts = append(parts,
			present.FormatCountOf(int64(report.Objects), "object", "objects"))
	}
	parts = append(parts, present.FormatCountOf(report.Rows, "row", "rows"))
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// Server is the connection a dump reads.
type Server interface {
	Describe() db.SessionDescriptor
	Dialect() *query.Dialect
	ListTables(ctx context.Context) ([]db.TableRef, error)
	ListSchemaObjects(ctx context.Context) ([]db.SchemaObject, error)
	ListRelationships(ctx context.Context) ([]db.Relationship, error)
	BuildTableDDL(ctx context.Context, table db.TableRef) ([]string, error)
	DescribeTable(ctx context.Context, table db.TableRef) (db.TableDetail, error)
	BuildObjectDDL(ctx context.Context, object db.SchemaObject) ([]string, error)
	StreamQuery(
		ctx context.Context, sql string, params []any, batchSize int,
		onBatch func(rows [][]any, columns []db.ResultColumn) error,
	) (int64, error)
}

// ErrNoTable is the error of a dump that has nothing to write.
var ErrNoTable = db.NewDatabaseError("no table to dump")

// failEmptySchema names the schema the dump found nothing in. A MySQL-protocol connection
// lists the database it opened alone, so another database of that server holds nothing here.
func failEmptySchema(schema string) error {
	return fmt.Errorf("%w in %s", ErrNoTable, schema)
}

// Plan is what a dump writes, in the order it writes it. A definition follows everything it
// uses: the types, the sequences and the functions come first, then the tables, then the
// views over them, then the triggers on them.
type Plan struct {
	Before []db.SchemaObject
	Tables []db.TableRef
	Views  []db.TableRef
	After  []db.SchemaObject
}

// IsEmpty is true for a plan with nothing in it.
func (plan Plan) IsEmpty() bool {
	return len(plan.Before)+len(plan.Tables)+len(plan.Views)+len(plan.After) == 0
}

// beforeTables are the kinds of object a table can use, which are written in front of it.
var beforeTables = []db.SchemaObjectKind{db.ObjectType, db.ObjectSequence, db.ObjectFunction}

// BuildPlan returns what the dump writes. A named list of tables is taken as it is, and
// holds no object and no view.
func BuildPlan(ctx context.Context, server Server, options Options) (Plan, error) {
	relationships, err := server.ListRelationships(ctx)
	if err != nil {
		return Plan{}, err
	}
	if len(options.Tables) > 0 {
		return Plan{Tables: SortByReference(options.Tables, relationships)}, nil
	}

	held, err := server.ListTables(ctx)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{}
	for _, relation := range held {
		if relation.Schema != options.Schema {
			continue
		}
		if relation.Kind == db.RelationTable {
			plan.Tables = append(plan.Tables, relation)
			continue
		}
		plan.Views = append(plan.Views, relation)
	}
	plan.Tables = SortByReference(plan.Tables, relationships)
	if options.Content.WritesSchema() {
		ordered, viewErr := sortViewsByReference(ctx, server, plan.Views)
		if viewErr != nil {
			return Plan{}, viewErr
		}
		plan.Views = ordered
	}

	objects, objectErr := server.ListSchemaObjects(ctx)
	if objectErr != nil {
		return Plan{}, objectErr
	}
	for _, object := range objects {
		if object.Schema != options.Schema {
			continue
		}
		if slices.Contains(beforeTables, object.Kind) {
			plan.Before = append(plan.Before, object)
			continue
		}
		plan.After = append(plan.After, object)
	}
	return plan, nil
}

// sortViewsByReference returns the views with every view in front of the views that read it.
// A view reads another view where its definition holds the name of that view as a word.
func sortViewsByReference(
	ctx context.Context, server Server, views []db.TableRef,
) ([]db.TableRef, error) {
	if len(views) < 2 {
		return views, nil
	}
	references := []db.Relationship{}
	for _, view := range views {
		lines, err := server.BuildTableDDL(ctx, view)
		if err != nil {
			return nil, err
		}
		definition := strings.ToLower(strings.Join(lines, "\n"))
		for _, target := range views {
			if target == view || !holdsWord(definition, strings.ToLower(target.Name)) {
				continue
			}
			references = append(references, db.Relationship{
				Schema: view.Schema, Table: view.Name,
				TargetSchema: target.Schema, TargetTable: target.Name,
			})
		}
	}
	return SortByReference(views, references), nil
}

// holdsWord is true where the text holds that word, with no letter, digit or underscore on
// either side of it.
func holdsWord(text, word string) bool {
	if word == "" {
		return false
	}
	for at := 0; ; {
		found := strings.Index(text[at:], word)
		if found < 0 {
			return false
		}
		start := at + found
		at = start + len(word)
		if !isWordByte(text, start-1) && !isWordByte(text, at) {
			return true
		}
	}
}

// isWordByte is true where the text holds a letter, a digit or an underscore at that index.
func isWordByte(text string, at int) bool {
	if at < 0 || at >= len(text) {
		return false
	}
	held := text[at]
	return held == '_' || (held >= '0' && held <= '9') ||
		(held >= 'a' && held <= 'z') || (held >= 'A' && held <= 'Z')
}

// SortByReference returns the tables with every table in front of the tables that name it in
// a foreign key. Tables that reference each other keep their catalog order.
func SortByReference(tables []db.TableRef, relationships []db.Relationship) []db.TableRef {
	held := map[string]bool{}
	for _, table := range tables {
		held[buildTableKey(table.Schema, table.Name)] = true
	}

	// The tables each table waits for, and the tables waiting for it.
	waitsFor := map[string]map[string]bool{}
	blocks := map[string][]string{}
	for _, relationship := range relationships {
		child := buildTableKey(relationship.Schema, relationship.Table)
		parent := buildTableKey(relationship.TargetSchema, relationship.TargetTable)
		if child == parent || !held[child] || !held[parent] {
			continue
		}
		if waitsFor[child] == nil {
			waitsFor[child] = map[string]bool{}
		}
		if waitsFor[child][parent] {
			continue
		}
		waitsFor[child][parent] = true
		blocks[parent] = append(blocks[parent], child)
	}

	sorted := make([]db.TableRef, 0, len(tables))
	written := map[string]bool{}
	// Every round takes the tables that wait for nothing, in catalog order, so a schema
	// without a foreign key keeps the order of the catalog.
	for len(sorted) < len(tables) {
		moved := false
		for _, table := range tables {
			key := buildTableKey(table.Schema, table.Name)
			if written[key] || len(waitsFor[key]) > 0 {
				continue
			}
			sorted = append(sorted, table)
			written[key] = true
			moved = true
			for _, waiting := range blocks[key] {
				delete(waitsFor[waiting], key)
			}
		}
		if moved {
			continue
		}
		// The tables left reference each other. The first of them is written next, and
		// the rest follow it.
		for _, table := range tables {
			key := buildTableKey(table.Schema, table.Name)
			if written[key] {
				continue
			}
			waitsFor[key] = nil
			break
		}
	}
	return sorted
}

// buildTableKey names one relation for the reference map.
func buildTableKey(schema, name string) string { return schema + "." + name }

// Write writes the schema as SQL: each definition, and one INSERT per row of a table. The
// rows are read a batch at a time, so a large table is never held whole.
func Write(
	ctx context.Context, server Server, options Options, out io.Writer,
) (Report, error) {
	plan, err := BuildPlan(ctx, server, options)
	if err != nil {
		return Report{}, err
	}
	if plan.IsEmpty() {
		return Report{}, failEmptySchema(options.Schema)
	}

	writer := &sqlWriter{out: bufio.NewWriter(out)}
	writeHeader(writer, server.Describe(), options, plan)
	if selected := server.Dialect().BuildSelectSchema(options.Schema); selected != "" {
		writer.writeLine("")
		writer.writeLine(selected)
	}

	report := Report{}
	report.report(options, len(plan.Tables), "")
	if options.DropsFirst && options.Content.WritesSchema() {
		writeDrops(writer, server.Dialect(), plan)
	}
	if options.Content.WritesSchema() {
		count, objectErr := writeObjects(ctx, server, writer, options, plan.Before)
		report.Objects += count
		if objectErr != nil {
			return report, objectErr
		}
	}
	for _, table := range plan.Tables {
		report.report(options, len(plan.Tables), table.Name)
		if tableErr := writeTable(
			ctx, server, writer, options, &report, len(plan.Tables), table); tableErr != nil {
			return report, tableErr
		}
		report.Tables++
		report.report(options, len(plan.Tables), table.Name)
	}
	if !options.Content.WritesSchema() {
		return report, writer.close()
	}

	for _, view := range plan.Views {
		if viewErr := writeView(ctx, server, writer, options, view); viewErr != nil {
			return report, viewErr
		}
		report.Views++
	}
	count, objectErr := writeObjects(ctx, server, writer, options, plan.After)
	report.Objects += count
	if objectErr != nil {
		return report, objectErr
	}
	return report, writer.close()
}

// report hands the caller how far the dump has come.
func (report *Report) report(options Options, tables int, table string) {
	if options.OnProgress == nil {
		return
	}
	options.OnProgress(Progress{
		Tables: report.Tables, OfTables: tables, Rows: report.Rows, Table: table,
	})
}

// writeHeader writes the lines that say what the file holds.
func writeHeader(
	writer *sqlWriter, descriptor db.SessionDescriptor, options Options, plan Plan,
) {
	writer.writeLine("-- masume dump")
	writer.writeLine("-- server: " + string(descriptor.Profile.Engine) + " " +
		descriptor.ServerVersion)
	writer.writeLine("-- database: " + descriptor.Profile.Database)
	writer.writeLine("-- schema: " + options.Schema)
	writer.writeLine("-- content: " + string(options.Content))
	writer.writeLine("-- tables: " + strconv.Itoa(len(plan.Tables)))
	if len(plan.Views) > 0 {
		writer.writeLine("-- views: " + strconv.Itoa(len(plan.Views)))
	}
	if objects := len(plan.Before) + len(plan.After); objects > 0 {
		writer.writeLine("-- objects: " + strconv.Itoa(objects))
	}
	writer.writeLine("-- written: " + time.Now().UTC().Format(time.RFC3339))
}

// writeDrops writes a DROP for everything the dump makes, in the order that removes a thing
// before the things it stands on: the triggers, the views, the tables of a foreign key
// before the tables they name, and last the objects the tables use.
func writeDrops(writer *sqlWriter, dialect *query.Dialect, plan Plan) {
	writer.writeLine("")
	for _, object := range plan.After {
		writer.writeLine(buildDropObject(dialect, object))
	}
	for _, view := range backwards(plan.Views) {
		writer.writeLine(addIfExists(
			build.GenerateDrop(view.Qualified(), string(view.Kind), dialect),
			dialect, view.Schema, view.Name))
	}
	for _, table := range backwards(plan.Tables) {
		writer.writeLine("drop table if exists " +
			dialect.BuildQualifiedName(table.Qualified()) + ";")
	}
	for _, object := range backwards(plan.Before) {
		writer.writeLine(buildDropObject(dialect, object))
	}
}

// buildDropObject writes the DROP of one object of the schema.
func buildDropObject(dialect *query.Dialect, object db.SchemaObject) string {
	return addIfExists(build.GenerateDropObject(build.TemplateObject{
		Schema: object.Schema, Name: object.Name, Kind: string(object.Kind),
		Detail: object.Detail, Identity: object.Identity,
	}, dialect), dialect, object.Schema, object.Name)
}

// backwards returns the list from its last entry to its first.
func backwards[T any](held []T) []T {
	turned := make([]T, 0, len(held))
	for at := len(held) - 1; at >= 0; at-- {
		turned = append(turned, held[at])
	}
	return turned
}

// writeTable writes one table. The rows it writes are counted in the report as they are
// written, so a caller that follows the dump sees a large table move.
func writeTable(
	ctx context.Context, server Server, writer *sqlWriter,
	options Options, report *Report, tables int, table db.TableRef,
) error {
	dialect := server.Dialect()
	target := dialect.BuildQualifiedName(table.Qualified())
	writer.writeLine("")
	writer.writeLine("-- table " + target)

	if options.Content.WritesSchema() {
		lines, err := server.BuildTableDDL(ctx, table)
		if err != nil {
			return err
		}
		writeDefinition(writer, lines)
	}
	if !options.Content.WritesRows() {
		return writer.err
	}

	detail, detailErr := server.DescribeTable(ctx, table)
	if detailErr != nil {
		return detailErr
	}
	// The rows of an identity column are data, and the restore writes them back.
	overrides := db.HoldsIdentityAlways(detail.Columns)
	if overrides && dialect.SwitchIdentityInsert != nil {
		writer.writeLine(dialect.SwitchIdentityInsert(dialect, target, true))
	}
	_, err := server.StreamQuery(ctx, buildRowRead(detail, dialect, target), nil, BatchRows,
		func(batch [][]any, columns []db.ResultColumn) error {
			writer.write(result.BuildInsertScript(
				columns, batch, table.Qualified(), dialect, overrides))
			if writer.err != nil {
				return writer.err
			}
			report.Rows += int64(len(batch))
			report.report(options, tables, table.Name)
			return nil
		})
	if err != nil {
		return err
	}
	if overrides && dialect.SwitchIdentityInsert != nil {
		writer.writeLine(dialect.SwitchIdentityInsert(dialect, target, false))
	}
	return writer.err
}

// buildRowRead returns the read of the rows of a table. A column the server computes from
// other columns is left out, because the server refuses an INSERT that names one. An
// identity column stays: its values are data, and the INSERT overrides the numbering.
func buildRowRead(detail db.TableDetail, dialect *query.Dialect, target string) string {
	names := make([]string, 0, len(detail.Columns))
	skipped := 0
	for _, column := range detail.Columns {
		if column.IsGenerated && !column.IsIdentityAlways {
			skipped++
			continue
		}
		names = append(names, dialect.QuoteIdentifier(column.Name))
	}
	if len(names) == 0 || skipped == 0 {
		return "select * from " + target
	}
	return "select " + strings.Join(names, ", ") + " from " + target
}

// writeView writes the definition of one view. A view holds no row of its own.
func writeView(
	ctx context.Context, server Server, writer *sqlWriter,
	options Options, view db.TableRef,
) error {
	dialect := server.Dialect()
	writer.writeLine("")
	writer.writeLine("-- " + string(view.Kind) + " " +
		dialect.BuildQualifiedName(view.Qualified()))
	lines, err := server.BuildTableDDL(ctx, view)
	if err != nil {
		return err
	}
	writeDefinition(writer, lines)
	return writer.err
}

// writeObjects writes the definition of each object and returns how many it wrote.
func writeObjects(
	ctx context.Context, server Server, writer *sqlWriter,
	options Options, objects []db.SchemaObject,
) (int, error) {
	dialect := server.Dialect()
	written := 0
	for _, object := range objects {
		writer.writeLine("")
		writer.writeLine("-- " + string(object.Kind) + " " +
			dialect.BuildQualifiedName(
				query.QualifiedName{Schema: object.Schema, Name: object.Name}))
		lines, err := server.BuildObjectDDL(ctx, object)
		if err != nil {
			return written, err
		}
		writeDefinition(writer, lines)
		if writer.err != nil {
			return written, writer.err
		}
		written++
	}
	return written, nil
}

// addIfExists puts `if exists` in front of the name a DROP statement names, so the file also
// runs on a server that does not hold the object yet. The statement comes from the dialect of
// the engine, and the name is the first quoted identifier in it. A statement that is a
// comment is left as it is.
func addIfExists(statement string, dialect *query.Dialect, schema, name string) string {
	if strings.HasPrefix(strings.TrimSpace(statement), "--") ||
		strings.Contains(statement, " if exists ") {
		return statement
	}
	for _, target := range []string{
		dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: name}),
		dialect.QuoteIdentifier(name),
	} {
		if at := strings.Index(statement, target); at > 0 {
			return statement[:at] + "if exists " + statement[at:]
		}
	}
	return statement
}

// writeDefinition writes the lines of one definition, with a semicolon after the last of
// them. ClickHouse answers a definition without one, and the statements of a dump are split
// on semicolons.
func writeDefinition(writer *sqlWriter, lines []string) {
	for at := len(lines) - 1; at >= 0; at-- {
		text := strings.TrimRight(lines[at], " \t")
		if text == "" {
			continue
		}
		if !strings.HasSuffix(text, ";") {
			lines[at] = text + ";"
		}
		break
	}
	for _, line := range lines {
		writer.writeLine(line)
	}
}

// sqlWriter writes the text of a dump and keeps the first failure.
type sqlWriter struct {
	out *bufio.Writer
	err error
}

// write puts the text in the file, and does nothing after a failed write.
func (writer *sqlWriter) write(text string) {
	if writer.err != nil {
		return
	}
	_, writer.err = writer.out.WriteString(text)
}

// writeLine puts the text in the file with the line break after it.
func (writer *sqlWriter) writeLine(text string) { writer.write(text + "\n") }

// close writes what is left in the buffer.
func (writer *sqlWriter) close() error {
	if writer.err != nil {
		return writer.err
	}
	return writer.out.Flush()
}
