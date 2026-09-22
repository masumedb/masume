package app_test

import (
	"testing"
	"unicode/utf8"

	"github.com/masumedb/masume/internal/app"
)

func TestSelectedLineRangeTakesTheWholeLineUnderTheCaret(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders\nwhere id = 1", 12)

	start, end := buffer.SelectedLineRange()
	if written := buffer.Text[start:end]; written != "from orders" {
		t.Errorf("answered %q, wanted the whole line under the caret", written)
	}
}

func TestSelectedLineRangeStopsAtTheLineAboveASelectionEndingOnALineStart(t *testing.T) {
	buffer := app.NewEditorBuffer("one\ntwo\nthree", 0)
	// From the top down to the first cell of "two", which is the line below "one".
	buffer.SelectRange(0, 4)

	start, end := buffer.SelectedLineRange()
	if written := buffer.Text[start:end]; written != "one" {
		t.Errorf("answered %q, wanted only the line the selection covers", written)
	}
}

func TestCommentLinesWritesTheMarkAtTheShallowestIndent(t *testing.T) {
	// Every line is indented, so the mark belongs at column 4 and the block keeps its shape.
	buffer := app.NewEditorBuffer("    select id\n        from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "    -- select id\n    --     from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestCommentLinesWritesTheMarkAtTheLeftWhereALineHasNoIndent(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\n    from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "-- select id\n--     from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestCommentLinesTakesTheMarkAwayWhereEveryLineCarriesIt(t *testing.T) {
	buffer := app.NewEditorBuffer("-- select id\n-- from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "select id\nfrom orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted the mark taken off both lines", buffer.Text)
	}
}

func TestCommentLinesCommentsABlockThatIsHalfCommented(t *testing.T) {
	buffer := app.NewEditorBuffer("-- select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "-- -- select id\n-- from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted every line commented", buffer.Text)
	}
}

func TestCommentLinesKeepsABlankLineAsItIs(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\n\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "-- select id\n\n-- from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted the blank line left alone", buffer.Text)
	}
}

func TestCommentLinesChangesNothingWithoutAMark(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 0)

	if buffer.CommentLines("") {
		t.Error("reported a change for an engine that has no comment mark")
	}
	if buffer.Text != "select id" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestCommentLinesChangesNothingWhereEveryLineIsBlank(t *testing.T) {
	buffer := app.NewEditorBuffer("\n\n", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if buffer.CommentLines("--") {
		t.Error("reported a change for a block that holds nothing")
	}
}

func TestIndentLinesMovesEveryLineOfTheSelection(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.IndentLines(2) {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "  select id\n  from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestIndentLinesKeepsTheSelectionOverTheSameLines(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))
	buffer.IndentLines(2)

	// A second press has to move the same block again.
	if !buffer.IndentLines(2) {
		t.Fatal("the second press changed nothing")
	}
	wanted := "    select id\n    from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted the same block moved twice", buffer.Text)
	}
}

func TestIndentLinesChangesNothingForAWidthBelowOne(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 0)

	if buffer.IndentLines(0) {
		t.Error("reported a change for a width of nothing")
	}
	if buffer.Text != "select id" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestOutdentLinesTakesAsMuchAsEachLineCanGive(t *testing.T) {
	buffer := app.NewEditorBuffer("    select id\n  from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.OutdentLines(4) {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "select id\nfrom orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted each line moved as far as it could", buffer.Text)
	}
}

func TestOutdentLinesReportsNoChangeWhereNoLineIsIndented(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if buffer.OutdentLines(4) {
		t.Error("reported a change for a block already at the left")
	}
	if buffer.Text != "select id\nfrom orders" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestOutdentLinesTakesATabAsOneStep(t *testing.T) {
	buffer := app.NewEditorBuffer("\tselect id", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.OutdentLines(4) {
		t.Fatal("reported that it changed nothing")
	}
	if buffer.Text != "select id" {
		t.Errorf("answered %q, wanted the tab taken off", buffer.Text)
	}
}

func TestCommentLinesUndoesInOneStep(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))
	buffer.CommentLines("--")

	buffer.Undo()
	if buffer.Text != "select id\nfrom orders" {
		t.Errorf("one undo answered %q, wanted the whole comment taken back", buffer.Text)
	}
}

func TestFindMatchesReadsATermInLowerCaseInEitherCase(t *testing.T) {
	buffer := app.NewEditorBuffer("select ID from Orders where id = 1", 0)

	if found := buffer.FindMatches("id", false); len(found) != 2 {
		t.Errorf("answered %v, wanted both cases of the term", found)
	}
}

