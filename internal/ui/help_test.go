package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

func TestTheHelpDrawsTheKeysInTheMutedInk(t *testing.T) {
	for _, term := range []string{"", "query tab"} {
		model := buildOfflineModel(t, 120, 34)
		connection := model.Active()
		connection.Overlay = app.Overlay{
			Kind: app.OverlayHelp, Draft: app.NewEditorBuffer(term, len(term)),
		}
		chord := model.registry.FormatFirstActionChordName(cfg.ScopeGlobal, ActionNewQueryTab)
		muted := describeInk(model.styles.Theme.Muted)
		accent := describeInk(model.styles.Theme.Accent)

		found := false
		for _, line := range strings.Split(model.render(), "\n") {
			if !strings.Contains(stripEscapes(line), "new query tab") {
				continue
			}
			found = true
			keys := strings.Builder{}
			for _, cell := range mapCells(line) {
				if strings.TrimSpace(cell.text) == "" || cell.text == "│" {
					continue
				}
				if strings.Contains(cell.sgr, accent) {
					t.Errorf("term %q: the help row draws %q in the accent ink", term, cell.text)
				}
				if strings.Contains(cell.sgr, muted) {
					keys.WriteString(cell.text)
				}
			}
			if !strings.HasPrefix(keys.String(), strings.ReplaceAll(chord, " ", "")) {
				t.Errorf("term %q: the muted cells read %q, wanted the key %q first",
					term, keys.String(), chord)
			}
		}
		if !found {
			t.Errorf("term %q: the help has no row for a new query tab", term)
		}
	}
}
