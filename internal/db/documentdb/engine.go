package documentdb

import (
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/mongo"
)

// Support is everything known about Amazon DocumentDB before a connection exists. It speaks
// the MongoDB wire protocol, so it takes that dialect and language.
var Support = mongo.BuildSupport(core.EngineDocumentdb)
