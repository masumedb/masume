package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// Cancellation requires a known connection ID.
func TestCancelRunningQueryRefusesAnUnknownBackend(t *testing.T) {
	session := &postgresSession{SessionFacts: db.SessionFacts{
		Support: db.EngineSupport{EngineInfo: core.EngineInfo{
			Capabilities: core.Capabilities{CancelsRunningQuery: true},
		}},
	}}

	stopped, err := session.CancelRunningQuery(context.Background())
	if stopped {
		t.Error("a cancel with no backend id reported that it stopped one")
	}
	if err == nil {
		t.Fatal("a cancel with no backend id answered no error")
	}
	if described := db.DescribeError(err); described !=
		"cannot cancel the statement: the connection ID is unavailable" {
		t.Errorf("the cancel describes as %q", described)
	}
}

// Only PostgreSQL itself keeps statement statistics in an extension.
func TestBuildIdentityStatementAsksOnlyTheServersThatHoldTheCatalog(t *testing.T) {
	for _, one := range []struct {
		name    string
		flavour Flavour
		asks    bool
	}{
		{"standard", FlavourStandard, true},
		{"cockroach", FlavourCockroach, false},
		{"redshift", FlavourRedshift, false},
	} {
		written := buildIdentityStatement(one.flavour)
		if strings.Contains(written, "pg_extension") != one.asks {
			t.Errorf("%s reads %q, wanted the catalog asked: %v", one.name, written, one.asks)
		}
	}
}
