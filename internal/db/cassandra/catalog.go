package cassandra

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// The kinds system_schema.columns reports for a column of the key.
const (
	kindPartitionKey = "partition_key"
	kindClustering   = "clustering"
)

// readCatalog returns a catalog read as rows keyed by column name.
func (session *cassandraSession) readCatalog(
	ctx context.Context, written string, params ...any,
) ([]map[string]any, error) {
	held := session.cluster.Query(written, params...).WithContext(ctx)
	defer held.Release()

	iterator := held.Iter()
	names := []string{}
	for _, column := range iterator.Columns() {
		names = append(names, column.Name)
	}

	read := []map[string]any{}
	for {
		values, more := readRow(iterator)
		if !more {
			break
		}
		row := map[string]any{}
		for at, name := range names {
			row[name] = values[at]
		}
		read = append(read, row)
	}
	if err := iterator.Close(); err != nil {
		return nil, db.WrapDatabaseError(err)
	}
	return read, nil
}

// ListSchemas returns every keyspace of the cluster.
func (session *cassandraSession) ListSchemas(ctx context.Context) ([]string, error) {
	rows, err := session.readCatalog(
		ctx, "select keyspace_name from system_schema.keyspaces")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, db.ReadAnyText(row["keyspace_name"]))
	}
	slices.Sort(names)
	return names, nil
}

// ListTables returns every table and materialized view of the cluster.
func (session *cassandraSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	tables, err := session.readCatalog(
		ctx, "select keyspace_name, table_name from system_schema.tables")
	if err != nil {
		return nil, err
	}
	views, viewErr := session.readCatalog(
		ctx, "select keyspace_name, view_name from system_schema.views")
	if viewErr != nil {
		return nil, viewErr
	}

	listed := make([]db.TableRef, 0, len(tables)+len(views))
	for _, row := range tables {
		listed = append(listed, db.TableRef{
			Schema: db.ReadAnyText(row["keyspace_name"]),
			Name:   db.ReadAnyText(row["table_name"]), Kind: db.RelationTable,
		})
	}
	for _, row := range views {
		listed = append(listed, db.TableRef{
			Schema: db.ReadAnyText(row["keyspace_name"]),
			Name:   db.ReadAnyText(row["view_name"]), Kind: db.RelationMaterializedView,
		})
	}
	slices.SortFunc(listed, func(left, right db.TableRef) int {
		if held := strings.Compare(left.Schema, right.Schema); held != 0 {
			return held
		}
		return strings.Compare(left.Name, right.Name)
	})
	return listed, nil
}

// ListRoles returns the roles of the cluster. A cluster that authenticates nobody keeps no
// role table a session can read.
func (session *cassandraSession) ListRoles(ctx context.Context) ([]db.DbRole, error) {
	rows, err := session.readCatalog(
		ctx, "select role, is_superuser, can_login from system_auth.roles")
	if err != nil {
		return nil, nil
	}
	roles := make([]db.DbRole, 0, len(rows))
	for _, row := range rows {
		detail := []string{}
		if held, is := row["is_superuser"].(bool); is && held {
			detail = append(detail, "superuser")
		}
		if held, can := row["can_login"].(bool); can && held {
			detail = append(detail, "login")
		}
		roles = append(roles, db.DbRole{
			Name: db.ReadAnyText(row["role"]), Detail: strings.Join(detail, ", "),
		})
	}
	return roles, nil
}

// ListSchemaObjects returns the functions and the aggregates of the cluster.
func (session *cassandraSession) ListSchemaObjects(
	ctx context.Context,
) ([]db.SchemaObject, error) {
	objects := []db.SchemaObject{}
	for _, held := range []struct {
		table  string
		column string
		detail string
	}{
		{"functions", "function_name", "function"},
		{"aggregates", "aggregate_name", "aggregate"},
	} {
		rows, err := session.readCatalog(ctx, fmt.Sprintf(
			"select keyspace_name, %s from system_schema.%s", held.column, held.table))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			objects = append(objects, db.SchemaObject{
				Schema: db.ReadAnyText(row["keyspace_name"]),
				Name:   db.ReadAnyText(row[held.column]),
				Kind:   db.ObjectFunction, Detail: held.detail,
			})
		}
	}
	return objects, nil
}

