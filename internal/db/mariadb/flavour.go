package mariadb

import (
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/mysql"
)

// Flavour has neither the plan tree of MySQL nor `explain analyze`. It
// writes JSON, and it measures a plan with its own keyword.
var Flavour = mysql.Flavour{
	BuildExplainStatement: func(sql string, analyze bool) string {
		if analyze {
			return "analyze format=json " + sql
		}
		return "explain format=json " + sql
	},
	ReadPlan: func(result db.QueryResult, analyzed bool) (db.QueryPlan, bool) {
		return ReadPlan(mysql.ReadFirstCell(result), analyzed)
	},
	ReadOnlyStatement:      "set session transaction read only",
	BuildKillStatement:     mysql.BuildKillStatement,
	ListLockWaitsStatement: listLockWaitsSQL,
}

// The table name of a lock is quoted with backticks, char(96).
const listLockWaitsSQL = `
  select waiting.trx_mysql_thread_id                  as blocked_pid,
         coalesce(waiting.trx_query, '')              as blocked_query,
         timestampdiff(microsecond, waiting.trx_wait_started, now(6)) div 1000 as waiting_ms,
         coalesce(held.lock_mode, '')                 as mode,
         replace(substring_index(held.lock_table, '.', -1), char(96), '') as relation,
         holder.trx_mysql_thread_id                   as blocking_pid,
         coalesce(holder.trx_query, '')               as blocking_query,
         timestampdiff(microsecond, holder.trx_started, now(6)) div 1000 as blocking_ms
    from information_schema.innodb_lock_waits wait
    join information_schema.innodb_trx waiting on waiting.trx_id = wait.requesting_trx_id
    join information_schema.innodb_trx holder on holder.trx_id = wait.blocking_trx_id
    join information_schema.innodb_locks held on held.lock_id = wait.blocking_lock_id
   where database() is null or held.lock_table like concat(char(96), database(), char(96), '.%')
   order by waiting.trx_wait_started, blocked_pid, blocking_pid
`
