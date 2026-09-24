package statement_test

import (
	"testing"

	"github.com/masumedb/masume/internal/db/mysql"
	"github.com/masumedb/masume/internal/db/postgres"
	"github.com/masumedb/masume/internal/db/sqlserver"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/statement"
)

func TestInlineBoundParametersWritesEachValueInOrder(t *testing.T) {
	for _, held := range []struct {
		sql     string
		params  []any
		dialect *query.Dialect
		want    string
	}{
		{`update "t" set "a" = $1, "b" = '$2' where "id" = $2`, []any{7, int64(1)},
			postgres.Dialect, `update "t" set "a" = 7, "b" = '$2' where "id" = 1`},
		{"update `t` set `a` = ? where `id` = ?", []any{"x", 2},
			mysql.Dialect, "update `t` set `a` = 'x' where `id` = 2"},
		{`delete from [t] where [id] = @p1 or [id] = @p2`, []any{1, 2},
			sqlserver.Dialect, `delete from [t] where [id] = 1 or [id] = 2`},
	} {
		written, inlined := statement.InlineBoundParameters(held.sql, held.params, held.dialect)
		if !inlined || written != held.want {
			t.Errorf("%q reads as %q (%v), wanted %q", held.sql, written, inlined, held.want)
		}
	}
	if _, inlined := statement.InlineBoundParameters(
		`delete from "t" where "id" = $1`, []any{1, 2}, postgres.Dialect); inlined {
		t.Error("a statement with fewer placeholders than values was inlined")
	}
}
