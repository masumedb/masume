package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/db"
)

// The saved catalog fills the explorer while the first read of the server runs.
func TestTheSavedCatalogIsDrawnWhileTheFirstReadRuns(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Catalog.Loading = true

	model.readCachedCatalogAnswer(catalogCachedMsg{
		ConnectionID: model.ActiveID(), Schemas: []string{"public"},
		Tables: []db.TableRef{{Schema: "public", Name: "orders", Kind: db.RelationTable}},
	})

	if len(connection.Catalog.Tables) != 1 || connection.Catalog.Tables[0].Name != "orders" {
		t.Fatalf("the catalog holds %+v, wanted the saved orders table", connection.Catalog.Tables)
	}
	if !connection.Catalog.Loading {
		t.Error("the saved catalog ended the read of the server")
	}
}

// The answer of the server comes first, so the saved catalog that arrives later is dropped.
func TestTheSavedCatalogIsDroppedAfterTheServerAnswered(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Catalog.Loading = true
	model.readCatalogAnswer(catalogReadMsg{
		ConnectionID: model.ActiveID(), Schemas: []string{"public"},
		Tables: []db.TableRef{{Schema: "public", Name: "invoices", Kind: db.RelationTable}},
	})

	model.readCachedCatalogAnswer(catalogCachedMsg{
		ConnectionID: model.ActiveID(), Schemas: []string{"public"},
		Tables: []db.TableRef{{Schema: "public", Name: "orders", Kind: db.RelationTable}},
	})

	if len(connection.Catalog.Tables) != 1 || connection.Catalog.Tables[0].Name != "invoices" {
		t.Errorf("the catalog holds %+v, wanted the invoices table the server answered",
			connection.Catalog.Tables)
	}
}
