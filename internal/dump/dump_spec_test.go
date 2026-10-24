package dump_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/clickhouse"
	"github.com/masumedb/masume/internal/db/mysql"
	"github.com/masumedb/masume/internal/db/postgres"
	"github.com/masumedb/masume/internal/db/sqlite"
	"github.com/masumedb/masume/internal/db/sqlserver"
	"github.com/masumedb/masume/internal/dump"
	"github.com/masumedb/masume/internal/query"
)

// fakeServer answers the reads of a dump from values held in the test.
type fakeServer struct {
	tables  []db.TableRef
	ddl     map[string][]string
	dialect *query.Dialect
	objects []db.SchemaObject
	// objectDDL is the definition of each object, by name.
	objectDDL map[string][]string
	// objectReads is the name of the object of every BuildObjectDDL call.
	objectReads   []string
	relationships []db.Relationship
	sequences     []db.SequenceValue
	columns       []db.ResultColumn
	// detail is the columns of each table, by name.
	detail map[string]db.TableDetail
	rows   [][]any
	// batches is the batch size of every stream call.
	batches []int
	readErr error
	readSQL []string
	// sequenceErr is the error of every ListSequenceValues call.
	sequenceErr error
}

func (server *fakeServer) Describe() db.SessionDescriptor {
	return db.SessionDescriptor{
		Profile:       cfg.Profile{Engine: "postgres", Database: "shop"},
		ServerVersion: "16.2",
	}
}

func (server *fakeServer) Dialect() *query.Dialect {
	if server.dialect != nil {
		return server.dialect
	}
	return postgres.Dialect
}

func (server *fakeServer) ListTables(context.Context) ([]db.TableRef, error) {
	return server.tables, nil
}

func (server *fakeServer) ListSchemaObjects(context.Context) ([]db.SchemaObject, error) {
	return server.objects, nil
}

func (server *fakeServer) ListRelationships(context.Context) ([]db.Relationship, error) {
	return server.relationships, nil
}

// ListSequenceValues returns the held sequences the dump names, and the held sequences of
// the tables the dump names.
func (server *fakeServer) ListSequenceValues(
	_ context.Context, sequences, tables []query.QualifiedName,
) ([]db.SequenceValue, error) {
	if server.sequenceErr != nil {
		return nil, server.sequenceErr
	}
	named := []db.SequenceValue{}
	for _, value := range server.sequences {
		if slices.Contains(sequences, query.QualifiedName{Schema: value.Schema, Name: value.Name}) ||
			(value.Table.Name != "" && slices.Contains(tables, value.Table)) {
			named = append(named, value)
		}
	}
	return named, nil
}

func (server *fakeServer) BuildObjectDDL(
	_ context.Context, object db.SchemaObject,
) ([]string, error) {
	server.objectReads = append(server.objectReads, object.Name)
	return server.objectDDL[object.Name], nil
}

func (server *fakeServer) BuildTableDDL(
	_ context.Context, table db.TableRef,
) ([]string, error) {
	return server.ddl[table.Name], nil
}

func (server *fakeServer) DescribeTable(
	_ context.Context, table db.TableRef,
) (db.TableDetail, error) {
	return server.detail[table.Name], nil
}

func (server *fakeServer) StreamQuery(
	_ context.Context, sql string, _ []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	server.readSQL = append(server.readSQL, sql)
	server.batches = append(server.batches, batchSize)
	if server.readErr != nil {
		return 0, server.readErr
	}
	if err := onBatch(server.rows, server.columns); err != nil {
		return 0, err
	}
	return int64(len(server.rows)), nil
}

