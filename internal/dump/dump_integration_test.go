//go:build integration

// An integration test: it dumps a real server and runs the dump back into it. PostgreSQL is
// named through MASUME_TEST_POSTGRES, MySQL through MASUME_TEST_MYSQL.
package dump_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
	"github.com/masumedb/masume/internal/dump"
	"github.com/masumedb/masume/internal/schemadiff"
)

const dropDumpSchema = `drop schema if exists masume_dump cascade;`

// The schema holds one of everything a dump writes: a sequence and a function in front of
// the tables, two tables joined by a foreign key, a view over them, and a trigger on one.
const dumpSchema = `
create schema masume_dump;
create sequence masume_dump.order_number;
create function masume_dump.touch_order() returns trigger as $$ begin return new; end; $$
  language plpgsql;
create function masume_dump.describe(order_id integer) returns text as $body$
begin
  if order_id is null then
    return 'none; nothing';
  end if;
  return 'order';
end;
$body$ language plpgsql;
create table masume_dump.customers (
  id   integer primary key,
  name text not null
);
create table masume_dump.orders (
  id          integer not null,
  customer_id integer not null references masume_dump.customers (id),
  status      text not null,
  note        text
);
create view masume_dump.open_orders as
  select id from masume_dump.orders where status = 'open';
create trigger t_touch_order before update on masume_dump.orders
  for each row execute function masume_dump.touch_order();
insert into masume_dump.customers (id, name) values (1, 'ada');
insert into masume_dump.orders (id, customer_id, status, note)
  values (1, 1, 'open', 'first'), (2, 1, 'sent', null), (3, 1, 'open', 'a '';'' note');
`

const dropDumpDatabase = `drop database if exists masume_dump`

// The MySQL protocol takes one statement per call, so the database is built statement by
// statement.
var dumpDatabase = []string{
	"create database masume_dump",
	`create table masume_dump.orders (
  id     int not null,
  status varchar(20) not null,
  note   varchar(50)
)`,
	`insert into masume_dump.orders (id, status, note)
  values (1, 'open', 'first'), (2, 'sent', null), (3, 'open', 'a '';'' note')`,
}

// writeDump writes the schema to a file and returns the path and the report.
func writeDump(
	t *testing.T, session db.Session, options dump.Options,
) (string, dump.Report) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.sql")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	report, writeErr := dump.Write(context.Background(), session, options, file)
	if closeErr := file.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if writeErr != nil {
		t.Fatalf("the dump answered %v", writeErr)
	}
	return path, report
}

// readOrders returns the note of every order, by id.
func readOrders(t *testing.T, session db.Session, target string) map[int64]string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select id, note from "+target+" order by id", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held := map[int64]string{}
	for _, row := range answered.Rows {
		held[db.ReadNonNegativeCount(row[0])] = db.ReadAnyText(row[1])
	}
	return held
}

func TestPostgresDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropDumpSchema, dumpSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropDumpSchema, dbtest.ReadEverything, nil)
	})

	path, report := writeDump(t, session, dump.Options{
		Schema: "masume_dump", Content: dump.ContentAll, DropsFirst: true,
	})
	if report.Tables != 2 || report.Rows != 4 {
		t.Fatalf("report: %+v, want two tables and four rows", report)
	}
	if report.Views != 1 || report.Objects != 4 {
		t.Fatalf("report: %+v, want the view, the sequence, both functions and the trigger",
			report)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(written))
	for _, wanted := range []string{
		"create sequence", "create table", "create view", "create trigger",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("the dump holds no %q:\n%s", wanted, written)
		}
	}
	// The parent of a foreign key stands in front of the table that names it.
	if strings.Index(text, "masume_dump.customers (") >
		strings.Index(text, "table \"masume_dump\".\"orders\"") {
		t.Fatalf("the parent table stands after the table that names it:\n%s", written)
	}

	// The dump drops what it makes, so the same file rebuilds the schema it came from,
	// rows and all. It runs twice, because a restore of a restore must hold as well.
	for round := range 2 {
		run, runErr := dump.RunFile(context.Background(), session, path,
			session.Dialect().Syntax)
		if runErr != nil {
			t.Fatalf("round %d answered %v after %d statements",
				round+1, runErr, run.Statements)
		}
		notes := readOrders(t, session, "masume_dump.orders")
		if len(notes) != 3 || notes[3] != "a ';' note" {
			t.Fatalf("round %d orders: %v, want the three rows of the dump", round+1, notes)
		}
	}
}

func TestMysqlDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	dbtest.RunStatements(t, session, append([]string{dropDumpDatabase}, dumpDatabase...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropDumpDatabase, dbtest.ReadEverything, nil)
	})

	path, report := writeDump(t, session, dump.Options{
		Schema: "masume_dump", Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{
			{Schema: "masume_dump", Name: "orders", Kind: db.RelationTable},
		},
	})
	if report.Tables != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table and three rows", report)
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	notes := readOrders(t, session, "masume_dump.orders")
	if len(notes) != 3 || notes[3] != "a ';' note" {
		t.Fatalf("orders: %v, want the three rows of the dump", notes)
	}
}

// The SQL Server schema lives inside the connected database, so the tables are dropped and
// made again rather than a database of their own.
var dropSqlserverTable = []string{
	"drop table if exists dbo.masume_dump_orders",
}

var sqlserverTable = []string{
	`create table dbo.masume_dump_orders (
  id     int not null,
  status varchar(20) not null,
  note   varchar(50) null
)`,
	`insert into dbo.masume_dump_orders (id, status, note)
  values (1, 'open', 'first'), (2, 'sent', null), (3, 'open', 'a '';'' note')`,
}

func TestSqlserverDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.Sqlserver)
	dbtest.RunStatements(t, session, append(dropSqlserverTable, sqlserverTable...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropSqlserverTable[0], dbtest.ReadEverything, nil)
	})

	table := db.TableRef{Schema: "dbo", Name: "masume_dump_orders", Kind: db.RelationTable}
	path, report := writeDump(t, session, dump.Options{
		Schema: "dbo", Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{table},
	})
	if report.Tables != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table and three rows", report)
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	notes := readOrders(t, session, "dbo.masume_dump_orders")
	if len(notes) != 3 || notes[3] != "a ';' note" {
		t.Fatalf("orders: %v, want the three rows of the dump", notes)
	}
}

// ClickHouse holds no transaction, so the statements of a restore stand on their own.
var dropClickhouseTable = []string{"drop table if exists masume_dump_orders"}

var clickhouseTable = []string{
	`create table masume_dump_orders (
  id     Int32,
  status String,
  note   Nullable(String)
) engine = MergeTree order by id`,
	`insert into masume_dump_orders (id, status, note)
  values (1, 'open', 'first'), (2, 'sent', null), (3, 'open', 'a '';'' note')`,
}

func TestClickhouseDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	dbtest.RunStatements(t, session, append(dropClickhouseTable, clickhouseTable...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropClickhouseTable[0], dbtest.ReadEverything, nil)
	})

	schema := session.Describe().Profile.Database
	table := db.TableRef{Schema: schema, Name: "masume_dump_orders", Kind: db.RelationTable}
	path, report := writeDump(t, session, dump.Options{
		Schema: schema, Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{table},
	})
	if report.Tables != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table and three rows", report)
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	notes := readOrders(t, session, "masume_dump_orders")
	if len(notes) != 3 || notes[3] != "a ';' note" {
		t.Fatalf("orders: %v, want the three rows of the dump", notes)
	}
}

// MongoDB reports no definition and holds another language, so neither command runs on it.
func TestMongoIsRefusedByBothCommands(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)
	if session.Capabilities().WritesDDL {
		t.Fatal("this MongoDB reports definitions")
	}

	_, err := dump.Write(context.Background(), session, dump.Options{
		Schema: "shop", Content: dump.ContentAll,
	}, &strings.Builder{})
	if err == nil {
		t.Error("a dump of MongoDB wrote a file")
	}
}

// The values of a table must read back exactly as they were: a dump that changes a moment, a
// blob or a list is worse than one that fails.

const dropTypeSchema = `drop schema if exists masume_values cascade;`

