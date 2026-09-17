package azuresql

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/sqlserver"
)

// Support is everything known about Azure SQL Database before a connection exists. It speaks
// TDS, so it takes the SQL Server dialect and language.
var Support = sqlserver.BuildSupport(core.EngineAzureSQL)