func buildServer() *fakeServer {
	return &fakeServer{
		tables: []db.TableRef{
			{Schema: "public", Name: "orders", Kind: db.RelationTable},
			{Schema: "public", Name: "order_view", Kind: db.RelationView},
			{Schema: "audit", Name: "log", Kind: db.RelationTable},
		},
		ddl: map[string][]string{
			"orders":     {"create table public.orders (", "    id integer", ");"},
			"order_view": {"create view public.order_view as select 1;"},
		},
		objects: []db.SchemaObject{
			{Schema: "public", Name: "order_seq", Kind: db.ObjectSequence},
			{Schema: "public", Name: "note_order", Kind: db.ObjectTrigger, Detail: "orders"},
			{Schema: "audit", Name: "other_seq", Kind: db.ObjectSequence},
		},
		objectDDL: map[string][]string{
			"order_seq":  {"create sequence public.order_seq;"},
			"note_order": {"create trigger note_order after update on public.orders;"},
		},
		columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"}, {Name: "name", DataType: "text"},
		},
		rows: [][]any{{int64(1), "first"}, {int64(2), nil}},
	}
}

func TestWriteHoldsSchemaAndRows(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Tables != 1 || report.Rows != 2 || report.Views != 1 || report.Objects != 2 {
		t.Fatalf("report: %+v, want one table, two rows, one view and two objects", report)
	}

	text := written.String()
	for _, wanted := range []string{
		"-- masume dump",
		"-- server: postgres 16.2",
		"-- database: shop",
		"-- schema: public",
		"-- content: schema and rows",
		"-- tables: 1",
		"create table public.orders (",
		`insert into "public"."orders" ("id", "name") values (1, 'first');`,
	} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the dump has no %q in it:\n%s", wanted, text)
		}
	}
	if strings.Contains(text, "audit") || strings.Contains(text, "other_seq") {
		t.Errorf("the dump holds a relation of another schema:\n%s", text)
	}
	// A definition follows everything it uses: the sequence, then the table, then the view
	// over it, then the trigger on it.
	wantedOrder := []string{
		"create sequence", "create table", "insert into", "create view", "create trigger",
	}
	at := 0
	for _, wanted := range wantedOrder {
		found := strings.Index(text[at:], wanted)
		if found < 0 {
			t.Fatalf("the dump holds no %q after the part before it:\n%s", wanted, text)
		}
		at += found
	}
	if len(server.batches) != 1 || server.batches[0] != dump.BatchRows {
		t.Errorf("batch sizes: %v, want one of %d", server.batches, dump.BatchRows)
	}
}

func TestWriteSchemaOnlyReadsNoRow(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentSchema, DropsFirst: true,
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rows != 0 {
		t.Errorf("rows: %d, want 0", report.Rows)
	}
	if len(server.readSQL) != 0 {
		t.Errorf("the dump read %v, want no read", server.readSQL)
	}
	text := written.String()
	if !strings.Contains(text, `drop table if exists "public"."orders";`) {
		t.Errorf("the dump has no drop statement in it:\n%s", text)
	}
	if strings.Contains(text, "insert into") {
		t.Errorf("the dump holds rows:\n%s", text)
	}
}

func TestWriteRowsOnlyHoldsNoDefinition(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentRows, DropsFirst: true,
	}, written); err != nil {
		t.Fatal(err)
	}
	text := written.String()
	if strings.Contains(text, "create table") || strings.Contains(text, "drop table") {
		t.Errorf("the dump holds a definition:\n%s", text)
	}
	if !strings.Contains(text, "insert into") {
		t.Errorf("the dump holds no row:\n%s", text)
	}
}

func TestWriteNamedTables(t *testing.T) {
	server := buildServer()
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentRows,
		Tables: []db.TableRef{{Schema: "audit", Name: "log", Kind: db.RelationTable}},
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Tables != 1 {
		t.Fatalf("tables: %d, want 1", report.Tables)
	}
	if len(server.readSQL) != 1 || server.readSQL[0] != `select * from "audit"."log"` {
		t.Errorf("reads: %v, want the named table alone", server.readSQL)
	}
}

