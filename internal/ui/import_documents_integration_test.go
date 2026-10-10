//go:build integration

package ui

import (
	"context"
	"fmt"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/load"
	"github.com/masumedb/masume/internal/query"
)

func TestDocumentImportWritesEachDocumentWhole(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "db.masume_people.drop()")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "db.masume_people.drop()", dbtest.ReadEverything, nil)
	})
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Session = session

	path := writeCSV(t, "masume_people.ndjson",
		`{"_id": {"$oid": "65f0c0ffee0000000000c0de"}, "name": "Ada", "born": {"$date": "1815-12-10T00:00:00Z"},`+
			` "tags": ["math"], "address": {"city": "London"}}`+"\n"+
			`{"name": "Bad", "born": {"$date": "nonsense"}}`+"\n"+
			`{"name": "Alan", "visits": 3}`+"\n")
	plan := load.Plan{
		Path: path, Options: load.ReadOptions{Format: load.FileJSON},
		Table: query.QualifiedName{Schema: session.Describe().DefaultSchema, Name: "masume_people"},
	}

	checked, is := checkDocumentFile(1, plan)().(importCheckedMsg)
	if !is || checked.Problem != "" || checked.Report.Rows != 3 || checked.Report.Refused != 1 ||
		checked.Report.Problems[0].Line != 2 || len(checked.Statements) != 1 {
		t.Fatalf("the check answered %+v", checked)
	}

	ran, is := runDocumentImport(connection, 1, plan, 2, make(chan app.Progress, 10))().(importRanMsg)
	if !is || ran.Problem != "" || ran.Written != 2 {
		t.Fatalf("the import answered %+v", ran)
	}
	for statement, wanted := range map[string]int64{
		`db.masume_people.countDocuments({})`:                                                2,
		`db.masume_people.countDocuments({"address.city": "London", tags: "math"})`:          1,
		`db.masume_people.countDocuments({born: {$type: "date"}, _id: {$type: "objectId"}})`: 1,
		`db.masume_people.countDocuments({visits: {$type: "int"}})`:                          1,
	} {
		read, err := session.RunQuery(ctx, statement, dbtest.ReadEverything, nil)
		if err != nil || len(read.Rows) != 1 || fmt.Sprint(read.Rows[0][0]) != fmt.Sprint(wanted) {
			t.Errorf("%s reads %v: %v", statement, read.Rows, db.DescribeError(err))
		}
	}
}
