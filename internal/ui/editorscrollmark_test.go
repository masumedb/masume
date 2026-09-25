package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

func TestALineMovedLeftMarksItsHiddenStart(t *testing.T) {
	model := buildOfflineModel(t, 100, 30)
	tab := model.Active().Active()
	tab.Focus = app.PaneEditor
	written := "select " + strings.Repeat("a_long_column_name, ", 12) + "id from orders"
	tab.Editor = app.NewEditorBuffer(written, 0)

	if !strings.Contains(stripEscapes(model.View().Content), model.icons.Icon(cfg.IconStepOn)) {
		t.Error("a line cut at the right shows no mark at its end")
	}

	tab.Editor = app.NewEditorBuffer(written, len(written))
	if !strings.Contains(stripEscapes(model.View().Content), model.icons.Icon(cfg.IconStepBack)+" 1 ") {
		t.Error("a line moved left shows no mark before its number")
	}
}
