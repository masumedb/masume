package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/query/syntax"
)

func TestTheYesOfAWriteQuestionNamesTheWrite(t *testing.T) {
	for _, held := range []struct {
		statements []string
		wanted     string
	}{
		{[]string{"delete from orders where id = 1"}, "delete"},
		{[]string{"drop table orders"}, "drop"},
		{[]string{"update a set b = 1", "delete from c"}, "run 2 statements"},
	} {
		if label := describeWriteAnswer(held.statements, syntax.FlavourStandard); label != held.wanted {
			t.Errorf("%q is answered by %q, wanted %q", held.statements, label, held.wanted)
		}
	}
}

func TestOnlyADestructiveQuestionIsDrawnAsAnError(t *testing.T) {
	model := buildOfflineModel(t, 140, 40)
	question := app.Overlay{
		Kind: app.OverlayConfirm, Title: " close connection ", Body: "Close it?",
		Yes: "close connection",
	}
	plain := model.renderConfirm(question, 60)
	question.Destructive = true
	destructive := model.renderConfirm(question, 60)
	if plain == destructive {
		t.Error("the plain question is drawn like the destructive one")
	}
	if stripEscapes(plain) != stripEscapes(destructive) {
		t.Error("the two questions differ in more than their colours")
	}
}
