package supabase

import (
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/postgres"
)

// Support is everything known about Supabase before a connection exists. It speaks
// the postgres protocol, so it takes that dialect and language.
var Support = postgres.BuildSupport(core.EngineSupabase)
