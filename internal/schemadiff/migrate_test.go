package schemadiff_test

import (
	"context"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/schemadiff"
)

// migrate runs the script of source to target on source, and returns what still differs.
func migrate(
	t *testing.T, source, target db.Session, sourceSchema, targetSchema string,
) []schemadiff.Difference {
	t.Helper()
	ctx := context.Background()
	read := func(session db.Session, schema string) schemadiff.Snapshot {
		snapshot, err := schemadiff.ReadSnapshot(ctx, session, schema)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	family := core.ResolveEngineInfo(source.Describe().Profile.Engine).Family
	script, err := schemadiff.WriteScript(ctx, schemadiff.ScriptRequest{
		Source: read(source, sourceSchema), Target: read(target, targetSchema),
		Family: family, Dialect: source.Dialect(), TargetCatalog: target,
		SessionSchema: source.Describe().DefaultSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(script.Notes) > 0 {
		t.Errorf("the script has notes: %v", script.Notes)
	}
	written := script.Write()
	t.Log("\n" + written)
	for _, statement := range source.Language().SplitStatements(written) {
		if _, err := source.RunQuery(ctx, statement, dbtest.ReadEverything, nil); err != nil {
			t.Fatalf("the script failed at %q: %v", statement, err)
		}
	}
	return schemadiff.Compare(read(source, sourceSchema), read(target, targetSchema))
}