// ListRelationships returns nothing. CQL has no foreign key.
func (session *cassandraSession) ListRelationships(
	context.Context,
) ([]db.Relationship, error) {
	return nil, nil
}

// readTableColumns returns the columns of a relation, in the order the table holds them:
// the partition key, then the clustering columns, then the rest by name.
func (session *cassandraSession) readTableColumns(
	ctx context.Context, table db.TableRef,
) ([]map[string]any, error) {
	rows, err := session.readCatalog(ctx,
		`select column_name, type, kind, position, clustering_order
		   from system_schema.columns
		  where keyspace_name = ? and table_name = ?`,
		table.Schema, table.Name)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(rows, func(left, right map[string]any) int {
		return compareColumnOrder(left, right)
	})
	return rows, nil
}

// compareColumnOrder orders the key columns first, each by its position, and the rest by
// name.
func compareColumnOrder(left, right map[string]any) int {
	leftRank, rightRank := rankColumnKind(left), rankColumnKind(right)
	if leftRank != rightRank {
		return leftRank - rightRank
	}
	if leftRank < 2 {
		leftAt := int(db.ReadNonNegativeCount(left["position"]))
		rightAt := int(db.ReadNonNegativeCount(right["position"]))
		if leftAt != rightAt {
			return leftAt - rightAt
		}
	}
	return strings.Compare(
		db.ReadAnyText(left["column_name"]), db.ReadAnyText(right["column_name"]))
}

// rankColumnKind returns the group a column is drawn in.
func rankColumnKind(row map[string]any) int {
	switch db.ReadAnyText(row["kind"]) {
	case kindPartitionKey:
		return 0
	case kindClustering:
		return 1
	}
	return 2
}

func (session *cassandraSession) DescribeTable(
	ctx context.Context, table db.TableRef,
) (db.TableDetail, error) {
	rows, err := session.readTableColumns(ctx, table)
	if err != nil {
		return db.TableDetail{}, err
	}

	columns := make([]db.ColumnDetail, 0, len(rows))
	for _, row := range rows {
		kind := db.ReadAnyText(row["kind"])
		columns = append(columns, db.ColumnDetail{
			Name:     db.ReadAnyText(row["column_name"]),
			DataType: db.ReadAnyText(row["type"]),
			// Every column outside the key takes a null, and a key column never does.
			Nullable:     kind != kindPartitionKey && kind != kindClustering,
			IsPrimaryKey: kind == kindPartitionKey || kind == kindClustering,
		})
	}
	return db.TableDetail{Table: table, Columns: columns}, nil
}

func (session *cassandraSession) ListIndexes(
	ctx context.Context, table db.TableRef,
) ([]db.IndexDetail, error) {
	rows, err := session.readCatalog(ctx,
		`select index_name, kind, options from system_schema.indexes
		  where keyspace_name = ? and table_name = ?`,
		table.Schema, table.Name)
	if err != nil {
		return nil, err
	}

	indexes := make([]db.IndexDetail, 0, len(rows))
	for _, row := range rows {
		indexes = append(indexes, db.IndexDetail{
			Name:       db.ReadAnyText(row["index_name"]),
			Definition: readIndexTarget(row["options"]),
		})
	}
	return indexes, nil
}

// readIndexTarget returns the column an index covers, which the catalog keeps in the
// options of the index.
func readIndexTarget(held any) string {
	options, is := held.(map[string]string)
	if !is {
		return ""
	}
	return options["target"]
}

func (session *cassandraSession) ListConstraints(
	ctx context.Context, table db.TableRef,
) ([]db.ConstraintDetail, error) {
	rows, err := session.readTableColumns(ctx, table)
	if err != nil {
		return nil, err
	}
	key := readKeyColumns(rows)
	if len(key) == 0 {
		return nil, nil
	}
	return []db.ConstraintDetail{{
		Name: table.Name + "_primary_key", Kind: db.ConstraintPrimaryKey,
		Definition: "PRIMARY KEY (" + strings.Join(key, ", ") + ")",
	}}, nil
}