func TestWriteNamedTablesWritesTheTypesAndSequencesTheyUse(t *testing.T) {
	server := buildServer()
	server.ddl["orders"] = []string{
		"create table public.orders (",
		"    id integer default nextval('public.order_seq'::regclass),",
		"    feeling public.mood",
		");",
	}
	server.detail = map[string]db.TableDetail{"orders": {Columns: []db.ColumnDetail{
		{Name: "id", DataType: "integer", DefaultValue: "nextval('public.order_seq'::regclass)"},
		{Name: "feeling", DataType: "public.mood"},
	}}}
	server.objects = append(server.objects,
		db.SchemaObject{Schema: "public", Name: "mood", Kind: db.ObjectType},
		db.SchemaObject{Schema: "public", Name: "unused", Kind: db.ObjectType})
	server.objectDDL["mood"] = []string{"create type public.mood as enum ('happy');"}
	server.objectDDL["unused"] = []string{"create type public.unused as enum ('x');"}
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
		Tables: []db.TableRef{{Schema: "public", Name: "orders", Kind: db.RelationTable}},
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Objects != 2 {
		t.Fatalf("objects: %d, want the sequence and the type", report.Objects)
	}
	text := written.String()
	for _, wanted := range []string{
		"create sequence public.order_seq;", "create type public.mood", "create table",
	} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the dump has no %q in it:\n%s", wanted, text)
		}
	}
	if strings.Index(text, "create type public.mood") > strings.Index(text, "create table") {
		t.Errorf("the type stands after the table that uses it:\n%s", text)
	}
}

// A column named like a type or a sequence uses neither.
func TestWriteNamedTablesSkipsAnObjectNamedLikeAColumn(t *testing.T) {
	server := buildServer()
	server.ddl["orders"] = []string{
		"create table public.orders (", "    status text,", "    feeling integer", ");",
	}
	server.detail = map[string]db.TableDetail{"orders": {Columns: []db.ColumnDetail{
		{Name: "status", DataType: "text"},
		{Name: "feeling", DataType: "integer"},
	}}}
	server.objects = append(server.objects,
		db.SchemaObject{Schema: "public", Name: "status", Kind: db.ObjectType},
		db.SchemaObject{Schema: "public", Name: "feeling", Kind: db.ObjectSequence})
	server.objectDDL["status"] = []string{"create type public.status as enum ('open');"}
	server.objectDDL["feeling"] = []string{"create sequence public.feeling;"}
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
		Tables: []db.TableRef{{Schema: "public", Name: "orders", Kind: db.RelationTable}},
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Objects != 0 {
		t.Errorf("objects: %d, want none:\n%s", report.Objects, written.String())
	}
}

func TestWriteNamedTablesWritesATypeInFrontOfTheTypeThatUsesIt(t *testing.T) {
	server := buildServer()
	server.ddl["orders"] = []string{"create table public.orders (", "    feeling cheerful", ");"}
	server.detail = map[string]db.TableDetail{"orders": {Columns: []db.ColumnDetail{
		{Name: "feeling", DataType: "cheerful"},
	}}}
	server.objects = []db.SchemaObject{
		{Schema: "public", Name: "cheerful", Kind: db.ObjectType},
		{Schema: "public", Name: "mood", Kind: db.ObjectType},
	}
	server.objectDDL["cheerful"] = []string{"create domain public.cheerful as mood;"}
	server.objectDDL["mood"] = []string{"create type public.mood as enum ('happy');"}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentSchema,
		Tables: []db.TableRef{{Schema: "public", Name: "orders", Kind: db.RelationTable}},
	}, written); err != nil {
		t.Fatal(err)
	}
	text := written.String()
	enum, domain := strings.Index(text, "create type public.mood"),
		strings.Index(text, "create domain public.cheerful")
	if enum < 0 || domain < 0 || enum > domain {
		t.Errorf("the enum does not stand in front of the domain over it:\n%s", text)
	}
}

