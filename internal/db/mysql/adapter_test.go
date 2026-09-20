package mysql

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Cancellation requires a known connection ID.
func TestCancelRunningQueryRefusesAnUnknownThread(t *testing.T) {
	session := &mysqlSession{SessionFacts: db.SessionFacts{
		Support: db.EngineSupport{EngineInfo: core.EngineInfo{
			Capabilities: core.Capabilities{CancelsRunningQuery: true},
		}},
	}}

	stopped, err := session.CancelRunningQuery(context.Background())
	if stopped {
		t.Error("a cancel with no thread id reported that it stopped one")
	}
	if err == nil {
		t.Fatal("a cancel with no thread id answered no error")
	}
	if described := db.DescribeError(err); described !=
		"cannot cancel the statement: the connection ID is unavailable" {
		t.Errorf("the cancel describes as %q", described)
	}
}

// The session row cap holds the rows a read returns. A statement whose SELECT feeds a write
// runs without it, and keeps every row it writes.
func TestResolveSelectLimitCapsAReadAndNotAWrite(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want int
	}{
		{"a select", "select * from orders", 101},
		{"a common table expression", "with c as (select 1) select * from c", 101},
		{"an insert from a select", "insert into copy select * from orders", -1},
		{"a select into a table", "select * into copy from orders", -1},
		{"a create from a select", "create table copy as select * from orders", -1},
		{"a call", "call fill_copy()", -1},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := resolveSelectLimit(held.sql, syntax.FlavourMysql, 100)
			if got != held.want {
				t.Errorf("the cap of %q is %d, want %d", held.sql, got, held.want)
			}
		})
	}
}
