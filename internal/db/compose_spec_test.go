package db_test

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/postgres"
)

// A grid filter belongs to the rows of a read. A statement whose SELECT feeds a write takes
// neither the filter nor the sort, and is never paged: both would cut the rows it writes.
func TestComposeStatementReadLeavesAWriteAsItIs(t *testing.T) {
	composer := db.NewSQLComposer(postgres.Dialect)
	rewrite := core.ReadRewrite{
		Filter: []core.FilterStep{core.BuildCellFilter("status", "new", false)},
	}

	for _, held := range []struct {
		name     string
		sql      string
		filtered bool
		pageable bool
	}{
		{"a select", "select * from orders", true, true},
		{"an insert from a select", "insert into copy select * from orders", false, false},
		{"a select into a table", "select * into copy from orders", false, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			read := composer.ComposeStatementRead(db.BoundText{Text: held.sql}, rewrite)
			if strings.Contains(read.Text, "status") != held.filtered {
				t.Errorf("the read is %q", read.Text)
			}
			if read.Pageable != held.pageable {
				t.Errorf("the read is pageable: %v", read.Pageable)
			}
		})
	}
}