func TestWriteReadsEachTypeDefinitionOnce(t *testing.T) {
	for name, tables := range map[string][]db.TableRef{
		"a schema dump":      nil,
		"a named table dump": {{Schema: "public", Name: "orders", Kind: db.RelationTable}},
	} {
		t.Run(name, func(t *testing.T) {
			server := buildServer()
			server.ddl["orders"] = []string{
				"create table public.orders (", "    feeling cheerful", ");",
			}
			server.detail = map[string]db.TableDetail{"orders": {Columns: []db.ColumnDetail{
				{Name: "feeling", DataType: "cheerful"},
			}}}
			server.objects = []db.SchemaObject{
				{Schema: "public", Name: "cheerful", Kind: db.ObjectType},
				{Schema: "public", Name: "mood", Kind: db.ObjectType},
			}
			server.objectDDL["cheerful"] = []string{"create domain public.cheerful as mood;"}
			server.objectDDL["mood"] = []string{"create type public.mood as enum ('happy');"}
			if _, err := dump.Write(context.Background(), server, dump.Options{
				Schema: "public", Content: dump.ContentSchema, Tables: tables,
			}, &strings.Builder{}); err != nil {
				t.Fatal(err)
			}
			slices.Sort(server.objectReads)
			if reads := strings.Join(server.objectReads, " "); reads != "cheerful mood" {
				t.Errorf("the dump read the definitions of %s", reads)
			}
		})
	}
}

func TestWriteEmptySchemaFails(t *testing.T) {
	server := buildServer()
	_, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "nothing", Content: dump.ContentAll,
	}, &strings.Builder{})
	if !errors.Is(err, dump.ErrNoTable) {
		t.Fatalf("error: %v, want %v", err, dump.ErrNoTable)
	}
}

func TestWriteReportsReadFailure(t *testing.T) {
	server := buildServer()
	server.readErr = errors.New("connection lost")
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "connection lost") {
		t.Fatalf("error: %v, want the read failure", err)
	}
	if report.Tables != 0 {
		t.Errorf("tables: %d, want 0", report.Tables)
	}
}

func TestFindContentNamed(t *testing.T) {
	content, known := dump.FindContentNamed(" Schema Only ")
	if !known || content != dump.ContentSchema {
		t.Fatalf("content: %q %v, want %q", content, known, dump.ContentSchema)
	}
	if _, known := dump.FindContentNamed("everything"); known {
		t.Error("an unknown name was taken")
	}
}

func TestWriteEmptyTableHoldsItsDefinition(t *testing.T) {
	server := buildServer()
	server.rows = nil
	written := &strings.Builder{}
	report, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
	}, written)
	if err != nil {
		t.Fatal(err)
	}
	if report.Tables != 1 || report.Rows != 0 {
		t.Fatalf("report: %+v, want one table and no row", report)
	}
	text := written.String()
	if !strings.Contains(text, "create table public.orders (") {
		t.Errorf("the dump holds no definition:\n%s", text)
	}
	if strings.Contains(text, "insert into") {
		t.Errorf("the dump holds a row:\n%s", text)
	}
}

func TestWriteEndsADefinitionWithASemicolon(t *testing.T) {
	server := buildServer()
	// ClickHouse answers a definition without a terminator.
	server.ddl["orders"] = []string{"create table public.orders (id Int32) engine = Memory"}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentSchema,
	}, written); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(written.String(), "engine = Memory;") {
		t.Errorf("the definition has no terminator:\n%s", written.String())
	}
}

func TestSortByReferenceWritesAParentFirst(t *testing.T) {
	tables := []db.TableRef{
		{Schema: "public", Name: "lines"},
		{Schema: "public", Name: "orders"},
		{Schema: "public", Name: "customers"},
	}
	relationships := []db.Relationship{
		{
			Schema: "public", Table: "lines",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "orders"},
		},
		{
			Schema: "public", Table: "orders",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "customers"},
		},
	}

	sorted := dump.SortByReference(tables, relationships)
	names := make([]string, 0, len(sorted))
	for _, table := range sorted {
		names = append(names, table.Name)
	}
	if strings.Join(names, ",") != "customers,orders,lines" {
		t.Fatalf("order: %v, want the parent of each table before it", names)
	}
}

