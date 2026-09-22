package mariadb

import (
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/mysql"
)

// Support is everything known about MariaDB before a connection exists. It speaks
// the mysql protocol, so it takes that dialect and language.
var Support = mysql.BuildSupport(core.EngineMariadb)