const typeSchema = `
create schema masume_values;
create table masume_values."Odd Name" (
  id           integer primary key,
  amount       numeric(12,4),
  ratio        double precision,
  flag         boolean,
  note         text,
  raw          bytea,
  made_at      timestamp,
  made_at_zone timestamptz,
  day          date,
  clock        time,
  payload      jsonb,
  tags         text[],
  missing      text
);
insert into masume_values."Odd Name" values
  (1, 1234.5678, 0.1, true, 'a ; note', '\x00ff10'::bytea,
   '2026-09-10 14:30:00', '2026-09-10 14:30:00+02', '2026-09-10', '14:30:00',
   '{"a": 1, "c": "x; y"}'::jsonb, array['one','two;three'], null),
  (2, -0.0001, 1e300, false, e'line\nbreak\ttab', '\x'::bytea,
   '0001-01-01 00:00:00', '2026-01-01 00:00:00+00', '0001-01-01', '00:00:00',
   'null'::jsonb, array[]::text[], null);
`

// A MySQL-protocol connection lists the database it opened alone, so the table stands in the
// database of the test connection.
const dropTypeDatabase = "drop table if exists `Odd Name`"

var typeDatabase = []string{
	"create table `Odd Name` (\n" +
		"  id int primary key, amount decimal(12,4), ratio double, flag tinyint(1),\n" +
		"  note text, raw blob, made_at datetime, day date, clock time, payload json,\n" +
		"  kind enum('one','two'), bits bit(8), missing varchar(10))",
	"insert into `Odd Name` values (1, 1234.5678, 0.1, 1, 'a ; note', " +
		"x'00ff10', '2026-09-10 14:30:00', '2026-09-10', '14:30:00', " +
		`'{"a": 1, "c": "x; y"}', 'two', b'10101010', null)`,
}

// readEveryCell returns every cell of the relation as the client formats it.
func readEveryCell(t *testing.T, session db.Session, target string) []string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select * from "+target+" order by id", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held := []string{}
	for _, row := range answered.Rows {
		for at, value := range row {
			dataType := ""
			if at < len(answered.Columns) {
				dataType = answered.Columns[at].DataType
			}
			held = append(held, core.FormatCell(value, dataType))
		}
	}
	return held
}

// checkValuesSurviveTheRoundTrip dumps the relation, runs the dump back and compares every
// cell with what stood there before.
func checkValuesSurviveTheRoundTrip(
	t *testing.T, session db.Session, options dump.Options, target string,
) {
	t.Helper()
	before := readEveryCell(t, session, target)
	if len(before) == 0 {
		t.Fatal("the relation holds no cell to compare")
	}

	path, _ := writeDump(t, session, options)
	run, err := dump.RunFile(context.Background(), session, path, session.Dialect().Syntax)
	if err != nil {
		t.Fatalf("the restore answered %v after %d statements", err, run.Statements)
	}

	after := readEveryCell(t, session, target)
	if len(after) != len(before) {
		t.Fatalf("%d cells came back, want %d", len(after), len(before))
	}
	for at, held := range before {
		if after[at] != held {
			t.Errorf("cell %d: %q, want %q", at, after[at], held)
		}
	}
}

func TestPostgresValuesSurviveTheRoundTrip(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropTypeSchema, typeSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropTypeSchema, dbtest.ReadEverything, nil)
	})

	checkValuesSurviveTheRoundTrip(t, session, dump.Options{
		Schema: "masume_values", Content: dump.ContentAll, DropsFirst: true,
	}, `masume_values."Odd Name"`)
}

func TestMysqlValuesSurviveTheRoundTrip(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	dbtest.RunStatements(t, session, append([]string{dropTypeDatabase}, typeDatabase...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropTypeDatabase, dbtest.ReadEverything, nil)
	})

	schema := session.Describe().Profile.Database
	checkValuesSurviveTheRoundTrip(t, session, dump.Options{
		Schema: schema, Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{
			{Schema: schema, Name: "Odd Name", Kind: db.RelationTable},
		},
	}, "`Odd Name`")
}