func TestSortByReferenceKeepsEveryTableOfACycle(t *testing.T) {
	tables := []db.TableRef{
		{Schema: "public", Name: "left"},
		{Schema: "public", Name: "right"},
		{Schema: "public", Name: "alone"},
	}
	relationships := []db.Relationship{
		{
			Schema: "public", Table: "left",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "right"},
		},
		{
			Schema: "public", Table: "right",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "left"},
		},
		// A table that names itself waits for nothing.
		{
			Schema: "public", Table: "alone",
			ForeignKey: query.ForeignKey{TargetSchema: "public", TargetTable: "alone"},
		},
	}

	sorted := dump.SortByReference(tables, relationships)
	if len(sorted) != 3 {
		t.Fatalf("tables: %d, want every table of the cycle", len(sorted))
	}
	seen := map[string]bool{}
	for _, table := range sorted {
		if seen[table.Name] {
			t.Fatalf("%s was written twice", table.Name)
		}
		seen[table.Name] = true
	}
}

// A dump written with --drop runs on a server that does not hold the object yet, so every
// DROP of every engine names `if exists`.
func TestDropStatementsNameIfExists(t *testing.T) {
	dialects := map[string]*query.Dialect{
		"postgres":   postgres.Dialect,
		"mysql":      mysql.Dialect,
		"sqlserver":  sqlserver.Dialect,
		"clickhouse": clickhouse.Dialect,
		"sqlite":     sqlite.Dialect,
	}
	objects := []db.SchemaObject{
		{Schema: "public", Name: "order_seq", Kind: db.ObjectSequence},
		{Schema: "public", Name: "order_kind", Kind: db.ObjectType},
		{Schema: "public", Name: "note_order", Kind: db.ObjectFunction},
		{Schema: "public", Name: "audit_order", Kind: db.ObjectTrigger, Detail: "orders"},
	}
	views := []db.TableRef{
		{Schema: "public", Name: "open_orders", Kind: db.RelationView},
		{Schema: "public", Name: "sales", Kind: db.RelationMaterializedView},
	}

	for name, dialect := range dialects {
		server := buildServer()
		server.dialect = dialect
		server.objects = objects
		server.tables = append(views, db.TableRef{
			Schema: "public", Name: "orders", Kind: db.RelationTable,
		})
		written := &strings.Builder{}
		if _, err := dump.Write(context.Background(), server, dump.Options{
			Schema: "public", Content: dump.ContentSchema, DropsFirst: true,
		}, written); err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		for line := range strings.SplitSeq(written.String(), "\n") {
			if !strings.HasPrefix(line, "drop ") {
				continue
			}
			if !strings.Contains(line, " if exists ") {
				t.Errorf("%s: %q names no `if exists`", name, line)
			}
		}
	}
}

// The server refuses an INSERT that names a column it computes itself. The read of the rows
// leaves that column out.
func TestWriteLeavesOutAGeneratedColumn(t *testing.T) {
	server := buildServer()
	server.detail = map[string]db.TableDetail{
		"orders": {Columns: []db.ColumnDetail{
			{Name: "id", DataType: "integer"},
			{Name: "total", DataType: "numeric", IsGenerated: true},
		}},
	}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentRows,
	}, written); err != nil {
		t.Fatal(err)
	}

	read := strings.Join(server.readSQL, "\n")
	if !strings.Contains(read, `select "id" from "public"."orders"`) {
		t.Errorf("the rows were read with %q", read)
	}
}

// A view that reads another view is created after it. A restore then finds what it reads.
func TestWriteOrdersAViewAfterTheViewItReads(t *testing.T) {
	server := buildServer()
	server.tables = []db.TableRef{
		{Schema: "public", Name: "a_on_z", Kind: db.RelationView},
		{Schema: "public", Name: "z_base", Kind: db.RelationView},
	}
	server.ddl = map[string][]string{
		"a_on_z": {"create view public.a_on_z as select * from z_base;"},
		"z_base": {"create view public.z_base as select 1;"},
	}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentSchema,
	}, written); err != nil {
		t.Fatal(err)
	}

	text := written.String()
	if strings.Index(text, "create view public.z_base") >
		strings.Index(text, "create view public.a_on_z") {
		t.Errorf("the views are written in this order:\n%s", text)
	}
}

