package datasearch_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/datasearch"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/engines"
)

func openShop(t *testing.T) db.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := engines.CreateAdapters().Open(context.Background(), cfg.Profile{
		Name: "shop", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: cfg.DefaultPageSize,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	for _, statement := range []string{
		"create table customers (id integer primary key, name text, email text)",
		"insert into customers (name, email) values ('Ada', 'ada@example.com'), ('Alan', 'alan@example.com')",
		"create table orders (id integer primary key, note text, total integer)",
		"insert into orders (note, total) values ('for ada', 5), ('plain', 7)",
		"create table archive (id integer primary key, note text)",
	} {
		if _, err := session.RunQuery(context.Background(), statement, 10, nil); err != nil {
			t.Fatal(err)
		}
	}
	return session
}

func TestSearchFindsTheTablesAndColumnsThatHoldTheText(t *testing.T) {
	session := openShop(t)
	report, err := datasearch.Search(context.Background(), session, "main", "ADA", 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.Searched != 3 || len(report.Failed) != 0 || len(report.Matches) != 2 {
		t.Fatalf("report: %+v", report)
	}
	customers, orders := report.Matches[0], report.Matches[1]
	if customers.Table.Name != "customers" || customers.Rows != 1 ||
		!slices.Equal(customers.Columns, []string{"name", "email"}) {
		t.Errorf("customers: %+v", customers)
	}
	if orders.Table.Name != "orders" || orders.Rows != 1 || !slices.Equal(orders.Columns, []string{"note"}) {
		t.Errorf("orders: %+v", orders)
	}

	read, err := session.RunQuery(context.Background(),
		"select count(*) from orders where "+orders.Filter, 10, nil)
	if err != nil || len(read.Rows) != 1 || read.Rows[0][0] != int64(1) {
		t.Errorf("the filter of the match reads %v: %v", read.Rows, err)
	}
}

func TestSearchCapsTheRowsOfATable(t *testing.T) {
	session := openShop(t)
	report, err := datasearch.Search(context.Background(), session, "main", "example", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Matches) != 1 || report.Matches[0].Rows != 1 || !report.Matches[0].Capped {
		t.Errorf("report: %+v", report)
	}
}