// A dump of a table larger than one batch reports its progress as it goes, so a card can
// draw how far it has come.
func TestPostgresDumpReportsItsProgress(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropDumpSchema, `
create schema masume_dump;
create table masume_dump.many (id integer primary key);
insert into masume_dump.many select generate_series(1, 1500);
`)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropDumpSchema, dbtest.ReadEverything, nil)
	})

	held := []dump.Progress{}
	if _, err := dump.Write(context.Background(), session, dump.Options{
		Schema: "masume_dump", Content: dump.ContentAll,
		OnProgress: func(progress dump.Progress) { held = append(held, progress) },
	}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}

	// 1500 rows are read in batches of 500, so the rows move more than once.
	if len(held) < 4 {
		t.Fatalf("reports: %d, want one per batch and one per table", len(held))
	}
	last := held[len(held)-1]
	if last.Rows != 1500 || last.Tables != 1 || last.OfTables != 1 {
		t.Fatalf("the last report is %+v, want every row of the one table", last)
	}
	moved := false
	for _, report := range held {
		if report.Rows > 0 && report.Rows < 1500 {
			moved = true
		}
	}
	if !moved {
		t.Errorf("the rows never moved: %+v", held)
	}
}

const dropIdentitySchema = `drop schema if exists masume_identity cascade;`

// The schema holds the two kinds of column a client may not write: a column the server
// numbers itself, and a column it computes from another one.
const identitySchema = `
create schema masume_identity;
create table masume_identity.tickets (
  id    integer generated always as identity primary key,
  price numeric not null,
  tax   numeric generated always as (price * 0.2) stored
);
insert into masume_identity.tickets (price) values (100), (250);
`

// A dump of a table with an identity column carries the numbers back into the server, and
// leaves out the column the server computes from another one.
func TestPostgresDumpKeepsTheNumbersOfAnIdentityColumn(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropIdentitySchema, identitySchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropIdentitySchema, dbtest.ReadEverything, nil)
	})

	path, report := writeDump(t, session, dump.Options{
		Schema: "masume_identity", Content: dump.ContentAll, DropsFirst: true,
	})
	if report.Tables != 1 || report.Rows != 2 {
		t.Fatalf("report: %+v, want one table and two rows", report)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(written))
	if !strings.Contains(text, "overriding system value") {
		t.Fatalf("the INSERT overrides nothing:\n%s", written)
	}
	if strings.Contains(text, `"tax"`) && strings.Contains(text, "insert into") {
		if strings.Contains(text[strings.Index(text, "insert into"):], `"tax"`) {
			t.Fatalf("the INSERT names the computed column:\n%s", written)
		}
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	answered, readErr := session.RunQuery(context.Background(),
		"select id, tax from masume_identity.tickets order by id",
		dbtest.ReadEverything, nil)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(answered.Rows) != 2 ||
		db.ReadNonNegativeCount(answered.Rows[0][0]) != 1 ||
		db.ReadNonNegativeCount(answered.Rows[1][0]) != 2 {
		t.Fatalf("the restored rows are %v", answered.Rows)
	}
}

var dropIdentityTable = []string{
	"drop table if exists dbo.masume_identity_tickets",
}

var sqlserverIdentityTable = []string{
	`create table dbo.masume_identity_tickets (
  id    int identity(1,1) primary key,
  price decimal(10,2) not null
)`,
	`insert into dbo.masume_identity_tickets (price) values (100), (250)`,
}

// A dump of a SQL Server table with an identity column carries the numbers back into the
// server, between the two statements that open and close the column.
func TestSqlserverDumpKeepsTheNumbersOfAnIdentityColumn(t *testing.T) {
	session := dbtest.Open(t, dbtest.Sqlserver)
	dbtest.RunStatements(t, session, append(dropIdentityTable, sqlserverIdentityTable...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropIdentityTable[0], dbtest.ReadEverything, nil)
	})

	table := db.TableRef{
		Schema: "dbo", Name: "masume_identity_tickets", Kind: db.RelationTable,
	}
	path, report := writeDump(t, session, dump.Options{
		Schema: "dbo", Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{table},
	})
	if report.Tables != 1 || report.Rows != 2 {
		t.Fatalf("report: %+v, want one table and two rows", report)
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		written, _ := os.ReadFile(path)
		t.Fatalf("the restore answered %v after %d statements:\n%s",
			runErr, run.Statements, written)
	}
	answered, readErr := session.RunQuery(context.Background(),
		"select id from dbo.masume_identity_tickets order by id", dbtest.ReadEverything, nil)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(answered.Rows) != 2 ||
		db.ReadNonNegativeCount(answered.Rows[0][0]) != 1 ||
		db.ReadNonNegativeCount(answered.Rows[1][0]) != 2 {
		t.Fatalf("the restored rows are %v", answered.Rows)
	}
}

