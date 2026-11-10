package app

import (
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query/statement"
)

// A new catalog read replaces the names, the lookups and the suggestions kept for the read
// before it.
func TestCatalogCachesFollowANewCatalogRead(t *testing.T) {
	connection := buildCachedConnection(t)
	_, tables, _ := connection.CompletionSources()
	if len(tables) != 6 {
		t.Fatalf("the completion names hold %d tables, wanted 6", len(tables))
	}
	invoices := statement.SelectSource{Name: "invoices"}
	if _, found := connection.FindCatalogTable(invoices); found {
		t.Fatal("invoices was found before the catalog held it")
	}
	if suggestion := connection.SuggestTableName("invoice"); suggestion != "" {
		t.Fatalf("the suggestion for invoice is %q before the catalog held invoices", suggestion)
	}

	connection.Catalog.Tables = append(connection.Catalog.Tables,
		db.TableRef{Schema: "public", Name: "invoices", Kind: db.RelationTable})
	connection.Catalog.ReadAt = connection.Catalog.ReadAt.Add(time.Second)

	if _, tables, _ := connection.CompletionSources(); len(tables) != 8 {
		t.Errorf("the completion names hold %d tables after the read, wanted 8", len(tables))
	}
	if _, found := connection.FindCatalogTable(invoices); !found {
		t.Error("invoices was not found after the catalog read")
	}
	if suggestion := connection.SuggestTableName("invoice"); suggestion != "invoices" {
		t.Errorf("the suggestion for invoice is %q, wanted invoices", suggestion)
	}
}
