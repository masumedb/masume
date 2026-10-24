package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
)

func TestReadOnlyProfileRefusesEveryStagingKey(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'e', Text: "e"},
		{Code: 'd', Text: "d"},
		{Code: 'D', Text: "D"},
		{Code: 'n', Text: "n"},
	} {
		t.Run(key.Text, func(t *testing.T) {
			model := buildOfflineModel(t, 160, 48)
			connection := model.Active()
			connection.Session.(*offlineSession).profile.AccessMode = cfg.AccessReadOnly
			tab := connection.Active()
			tab.Focus = app.PaneResult
			tab.Results.Start([]string{"select * from tickets"}, 200)
			tab.Results.Succeed(0,
				db.ComposedRead{Text: "select * from tickets", Display: "select * from tickets"},
				db.QueryResult{
					Columns: []db.ResultColumn{
						{Name: "id", DataType: "integer"}, {Name: "note", DataType: "text"},
					},
					Rows: [][]any{{int64(1), "first"}},
				})
			tab.Target = app.EditTarget{
				Table:    db.TableRef{Schema: "public", Name: "tickets"},
				Editable: true, KeyColumns: []string{"id"},
				Columns: []db.ColumnDetail{
					{Name: "id", DataType: "integer", IsPrimaryKey: true},
					{Name: "note", DataType: "text"},
				},
			}
			model.render()

			model.Update(key)
			if connection.Overlay.IsOpen() {
				t.Errorf("the key opened the %q card", connection.Overlay.Kind)
			}
			if staged := len(tab.Pending.Inserts) + len(tab.Pending.DeletedRows) + len(tab.Pending.Edits); staged != 0 {
				t.Errorf("the key staged %d changes", staged)
			}
			if connection.Notice == nil || connection.Notice.Text != readOnlyStagingProblem {
				t.Errorf("the bar shows %+v", connection.Notice)
			}
		})
	}
}