const dropMomentSchema = `drop schema if exists masume_moment cascade;`

const momentSchema = `
create schema masume_moment;
create table masume_moment.events (
  id       integer primary key,
  happened timestamp not null
);
insert into masume_moment.events (id, happened)
  values (1, '2024-03-04 05:06:07.123456'), (2, '2024-03-04 05:06:07');
`

// A server holds a moment to the microsecond, so the dump writes every digit of it and the
// restore puts the same moment back.
func TestPostgresDumpKeepsTheMicrosecondsOfAMoment(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropMomentSchema, momentSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropMomentSchema, dbtest.ReadEverything, nil)
	})

	path, _ := writeDump(t, session, dump.Options{
		Schema: "masume_moment", Content: dump.ContentAll, DropsFirst: true,
	})
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "05:06:07.123456") {
		t.Fatalf("the dump holds no microseconds:\n%s", written)
	}

	if _, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax); runErr != nil {
		t.Fatalf("the restore answered %v", runErr)
	}
	answered, readErr := session.RunQuery(context.Background(),
		"select count(*) from masume_moment.events where happened = '2024-03-04 05:06:07.123456'",
		dbtest.ReadEverything, nil)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if db.ReadNonNegativeCount(answered.Rows[0][0]) != 1 {
		t.Errorf("the restored moment is not the one the row held: %v", answered.Rows)
	}
}

const dropRestoreSchema = `drop schema if exists masume_restore cascade;`

const restoreSchema = `
create schema masume_restore;
create type masume_restore.mood as enum ('happy', 'sad');
create domain masume_restore.cheerful as masume_restore.mood
  default 'happy' not null check (value <> 'sad');
create domain masume_restore.code as text collate "C" check (value ~ '^[a-z]+$');
create type masume_restore.address as (street text, city text collate "C", kind masume_restore.code);
create type masume_restore.price_range as range (subtype = numeric);
create sequence masume_restore.ticket_number;
create table masume_restore.customers (
  id      serial primary key,
  email   text not null unique,
  feeling masume_restore.mood,
  tags    text[],
  mood    masume_restore.cheerful,
  address masume_restore.address,
  prices  masume_restore.price_range
);
create table masume_restore.orders (
  id          integer generated always as identity primary key,
  customer_id integer references masume_restore.customers (id),
  note        text,
  constraint orders_note_key unique (note)
);
create table masume_restore.events (
  id    integer generated by default as identity primary key,
  label text
);
insert into masume_restore.customers (email, feeling, tags, address, prices)
  values ('ada@example.com', 'happy', '{a,b}', row('main', 'oslo', 'home'), '[1,5)'),
         ('bob@example.com', 'sad', '{}', null, null);
insert into masume_restore.orders (customer_id, note) values (1, 'first'), (2, 'second');
insert into masume_restore.events (label) values ('one'), ('two'), ('three');
select nextval('masume_restore.ticket_number');
`

const restoreDatabase = "masume_restore_copy"