// readKeyColumns returns the names of the key columns, the partition key first.
func readKeyColumns(rows []map[string]any) []string {
	names := []string{}
	for _, row := range rows {
		if rankColumnKind(row) < 2 {
			names = append(names, db.ReadAnyText(row["column_name"]))
		}
	}
	return names
}

func (session *cassandraSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	rows, err := session.readTableColumns(ctx, table)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, db.NewDatabaseError("no relation named %s.%s", table.Schema, table.Name)
	}

	dialect := session.Support.Dialect
	lines := []string{"create table " + dialect.BuildQualifiedName(table.Qualified()) + " ("}
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("    %s %s,",
			dialect.QuoteIdentifier(db.ReadAnyText(row["column_name"])),
			db.ReadAnyText(row["type"])))
	}
	lines = append(lines, "    "+buildPrimaryKeyClause(rows, dialect))
	lines = append(lines, ")"+buildClusteringOrder(rows, dialect)+";")

	indexes, indexErr := session.ListIndexes(ctx, table)
	if indexErr == nil {
		for _, index := range indexes {
			lines = append(lines, "", fmt.Sprintf("create index %s on %s (%s);",
				dialect.QuoteIdentifier(index.Name),
				dialect.BuildQualifiedName(table.Qualified()), index.Definition))
		}
	}
	return lines, nil
}

// buildPrimaryKeyClause writes the key of the table, with the partition key in brackets of
// its own where it holds more than one column.
func buildPrimaryKeyClause(rows []map[string]any, dialect *query.Dialect) string {
	partition, clustering := []string{}, []string{}
	for _, row := range rows {
		name := dialect.QuoteIdentifier(db.ReadAnyText(row["column_name"]))
		switch db.ReadAnyText(row["kind"]) {
		case kindPartitionKey:
			partition = append(partition, name)
		case kindClustering:
			clustering = append(clustering, name)
		}
	}
	if len(partition) == 0 {
		return ""
	}
	written := strings.Join(partition, ", ")
	if len(partition) > 1 {
		written = "(" + written + ")"
	}
	if len(clustering) > 0 {
		written += ", " + strings.Join(clustering, ", ")
	}
	return "primary key (" + written + ")"
}

// buildClusteringOrder writes the order the table keeps its rows in, where a clustering
// column is ordered backwards.
func buildClusteringOrder(rows []map[string]any, dialect *query.Dialect) string {
	written := []string{}
	for _, row := range rows {
		if db.ReadAnyText(row["kind"]) != kindClustering {
			continue
		}
		order := db.ReadAnyText(row["clustering_order"])
		if order == "" || order == "none" {
			order = "asc"
		}
		written = append(written, dialect.QuoteIdentifier(
			db.ReadAnyText(row["column_name"]))+" "+strings.ToUpper(order))
	}
	if len(written) == 0 {
		return ""
	}
	return " with clustering order by (" + strings.Join(written, ", ") + ")"
}

// BuildObjectDDL returns the definition of a function or an aggregate.
func (session *cassandraSession) BuildObjectDDL(
	ctx context.Context, object db.SchemaObject,
) ([]string, error) {
	rows, err := session.readCatalog(ctx,
		`select body, argument_types, return_type, language
		   from system_schema.functions
		  where keyspace_name = ? and function_name = ?`,
		object.Schema, object.Name)
	if err != nil || len(rows) == 0 {
		return nil, db.NewDatabaseError("no definition for %s.%s", object.Schema, object.Name)
	}

	dialect := session.Support.Dialect
	row := rows[0]
	name := dialect.BuildQualifiedName(
		query.QualifiedName{Schema: object.Schema, Name: object.Name})
	return []string{
		fmt.Sprintf("create function %s (%s)", name, readArgumentTypes(row["argument_types"])),
		"  returns null on null input",
		"  returns " + db.ReadAnyText(row["return_type"]),
		"  language " + db.ReadAnyText(row["language"]) + " as $$",
		db.ReadAnyText(row["body"]),
		"$$;",
	}, nil
}

// readArgumentTypes returns the argument types of a function as the catalog lists them.
func readArgumentTypes(held any) string {
	types, is := held.([]string)
	if !is {
		return ""
	}
	return strings.Join(types, ", ")
}
