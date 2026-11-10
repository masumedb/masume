//go:build integration

package mysql_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/db/dbtest"
)

// The row cap of a page read stays off a catalog read inside the user transaction.
func TestACatalogReadInATransactionListsEveryTable(t *testing.T) {
	session := openFlavoured(t)
	ctx := context.Background()
	for at := range 5 {
		dbtest.RunStatements(t, session,
			fmt.Sprintf("drop table if exists masume_cap_%d", at),
			fmt.Sprintf("create table masume_cap_%d (id int primary key)", at))
	}
	t.Cleanup(func() {
		for at := range 5 {
			_, _ = session.RunQuery(context.Background(),
				fmt.Sprintf("drop table if exists masume_cap_%d", at), dbtest.ReadEverything, nil)
		}
	})

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.RollbackTransaction(context.Background()) })
	if _, err := session.RunQuery(ctx,
		"select 1 union all select 2 union all select 3", 1, nil); err != nil {
		t.Fatal(err)
	}

	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed := 0
	for _, table := range tables {
		if strings.HasPrefix(table.Name, "masume_cap_") {
			listed++
		}
	}
	if listed != 5 {
		t.Errorf("the catalog read in the transaction listed %d of the 5 tables", listed)
	}
}