// openEmptyPostgresDatabase creates a database that holds nothing and opens a session on it.
// The database is dropped when the test ends.
func openEmptyPostgresDatabase(t *testing.T, setup db.Session) db.Session {
	t.Helper()
	dbtest.RunStatements(t, setup,
		"drop database if exists "+restoreDatabase+" with (force)",
		"create database "+restoreDatabase)
	t.Cleanup(func() {
		_, _ = setup.RunQuery(context.Background(),
			"drop database if exists "+restoreDatabase+" with (force)",
			dbtest.ReadEverything, nil)
	})

	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.Database = restoreDatabase
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// restoreIntoEmptyDatabase runs the dump into a database that holds nothing but the schema.
func restoreIntoEmptyDatabase(t *testing.T, setup db.Session, path string) db.Session {
	t.Helper()
	copied := openEmptyPostgresDatabase(t, setup)
	dbtest.RunStatements(t, copied, "create schema masume_restore")
	run, err := dump.RunFile(context.Background(), copied, path, copied.Dialect().Syntax)
	if err != nil {
		t.Fatalf("the restore answered %v after %d statements", err, run.Statements)
	}
	return copied
}

// readNumber returns the one number the read answers.
func readNumber(t *testing.T, session db.Session, sql string) int64 {
	t.Helper()
	answered, err := session.RunQuery(context.Background(), sql, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("%s answered %v", sql, err)
	}
	return db.ReadNonNegativeCount(answered.Rows[0][0])
}

// A schema dump restored into an empty database gives the same schema, and the next row of
// each table takes the number after the last restored row.
func TestPostgresDumpRestoresIntoAnEmptyDatabase(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropRestoreSchema, restoreSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropRestoreSchema, dbtest.ReadEverything, nil)
	})

	path, _ := writeDump(t, session, dump.Options{
		Schema: "masume_restore", Content: dump.ContentAll,
	})
	copied := restoreIntoEmptyDatabase(t, session, path)

	dbtest.RunStatements(t, copied,
		"insert into masume_restore.customers (email) values ('cy@example.com')",
		"insert into masume_restore.orders (customer_id, note) values (3, 'third')",
		"insert into masume_restore.events (label) values ('four')")
	for sql, wanted := range map[string]int64{
		"select max(id) from masume_restore.customers":   3,
		"select max(id) from masume_restore.orders":      3,
		"select max(id) from masume_restore.events":      4,
		"select nextval('masume_restore.ticket_number')": 2,
	} {
		if held := readNumber(t, copied, sql); held != wanted {
			t.Errorf("%s: %d, want %d", sql, held, wanted)
		}
	}

	ctx := context.Background()
	source, sourceErr := schemadiff.ReadSnapshot(ctx, session, "masume_restore")
	if sourceErr != nil {
		t.Fatal(sourceErr)
	}
	target, targetErr := schemadiff.ReadSnapshot(ctx, copied, "masume_restore")
	if targetErr != nil {
		t.Fatal(targetErr)
	}
	if found := schemadiff.Compare(source, target); len(found) != 0 {
		t.Errorf("the restored schema differs:\n%s", schemadiff.WriteReport(found))
	}
}

// A dump of named tables holds the types and the sequences the tables use, so it restores
// into an empty database and the next rows take the next numbers. It drops the tables and not
// the types, so it runs again where another table uses one of the types.
func TestPostgresTableDumpRestoresIntoAnEmptyDatabase(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropRestoreSchema, restoreSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropRestoreSchema, dbtest.ReadEverything, nil)
	})

	path, _ := writeDump(t, session, dump.Options{
		Schema: "masume_restore", Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{
			{Schema: "masume_restore", Name: "orders", Kind: db.RelationTable},
			{Schema: "masume_restore", Name: "customers", Kind: db.RelationTable},
		},
	})
	copied := restoreIntoEmptyDatabase(t, session, path)
	dbtest.RunStatements(t, copied, "create table masume_restore.other (feeling masume_restore.mood)")
	run, err := dump.RunFile(context.Background(), copied, path, copied.Dialect().Syntax)
	if err != nil {
		t.Fatalf("the second restore answered %v after %d statements", err, run.Statements)
	}

	dbtest.RunStatements(t, copied,
		"insert into masume_restore.customers (email, feeling) values ('cy@example.com', 'sad')",
		"insert into masume_restore.orders (customer_id, note) values (3, 'third')")
	for sql, wanted := range map[string]int64{
		"select max(id) from masume_restore.customers": 3,
		"select max(id) from masume_restore.orders":    3,
	} {
		if held := readNumber(t, copied, sql); held != wanted {
			t.Errorf("%s: %d, want %d", sql, held, wanted)
		}
	}
}

const dropSequenceSchemas = `
drop schema if exists masume_sequence cascade;
drop schema if exists masume_hidden cascade;
drop role if exists masume_dump_reader;
`

const sequenceSchemas = `
create role masume_dump_reader login password 'secret';
create schema masume_sequence;
create table masume_sequence.tickets (id serial primary key, note text);
insert into masume_sequence.tickets (note) values ('first'), ('second');
create table masume_sequence.notes (id serial primary key, note text);
insert into masume_sequence.notes (note) values ('first');
create schema masume_hidden;
create sequence masume_hidden.secret_number;
select nextval('masume_hidden.secret_number');
grant usage on schema masume_sequence, masume_hidden to masume_dump_reader;
grant select on all tables in schema masume_sequence to masume_dump_reader;
grant select on masume_sequence.tickets_id_seq to masume_dump_reader;
`