func TestFindMatchesReadsATermWithACapitalInThatCaseOnly(t *testing.T) {
	buffer := app.NewEditorBuffer("select ID from Orders where id = 1", 0)

	found := buffer.FindMatches("ID", false)
	if len(found) != 1 || found[0] != len("select ") {
		t.Errorf("answered %v, wanted the one match of that case", found)
	}
}

func TestFindMatchesSkipsAMatchInsideAWordForAWholeWordSearch(t *testing.T) {
	buffer := app.NewEditorBuffer("select id, valid, id_of from t", 0)

	found := buffer.FindMatches("id", true)
	if len(found) != 1 || found[0] != len("select ") {
		t.Errorf("answered %v, wanted the word on its own", found)
	}
}

func TestReplaceMatchesWritesAWholeWordOnly(t *testing.T) {
	buffer := app.NewEditorBuffer("select id, valid from t", 0)

	if written := buffer.ReplaceMatches("id", "key", true); written != 1 {
		t.Errorf("answered %d, wanted the one whole word written", written)
	}
	if buffer.Text != "select key, valid from t" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestMatchesTermReadsTheCaseOfTheTerm(t *testing.T) {
	if !app.MatchesTerm("ID", "id") {
		t.Error("a term in lower case refused a match in capitals")
	}
	if app.MatchesTerm("id", "ID") {
		t.Error("a term with a capital took a match in lower case")
	}
}

func TestReplaceMatchesWritesEveryOneAndCountsThem(t *testing.T) {
	buffer := app.NewEditorBuffer("select id from orders where id = id", 0)

	if written := buffer.ReplaceMatches("id", "key", false); written != 3 {
		t.Errorf("answered %d, wanted every match written", written)
	}
	wanted := "select key from orders where key = key"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestReplaceMatchesAnswersNothingForATermThatIsNotThere(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 0)

	if written := buffer.ReplaceMatches("orders", "rows", false); written != 0 {
		t.Errorf("answered %d, wanted none", written)
	}
	if buffer.Text != "select id" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestReplaceMatchesCarriesTheCaretWithTheText(t *testing.T) {
	// The caret stands on the "o" of "orders", with one match before it.
	buffer := app.NewEditorBuffer("select id from orders where id = 1", 15)

	buffer.ReplaceMatches("id", "identifier", false)
	if buffer.Text[buffer.Caret:buffer.Caret+6] != "orders" {
		t.Errorf("the caret stands at %d, on %q", buffer.Caret, buffer.Text[buffer.Caret:])
	}
}

func TestReplaceMatchesLeavesTheCaretOnACharacterStart(t *testing.T) {
	// The caret stands on the "ä", with one shorter match written before it.
	buffer := app.NewEditorBuffer("select id, 'ä' from orders", len("select id, '"))

	buffer.ReplaceMatches("id", "x", false)
	if !utf8.RuneStart(buffer.Text[buffer.Caret]) {
		t.Errorf("the caret at %d stands inside a character of %q", buffer.Caret, buffer.Text)
	}
	buffer.DeleteForward()
	if !utf8.ValidString(buffer.Text) {
		t.Errorf("a delete after the replace broke the text: %q", buffer.Text)
	}
}

func TestReplaceMatchesUndoesInOneStep(t *testing.T) {
	buffer := app.NewEditorBuffer("id and id", 0)
	buffer.ReplaceMatches("id", "key", false)

	buffer.Undo()
	if buffer.Text != "id and id" {
		t.Errorf("one undo answered %q, wanted the whole replace taken back", buffer.Text)
	}
}

func TestIndentLinesSelectsNothingWithoutASelection(t *testing.T) {
	// The caret stands on the "s" of the second line.
	buffer := app.NewEditorBuffer("select id\nfrom orders", 10)

	if !buffer.IndentLines(2) {
		t.Fatal("reported that it changed nothing")
	}
	if buffer.HasSelection() {
		t.Errorf("the indent selected %q", buffer.Selection())
	}
	// The caret keeps the character it stood on, which the indent moved two cells right.
	if buffer.Caret != 12 {
		t.Errorf("the caret stands at %d, wanted 12", buffer.Caret)
	}
}

func TestCommentLinesSelectsNothingWithoutASelection(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 10)

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	if buffer.HasSelection() {
		t.Errorf("the comment selected %q", buffer.Selection())
	}
	if buffer.Caret != 13 {
		t.Errorf("the caret stands at %d, wanted 13", buffer.Caret)
	}
}

func TestOutdentLinesSelectsNothingWithoutASelection(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\n    from orders", 14)

	if !buffer.OutdentLines(4) {
		t.Fatal("reported that it changed nothing")
	}
	if buffer.HasSelection() {
		t.Errorf("the outdent selected %q", buffer.Selection())
	}
	if buffer.Caret != 10 {
		t.Errorf("the caret stands at %d, wanted 10", buffer.Caret)
	}
}
