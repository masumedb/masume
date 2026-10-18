//go:build integration

package clickhouse_test

import (
	"context"
	"testing"

	"github.com/masumedb/masume/internal/db/clickhouse"
	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestTextLiteralReadsBackTheSameText(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)

	for _, text := range []string{`C:\new`, `ends with \`, `\'`, `it's`} {
		answered, err := session.RunQuery(context.Background(),
			"select "+clickhouse.Dialect.QuoteTextLiteral(text), dbtest.ReadEverything, nil)
		if err != nil {
			t.Errorf("the literal of %q answered %v", text, err)
			continue
		}
		if held := answered.Rows[0][0]; held != text {
			t.Errorf("the literal of %q reads %q", text, held)
		}
	}
}
