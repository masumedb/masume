//go:build integration

package postgres_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestResultReadsAUUIDAsText(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)

	answered, err := session.RunQuery(context.Background(),
		"select uuid 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', "+
			"array[uuid 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11']", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if held := answered.Rows[0][0]; held != "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11" {
		t.Errorf("the uuid reads %#v", held)
	}
	wanted := []any{"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"}
	if held := answered.Rows[0][1]; !reflect.DeepEqual(held, wanted) {
		t.Errorf("the uuid array reads %#v, wanted %#v", held, wanted)
	}
}

func TestBatchReadsAUUIDAsText(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)

	answered, err := session.RunQuery(context.Background(),
		"select 1; select uuid 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if held := answered.Rows[0][0]; held != "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11" {
		t.Errorf("the uuid reads %#v", held)
	}
}
