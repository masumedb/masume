package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query/syntax"
	"github.com/masumedb/masume/internal/writeplan"
)

func TestTheUndoQuestionNamesTheWriteAndItsRows(t *testing.T) {
	held := app.HeldUndo{
		Undo:  writeplan.Undo{Table: db.TableRef{Schema: "public", Name: "orders"}, Rows: 999},
		SQL:   "update orders set total = total where id < 1000",
		RanAt: time.Now().Add(-9 * time.Second),
	}
	written := describeUndoQuestion(held, syntax.FlavourStandard)
	if !strings.HasPrefix(written, "Undo the update of 999 rows in orders from 9s ago?") {
		t.Errorf("the question reads %q", written)
	}
}
