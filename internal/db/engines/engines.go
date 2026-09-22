// Package engines registers engine capabilities, adapters, and query composers.
package engines

import (
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/auroramysql"
	"github.com/masumedb/masume/internal/db/aurorapostgres"
	"github.com/masumedb/masume/internal/db/azuresql"
	"github.com/masumedb/masume/internal/db/cassandra"
	"github.com/masumedb/masume/internal/db/clickhouse"
	"github.com/masumedb/masume/internal/db/cockroach"
	"github.com/masumedb/masume/internal/db/documentdb"
	"github.com/masumedb/masume/internal/db/mariadb"
	"github.com/masumedb/masume/internal/db/mongo"
	"github.com/masumedb/masume/internal/db/mysql"
	"github.com/masumedb/masume/internal/db/neon"
	"github.com/masumedb/masume/internal/db/planetscale"
	"github.com/masumedb/masume/internal/db/postgres"
	"github.com/masumedb/masume/internal/db/redis"
	"github.com/masumedb/masume/internal/db/redshift"
	"github.com/masumedb/masume/internal/db/scylladb"
	"github.com/masumedb/masume/internal/db/sqlite"
	"github.com/masumedb/masume/internal/db/sqlserver"
	"github.com/masumedb/masume/internal/db/supabase"
	"github.com/masumedb/masume/internal/db/tidb"
	"github.com/masumedb/masume/internal/db/timescale"
	"github.com/masumedb/masume/internal/db/turso"
	"github.com/masumedb/masume/internal/db/yugabyte"
)

// support holds one entry per engine. An engine missing here cannot be opened.
var support = map[core.Engine]db.EngineSupport{
	core.EnginePostgres:   postgres.Support,
	core.EngineMysql:      mysql.Support,
	core.EngineSqlite:     sqlite.Support,
	core.EngineRedis:      redis.Support,
	core.EngineCassandra:  cassandra.Support,
	core.EngineScylladb:   scylladb.Support,
	core.EngineTurso:      turso.Support,
	core.EngineMongo:      mongo.Support,
	core.EngineDocumentdb: documentdb.Support,

	core.EngineSqlserver:  sqlserver.Support,
	core.EngineAzureSQL:   azuresql.Support,
	core.EngineClickhouse: clickhouse.Support,

	core.EngineCockroach:      cockroach.Support,
	core.EngineTimescale:      timescale.Support,
	core.EngineRedshift:       redshift.Support,
	core.EngineNeon:           neon.Support,
	core.EngineSupabase:       supabase.Support,
	core.EngineAuroraPostgres: aurorapostgres.Support,
	core.EngineYugabyte:       yugabyte.Support,

	core.EngineMariadb:     mariadb.Support,
	core.EngineTidb:        tidb.Support,
	core.EnginePlanetscale: planetscale.Support,
	core.EngineAuroraMysql: auroramysql.Support,
}

// ResolveSupport returns everything known about that engine before a connection exists.
func ResolveSupport(engine core.Engine) db.EngineSupport {
	return support[engine]
}
