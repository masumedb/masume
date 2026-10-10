package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/mongo"
	"github.com/masumedb/masume/internal/load"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query"
)

// The most documents, and the most bytes of document text, one insertMany writes.
const (
	documentBatchCount = 1000
	documentBatchBytes = 4 << 20
)

// documentShownCount is the most documents the insertMany of the review shows.
const documentShownCount = 5

// checkDocumentFile reads every document of the file and reports the documents that are not
// valid Extended JSON.
func checkDocumentFile(connectionID int, plan load.Plan) tea.Cmd {
	return func() tea.Msg {
		report := load.CheckReport{}
		shown := []string{}
		err := load.WalkDocuments(plan.Path, func(line int, text string) error {
			report.Rows++
			if _, readErr := mongo.ReadDocument(text); readErr != nil {
				report.Refused++
				if len(report.Problems) < load.MaxRowProblems {
					report.Problems = append(report.Problems,
						load.RowProblem{Line: line, Reason: readErr.Error()})
				}
				return nil
			}
			if len(shown) < documentShownCount {
				shown = append(shown, text)
			}
			return nil
		})
		if err != nil {
			return importCheckedMsg{ConnectionID: connectionID, Problem: err.Error()}
		}
		answered := importCheckedMsg{ConnectionID: connectionID, Report: report}
		if len(shown) > 0 {
			answered.Statements = []string{buildDocumentInsert(plan.Table, shown)}
		}
		return answered
	}
}

// buildDocumentInsert returns the insertMany that writes the documents into the collection.
func buildDocumentInsert(table query.QualifiedName, documents []string) string {
	return mongo.BuildStatementText(table.Schema, table.Name,
		"insertMany(["+strings.Join(documents, ",\n")+"])")
}

// runDocumentImport writes the documents of the file in batches, and leaves out the
// documents the check refused. Each batch is written on its own, with no transaction.
func runDocumentImport(
	connection *app.Connection, connectionID int, plan load.Plan, writes int64,
	updates chan app.Progress,
) tea.Cmd {
	session := connection.Session
	ctx, stop := context.WithCancel(context.Background())
	connection.BeginImport(stop)

	return func() tea.Msg {
		defer stop()
		defer closeProgress(updates)

		written := 0
		batch := []string{}
		batchBytes := 0
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			if _, err := session.RunQuery(
				ctx, buildDocumentInsert(plan.Table, batch), 1, nil); err != nil {
				return err
			}
			written += len(batch)
			batch, batchBytes = batch[:0], 0
			sendProgress(updates, app.Progress{
				Done: int64(written), Total: writes, Label: "documents",
			})
			return nil
		}
		err := load.WalkDocuments(plan.Path, func(_ int, text string) error {
			if _, readErr := mongo.ReadDocument(text); readErr != nil {
				return nil
			}
			batch = append(batch, text)
			batchBytes += len(text)
			if len(batch) < documentBatchCount && batchBytes < documentBatchBytes {
				return nil
			}
			return flush()
		})
		if err == nil {
			err = flush()
		}
		if err != nil {
			problem := db.DescribeError(err)
			if written > 0 {
				problem += "; " + present.FormatCountOf(int64(written), "document", "documents") +
					" were written before the failure"
			}
			return importRanMsg{ConnectionID: connectionID, Problem: problem}
		}
		return importRanMsg{ConnectionID: connectionID, Written: written}
	}
}
