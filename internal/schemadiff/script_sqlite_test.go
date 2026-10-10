package schemadiff_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

func openSqliteFile(t *testing.T, statements ...string) db.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	profile := cfg.Profile{
		Name: "test", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: 100,
	}
	session, err := engines.CreateAdapters().Open(context.Background(), profile, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	dbtest.RunStatements(t, session, statements...)
	return session
}

func TestScriptMakesTheSqliteSourceEqualTheTarget(t *testing.T) {
	source := openSqliteFile(t,
		`create table customers (id integer primary key, name text)`,
		`create table orders (id integer primary key, total integer, note text, legacy int,
			customer_id integer references customers (id))`,
		"create index orders_total on orders (total)",
		"create table archive (id int)",
		"create view old_orders as select id from orders",
		"insert into customers (id, name) values (1, 'ada')",
		"insert into orders (id, total, note, legacy, customer_id) values (1, 5, 'first', 0, 1)")
	target := openSqliteFile(t,
		`create table customers (id integer primary key, name text not null unique)`,
		`create table orders (id integer primary key, total bigint not null default 0 check (total >= 0),
			note text, placed_at text, customer_id integer references customers (id))`,
		"create index orders_placed on orders (placed_at)",
		"create view old_orders as select id from orders",
		"create view big_orders as select id, total from orders where total > 100")

	if left := migrate(t, source, target, "main", "main"); len(left) != 0 {
		t.Errorf("after the script the schemas still differ: %+v", left)
	}
	shown, err := source.RunQuery(context.Background(),
		"select o.note, c.name from orders o join customers c on c.id = o.customer_id",
		dbtest.ReadEverything, nil)
	if err != nil || len(shown.Rows) != 1 || shown.Rows[0][0] != "first" || shown.Rows[0][1] != "ada" {
		t.Errorf("after the script the rows are %v: %v", shown.Rows, err)
	}
	keys, err := source.RunQuery(context.Background(), "pragma foreign_keys", dbtest.ReadEverything, nil)
	if err != nil || len(keys.Rows) != 1 || fmt.Sprint(keys.Rows[0][0]) != "1" {
		t.Errorf("after the script foreign_keys is %v: %v", keys.Rows, err)
	}
}
