package neon

import (
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/postgres"
)

// Support is everything known about Neon before a connection exists. It speaks
// the postgres protocol, so it takes that dialect and language.
var Support = postgres.BuildSupport(core.EngineNeon)
