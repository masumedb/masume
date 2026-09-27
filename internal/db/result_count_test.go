package db

import "testing"

func TestCountReportedRowsReadsTheAffectedRowsOfAWrite(t *testing.T) {
	written := QueryResult{Command: "UPDATE", Affected: 999, HasAffected: true}
	if counted := written.CountReportedRows(); counted != 999 {
		t.Errorf("an update of 999 rows counts %d", counted)
	}
}

func TestCountReportedRowsReadsTheReturnedRowsOfAResultSet(t *testing.T) {
	returned := QueryResult{
		Columns: []ResultColumn{{Name: "id"}}, Rows: [][]any{{1}, {2}},
		Affected: 5, HasAffected: true,
	}
	if counted := returned.CountReportedRows(); counted != 2 {
		t.Errorf("a result of 2 rows counts %d", counted)
	}
}

func TestCountReportedRowsCountsNothingForDDL(t *testing.T) {
	if counted := (QueryResult{Command: "CREATE TABLE"}).CountReportedRows(); counted != 0 {
		t.Errorf("a create table counts %d", counted)
	}
}
