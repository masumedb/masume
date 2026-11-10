package sqlbase

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/masumedb/masume/internal/db"
)

// A reopen that fails reports the transaction the closed connection held.
func TestAFailedReopenReportsTheLostTransaction(t *testing.T) {
	pool, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := pool.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{Connection: connection, Pool: pool}
	session.Transaction.WriteState(db.TransactionOpen)
	session.broken = true
	_ = pool.Close()

	reopenErr := session.ReopenBrokenConnection(context.Background())
	if reopenErr == nil || !strings.Contains(db.DescribeError(reopenErr), "transaction was lost") {
		t.Fatalf("the failed reopen answered %v, wanted the lost transaction", reopenErr)
	}
	if session.Transaction.ReadState() != db.TransactionNone {
		t.Error("the transaction is still marked open after the connection was lost")
	}
}

// A failed reopen with no transaction reports only the connect error.
func TestAFailedReopenWithoutATransactionReportsNoLoss(t *testing.T) {
	pool, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := pool.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{Connection: connection, Pool: pool}
	session.broken = true
	_ = pool.Close()

	reopenErr := session.ReopenBrokenConnection(context.Background())
	if reopenErr == nil || strings.Contains(db.DescribeError(reopenErr), "transaction") {
		t.Errorf("the failed reopen answered %v, wanted a connect error alone", reopenErr)
	}
}
