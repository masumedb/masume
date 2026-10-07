package tidb

import (
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/mysql"
)

// Flavour writes a plan as rows, and it has no read-only session. The server
// takes the statement and does nothing under it, so a profile that asks for one is
// refused rather than opened on a promise the server would not keep.
var Flavour = mysql.Flavour{
	BuildExplainStatement: func(sql string, analyze bool) string {
		if analyze {
			return "explain analyze " + sql
		}
		return "explain " + sql
	},
	ReadPlan: func(result db.QueryResult, analyzed bool) (db.QueryPlan, bool) {
		rows, order := mysql.ReadNamedPlanRows(result)
		return ReadPlan(rows, order, analyzed)
	},
	ReadOnlyStatement:      "",
	BuildKillStatement:     mysql.BuildKillStatement,
	ListLockWaitsStatement: listLockWaitsSQL,
}

// The server reports no lock mode.
const listLockWaitsSQL = `
  select waiting.session_id as blocked_pid,
         coalesce(waiting_process.info, waiting.current_sql_digest_text, '') as blocked_query,
         coalesce(waiting_process.time, 0) * 1000 as waiting_ms,
         ''                                 as mode,
         coalesce(json_unquote(json_extract(wait.key_info, '$.table_name')), '') as relation,
         holder.session_id                  as blocking_pid,
         coalesce(holder_process.info, holder.current_sql_digest_text, '') as blocking_query,
         timestampdiff(microsecond, holder.start_time, now(6)) div 1000 as blocking_ms
    from information_schema.data_lock_waits wait
    join information_schema.cluster_tidb_trx waiting on waiting.id = wait.trx_id
    join information_schema.cluster_tidb_trx holder on holder.id = wait.current_holding_trx_id
    left join information_schema.cluster_processlist waiting_process
      on waiting_process.id = waiting.session_id and waiting_process.instance = waiting.instance
    left join information_schema.cluster_processlist holder_process
      on holder_process.id = holder.session_id and holder_process.instance = holder.instance
   where database() is null
      or json_unquote(json_extract(wait.key_info, '$.db_name')) = database()
   order by waiting_ms desc, blocked_pid, blocking_pid
`