// writeTicketDump writes the rows of the tickets and the notes tables and returns the dump.
func writeTicketDump(t *testing.T, session db.Session) string {
	t.Helper()
	path, _ := writeDump(t, session, dump.Options{
		Schema: "masume_sequence", Content: dump.ContentRows,
		Tables: []db.TableRef{
			{Schema: "masume_sequence", Name: "tickets", Kind: db.RelationTable},
			{Schema: "masume_sequence", Name: "notes", Kind: db.RelationTable},
		},
	})
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "masume_sequence.tickets_id_seq', 2, true") {
		t.Errorf("the dump sets no ticket number:\n%s", written)
	}
	return string(written)
}

func TestPostgresDumpSkipsTheTemporarySequenceOfAnotherSession(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropSequenceSchemas, sequenceSchemas)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropSequenceSchemas, dbtest.ReadEverything, nil)
	})
	other := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, other, "create temporary table masume_counter (id serial)")
	if held := readNumber(t, session,
		"select count(*) from pg_class where relname = 'masume_counter_id_seq'"); held == 0 {
		t.Fatal("the other session holds no temporary sequence")
	}

	writeTicketDump(t, session)
}

func TestPostgresDumpSkipsASequenceTheUserCannotRead(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropSequenceSchemas, sequenceSchemas)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropSequenceSchemas, dbtest.ReadEverything, nil)
	})
	profile, _ := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.User = "masume_dump_reader"
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	reader, err := engines.CreateAdapters().Open(ctx, profile, "secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	written := writeTicketDump(t, reader)
	for _, unread := range []string{"secret_number", "notes_id_seq"} {
		if strings.Contains(written, unread) {
			t.Errorf("the dump sets %s:\n%s", unread, written)
		}
	}
}

var dropSqlserverSequenceSchema = []string{
	"drop table if exists masume_sequence.tickets",
	"drop sequence if exists masume_sequence.ticket_number",
	"drop sequence if exists masume_sequence.spent_number",
	"drop sequence if exists masume_sequence.round_number",
	"drop schema if exists masume_sequence",
}

var sqlserverSequenceSchema = []string{
	"create schema masume_sequence",
	`create sequence masume_sequence.ticket_number as bigint
  start with 100 increment by 5 minvalue 1 maxvalue 1000000 no cycle cache 20`,
	`create sequence masume_sequence.spent_number as tinyint
  start with 1 increment by 1 minvalue 1 maxvalue 2 no cycle no cache`,
	`create sequence masume_sequence.round_number as decimal(9, 0)
  start with 2 increment by 1 minvalue 1 maxvalue 3 cycle no cache`,
	`create table masume_sequence.tickets (
  id   bigint not null default (next value for masume_sequence.ticket_number) primary key,
  note varchar(20) not null
)`,
	"insert into masume_sequence.tickets (note) values ('first'), ('second'), ('third')",
	"select next value for masume_sequence.spent_number",
	"select next value for masume_sequence.spent_number",
	"select next value for masume_sequence.round_number",
	"select next value for masume_sequence.round_number",
}

const sqlserverRestoreDatabase = "masume_sequence_copy"

// openEmptySqlserverDatabase creates a database with the masume_sequence schema alone and
// opens a session on it. The database is dropped when the test ends.
func openEmptySqlserverDatabase(t *testing.T, setup db.Session) db.Session {
	t.Helper()
	dropDatabase := "if db_id('" + sqlserverRestoreDatabase + "') is not null begin " +
		"alter database " + sqlserverRestoreDatabase +
		" set single_user with rollback immediate; " +
		"drop database " + sqlserverRestoreDatabase + "; end"
	dbtest.RunStatements(t, setup, dropDatabase, "create database "+sqlserverRestoreDatabase)
	t.Cleanup(func() {
		_, _ = setup.RunQuery(context.Background(), dropDatabase, dbtest.ReadEverything, nil)
	})

	profile, password := dbtest.BuildProfile(t, dbtest.Sqlserver)
	profile.Database = sqlserverRestoreDatabase
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	dbtest.RunStatements(t, session, "create schema masume_sequence")
	return session
}

