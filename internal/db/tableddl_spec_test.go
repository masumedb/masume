package db_test

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/postgres"
)

func TestRenderTableDDLWritesTheIndexOfAConstraintOnce(t *testing.T) {
	detail := db.TableDetail{
		Table: db.TableRef{Schema: "public", Name: "customers"},
		Columns: []db.ColumnDetail{
			{Name: "id", DataType: "integer"}, {Name: "email", DataType: "text"},
			{Name: "slot", DataType: "tstzrange"},
		},
	}
	indexes := []db.IndexDetail{
		{Name: "customers_pkey", IsPrimary: true, IsUnique: true,
			Definition: "CREATE UNIQUE INDEX customers_pkey ON public.customers USING btree (id)"},
		{Name: "customers_email_key", IsUnique: true,
			Definition: "CREATE UNIQUE INDEX customers_email_key ON public.customers USING btree (email)"},
		{Name: "customers_slot_excl",
			Definition: "CREATE INDEX customers_slot_excl ON public.customers USING gist (slot)"},
		{Name: "customers_lower_email_idx",
			Definition: "CREATE INDEX customers_lower_email_idx ON public.customers USING btree (lower(email))"},
	}
	constraints := []db.ConstraintDetail{
		{Name: "customers_pkey", Kind: db.ConstraintPrimaryKey, Definition: "PRIMARY KEY (id)"},
		{Name: "customers_email_key", Kind: db.ConstraintUnique, Definition: "UNIQUE (email)"},
		{Name: "customers_slot_excl", Kind: db.ConstraintExclusion,
			Definition: "EXCLUDE USING gist (slot WITH &&)"},
	}

	written := strings.Join(
		db.RenderTableDDL(detail, indexes, constraints, postgres.Dialect, false), "\n")
	for _, name := range []string{"customers_email_key", "customers_slot_excl"} {
		if count := strings.Count(written, name); count != 1 {
			t.Errorf("%s is written %d times:\n%s", name, count, written)
		}
	}
	if !strings.Contains(written, "CREATE INDEX customers_lower_email_idx") {
		t.Errorf("the definition has no plain index:\n%s", written)
	}
}
