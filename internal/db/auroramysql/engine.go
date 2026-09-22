package auroramysql

import (
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/mysql"
)

// Support is everything known about Aurora MySQL before a connection exists. It speaks
// the mysql protocol, so it takes that dialect and language.
var Support = mysql.BuildSupport(core.EngineAuroraMysql)