// runSqlserverRestore runs the dump into the session, and fails the test with the dump text.
func runSqlserverRestore(t *testing.T, session db.Session, path string) {
	t.Helper()
	run, err := dump.RunFile(context.Background(), session, path, session.Dialect().Syntax)
	if err != nil {
		written, _ := os.ReadFile(path)
		t.Fatalf("the restore answered %v after %d statements:\n%s",
			err, run.Statements, written)
	}
}

// checkSqlserverSequencesContinue inserts a ticket and reads each sequence once more.
func checkSqlserverSequencesContinue(t *testing.T, session db.Session, holdsAll bool) {
	t.Helper()
	dbtest.RunStatements(t, session,
		"insert into masume_sequence.tickets (note) values ('fourth')")
	wanted := map[string]int64{
		"select max(id) from masume_sequence.tickets":         115,
		"select count(*) from masume_sequence.tickets":        4,
		"select next value for masume_sequence.ticket_number": 120,
	}
	if holdsAll {
		wanted["select next value for masume_sequence.round_number"] = 1
	}
	for sql, number := range wanted {
		if held := readNumber(t, session, sql); held != number {
			t.Errorf("%s: %d, want %d", sql, held, number)
		}
	}
	if !holdsAll {
		return
	}
	if _, err := session.RunQuery(context.Background(),
		"select next value for masume_sequence.spent_number",
		dbtest.ReadEverything, nil); err == nil {
		t.Error("the spent sequence returned another value")
	}
}

// A SQL Server dump creates each sequence in front of the tables that use it, and the next
// value of each sequence follows the last value of the source.
func TestSqlserverDumpRestoresTheSequencesIntoAnEmptyDatabase(t *testing.T) {
	session := dbtest.Open(t, dbtest.Sqlserver)
	dbtest.RunStatements(t, session,
		append(slices.Clone(dropSqlserverSequenceSchema), sqlserverSequenceSchema...)...)
	t.Cleanup(func() {
		for _, sql := range dropSqlserverSequenceSchema {
			_, _ = session.RunQuery(context.Background(), sql, dbtest.ReadEverything, nil)
		}
	})

	for _, dropsFirst := range []bool{false, true} {
		path, report := writeDump(t, session, dump.Options{
			Schema: "masume_sequence", Content: dump.ContentAll, DropsFirst: dropsFirst,
		})
		if report.Tables != 1 || report.Objects != 3 || report.Rows != 3 {
			t.Fatalf("report: %+v, want one table, three objects and three rows", report)
		}
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{"AS decimal(9, 0)", "CACHE 20", "NO CACHE"} {
			if !strings.Contains(string(written), line) {
				t.Errorf("the dump holds no %q:\n%s", line, written)
			}
		}
		copied := openEmptySqlserverDatabase(t, session)
		runSqlserverRestore(t, copied, path)
		checkSqlserverSequencesContinue(t, copied, true)
	}
}

// A dump of a named SQL Server table holds the sequence the table uses, and it runs again
// into a database that holds that sequence.
func TestSqlserverTableDumpRestoresTheSequenceItUses(t *testing.T) {
	session := dbtest.Open(t, dbtest.Sqlserver)
	dbtest.RunStatements(t, session,
		append(slices.Clone(dropSqlserverSequenceSchema), sqlserverSequenceSchema...)...)
	t.Cleanup(func() {
		for _, sql := range dropSqlserverSequenceSchema {
			_, _ = session.RunQuery(context.Background(), sql, dbtest.ReadEverything, nil)
		}
	})

	path, report := writeDump(t, session, dump.Options{
		Schema: "masume_sequence", Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{
			{Schema: "masume_sequence", Name: "tickets", Kind: db.RelationTable},
		},
	})
	if report.Tables != 1 || report.Objects != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table, one object and three rows", report)
	}
	copied := openEmptySqlserverDatabase(t, session)
	runSqlserverRestore(t, copied, path)
	runSqlserverRestore(t, copied, path)
	checkSqlserverSequencesContinue(t, copied, false)
}
