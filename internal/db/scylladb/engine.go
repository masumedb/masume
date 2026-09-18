package scylladb

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/cassandra"
)

// Support is everything known about ScyllaDB before a connection exists. It speaks the
// Cassandra protocol, so it takes the CQL dialect and language.
var Support = cassandra.BuildSupport(core.EngineScylladb)
