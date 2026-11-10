package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
)

// A statement that cannot be paged reads past the page size on its first read.
func TestAnUnpagedStatementReadsPastThePageSize(t *testing.T) {
	model, _ := openTabSessionModel(t, "", true)
	connection := model.Active()
	tab := connection.Active()

	answered := runTabStatement(t, connection, tab,
		"with recursive n(i) as (select 1 union all select i + 1 from n where i < 150) "+
			"select i from n")
	if answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if !answered.Read.Pageable {
		t.Fatal("the recursive read was not pageable; the test needs a pageable read first")
	}
	if len(answered.Result.Rows) > 101 {
		t.Errorf("the pageable read holds %d rows, wanted one page", len(answered.Result.Rows))
	}

	answered = runTabStatement(t, connection, tab, "pragma function_list")
	if answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if answered.Read.Pageable {
		t.Fatal("pragma function_list was pageable; the test needs a statement that is not")
	}
	if len(answered.Result.Rows) <= 100 || answered.Result.Truncated {
		t.Errorf("pragma function_list holds %d rows, truncated %v; wanted every row",
			len(answered.Result.Rows), answered.Result.Truncated)
	}
}

// Ctrl+F on a cut result of a statement that cannot be paged shows that it cannot page.
func TestFetchMoreOnAnUnpagedResultShowsItCannotPage(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Results.Start([]string{"exec report"}, 100)
	active := tab.Results.Active()
	active.State.Kind = app.QuerySucceeded
	active.State.Result.Truncated = true
	active.State.Result.Rows = make([][]any, 10000)

	model.fetchMore(connection, tab)

	if text := connection.Notice.Text; text != "this statement cannot be paged; the first 10,000 rows are loaded" {
		t.Errorf("the notice reads %q", text)
	}
}