// An identity column holds data, so the dump keeps it and the INSERT overrides the numbering
// of the server.
func TestWriteKeepsAnIdentityColumnAndOverridesIt(t *testing.T) {
	server := buildServer()
	server.detail = map[string]db.TableDetail{
		"orders": {Columns: []db.ColumnDetail{
			{Name: "id", DataType: "integer", IsIdentityAlways: true},
			{Name: "name", DataType: "text"},
		}},
	}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentRows,
	}, written); err != nil {
		t.Fatal(err)
	}

	if read := strings.Join(server.readSQL, "\n"); !strings.Contains(read, "select * from") {
		t.Errorf("the rows were read with %q", read)
	}
	if !strings.Contains(written.String(), "overriding system value") {
		t.Errorf("the dump wrote\n%s", written.String())
	}
}

// A SQL Server identity column takes a value of the client only between the two statements
// that open and close it.
func TestWriteOpensAndClosesTheIdentityInsertOfTheServer(t *testing.T) {
	server := buildServer()
	server.dialect = sqlserver.Dialect
	server.detail = map[string]db.TableDetail{
		"orders": {Columns: []db.ColumnDetail{
			{Name: "id", DataType: "int", IsGenerated: true, IsIdentityAlways: true},
			{Name: "name", DataType: "nvarchar"},
		}},
	}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentRows,
	}, written); err != nil {
		t.Fatal(err)
	}

	text := written.String()
	on := strings.Index(text, "set identity_insert [public].[orders] on;")
	off := strings.Index(text, "set identity_insert [public].[orders] off;")
	insert := strings.Index(text, "insert into [public].[orders]")
	if on < 0 || off < 0 || insert < on || insert > off {
		t.Errorf("the dump wrote\n%s", text)
	}
}

func TestWriteSetsTheSequencesAfterTheRows(t *testing.T) {
	server := buildServer()
	server.sequences = []db.SequenceValue{
		{Schema: "public", Name: "order_seq",
			Statement: "SELECT pg_catalog.setval('public.order_seq', 7, true);"},
		{Schema: "public", Name: "orders_id_seq",
			Table:     query.QualifiedName{Schema: "public", Name: "orders"},
			Statement: "SELECT pg_catalog.setval('public.orders_id_seq', 2, true);"},
		{Schema: "audit", Name: "other_seq",
			Statement: "SELECT pg_catalog.setval('audit.other_seq', 3, true);"},
	}
	written := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
	}, written); err != nil {
		t.Fatal(err)
	}
	text := written.String()
	for _, wanted := range []string{"'public.order_seq', 7", "'public.orders_id_seq', 2"} {
		at := strings.Index(text, wanted)
		if at < 0 || at < strings.LastIndex(text, "insert into") {
			t.Errorf("the dump sets no %q after the rows:\n%s", wanted, text)
		}
	}
	if strings.Contains(text, "audit.other_seq") {
		t.Errorf("the dump sets a sequence of another schema:\n%s", text)
	}

	schemaOnly := &strings.Builder{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentSchema,
	}, schemaOnly); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(schemaOnly.String(), "setval") {
		t.Errorf("a dump without rows sets a sequence:\n%s", schemaOnly.String())
	}
}

func TestWriteReadsTheSequencesBeforeTheFirstLine(t *testing.T) {
	server := buildServer()
	server.sequenceErr = errors.New("not a sequence")
	for _, content := range []dump.Content{dump.ContentAll, dump.ContentRows} {
		written := &strings.Builder{}
		_, err := dump.Write(context.Background(), server, dump.Options{
			Schema: "public", Content: content,
		}, written)
		if err == nil || !strings.Contains(err.Error(), "not a sequence") {
			t.Fatalf("%s: error %v, want the sequence read failure", content, err)
		}
		if written.Len() > 0 || len(server.readSQL) > 0 {
			t.Errorf("%s: the dump read %v and wrote\n%s", content, server.readSQL, written)
		}
	}
}
