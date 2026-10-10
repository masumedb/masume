package schemadiff

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/syntax"
)

// ScriptRequest is the input of an ALTER script that changes the source schema to the target.
type ScriptRequest struct {
	Source Snapshot
	Target Snapshot
	// Family and Dialect are the ones of the source server.
	Family  core.Family
	Dialect *query.Dialect
	// TargetCatalog reads view definitions, and MySQL table and column text.
	TargetCatalog db.CatalogReader
	// SessionSchema is the schema the source connection uses. The script sets the source
	// schema only where it is another one, and gives the session its schema back at the end.
	// On SQL Server and SQLite the source schema must be this schema.
	SessionSchema string
}

// Script is the statements of an ALTER script, and the differences it does not write.
type Script struct {
	Statements []string
	Notes      []string
}

// Write returns the script as text: the notes as comments, then one statement per block.
func (script Script) Write() string {
	var text strings.Builder
	for _, note := range script.Notes {
		text.WriteString("-- " + note + "\n")
	}
	if len(script.Statements) == 0 {
		if len(script.Notes) == 0 {
			text.WriteString("-- no differences\n")
		}
		return text.String()
	}
	if len(script.Notes) > 0 {
		text.WriteString("\n")
	}
	text.WriteString(strings.Join(script.Statements, "\n\n") + "\n")
	return text.String()
}

// The phases of a script, in the order they run.
const (
	phaseDropConstraints = iota
	phaseDropIndexes
	phaseDropViews
	phaseDropTables
	phaseCreateTables
	phaseAddColumns
	phaseAlterColumns
	phaseDropColumns
	phaseCreateIndexes
	phaseAddConstraints
	phaseAddForeignKeys
	phaseCreateViews
	phaseCount
)

// WriteScript writes the statements that change the source schema to the target schema.
func WriteScript(ctx context.Context, request ScriptRequest) (Script, error) {
	switch request.Family {
	case core.FamilyPostgres, core.FamilyMysql:
	case core.FamilySqlserver, core.FamilySqlite:
		if request.Source.Schema != request.SessionSchema {
			return Script{}, db.NewDatabaseError(
				"an ALTER script for SQL Server or SQLite needs the source schema to be " +
					"the default schema of the connection")
		}
	default:
		return Script{}, db.NewDatabaseError(
			"an ALTER script is written for the PostgreSQL and MySQL families, " +
				"SQL Server, and SQLite only")
	}
	writer := scriptWriter{
		ctx: ctx, request: request, definitions: map[string][]string{},
		sources: indexTables(request.Source), targets: indexTables(request.Target),
		rebuilt: map[string]bool{}, recreated: map[string]bool{},
	}
	found := Compare(request.Source, request.Target)
	if writer.isSqlite() {
		if err := writer.rebuildTables(found); err != nil {
			return Script{}, err
		}
	}
	replaced := map[string]bool{}
	for _, difference := range found {
		if err := writer.writeDifference(difference, replaced); err != nil {
			return Script{}, err
		}
	}

	script := Script{Notes: writer.notes}
	for _, phase := range writer.phases {
		script.Statements = append(script.Statements, phase...)
	}
	if len(script.Statements) > 0 && writer.isSqlite() {
		script.Statements = append(append(
			[]string{"PRAGMA foreign_keys = OFF;", "PRAGMA legacy_alter_table = ON;"},
			script.Statements...),
			"PRAGMA legacy_alter_table = OFF;", "PRAGMA foreign_keys = ON;")
	}
	if len(script.Statements) > 0 && request.Source.Schema != request.SessionSchema {
		first, last := writer.writeSchemaStatements()
		script.Statements = append(append([]string{first}, script.Statements...), last...)
	}
	return script, nil
}

type scriptWriter struct {
	ctx     context.Context
	request ScriptRequest
	sources map[string]TableSnapshot
	targets map[string]TableSnapshot
	// The definition lines of target relations, read once per relation.
	definitions map[string][]string
	phases      [phaseCount][]string
	notes       []string
	// The SQLite tables the script creates again with the target definition.
	rebuilt map[string]bool
	// The indexes and constraints, by table and name, that a SQL Server column change
	// drops and creates again.
	recreated map[string]bool
}

func indexTables(snapshot Snapshot) map[string]TableSnapshot {
	tables := map[string]TableSnapshot{}
	for _, table := range snapshot.Tables {
		tables[table.Table.Name] = table
	}
	return tables
}

func (writer *scriptWriter) add(phase int, statement string) {
	writer.phases[phase] = append(writer.phases[phase], statement)
}

func (writer *scriptWriter) quote(name string) string {
	return writer.request.Dialect.QuoteIdentifierIfNeeded(name)
}

func (writer *scriptWriter) isMysql() bool {
	return writer.request.Family == core.FamilyMysql
}

func (writer *scriptWriter) isSqlserver() bool {
	return writer.request.Family == core.FamilySqlserver
}

func (writer *scriptWriter) isSqlite() bool {
	return writer.request.Family == core.FamilySqlite
}

// writeSchemaStatements returns the statements before and after the script that set the
// source schema for the script alone: a transaction on PostgreSQL, two USE on MySQL.
func (writer *scriptWriter) writeSchemaStatements() (string, []string) {
	schema := writer.quote(writer.request.Source.Schema)
	if !writer.isMysql() {
		return "BEGIN;\nSET LOCAL search_path TO " + schema + ";", []string{"COMMIT;"}
	}
	if writer.request.SessionSchema == "" {
		return "USE " + schema + ";", nil
	}
	return "USE " + schema + ";", []string{"USE " + writer.quote(writer.request.SessionSchema) + ";"}
}

// writeDifference adds the statements of one difference. A view is dropped and created
// again once, whatever changed in it.
func (writer *scriptWriter) writeDifference(difference Difference, replaced map[string]bool) error {
	source, inSource := writer.sources[difference.Table]
	target, inTarget := writer.targets[difference.Table]
	bothTables := inSource && inTarget &&
		source.Table.Kind == db.RelationTable && target.Table.Kind == db.RelationTable

	if difference.Part == PartTable || !bothTables {
		if replaced[difference.Table] {
			return nil
		}
		replaced[difference.Table] = true
		if inSource {
			writer.dropRelation(source.Table)
		}
		if inTarget {
			return writer.createRelation(target)
		}
		return nil
	}
	if writer.rebuilt[difference.Table] {
		return nil
	}

	switch difference.Part {
	case PartColumn:
		return writer.writeColumn(difference, source, target)
	case PartIndex:
		writer.writeIndex(difference, source, target)
	case PartConstraint:
		writer.writeConstraint(difference, source, target)
	}
	return nil
}

func (writer *scriptWriter) dropRelation(table db.TableRef) {
	name := writer.quote(table.Name)
	switch table.Kind {
	case db.RelationView:
		writer.add(phaseDropViews, "DROP VIEW "+name+";")
	case db.RelationMaterializedView:
		writer.add(phaseDropViews, "DROP MATERIALIZED VIEW "+name+";")
	default:
		writer.add(phaseDropTables, "DROP TABLE "+name+";")
	}
}

func (writer *scriptWriter) createRelation(target TableSnapshot) error {
	if target.Table.Kind != db.RelationTable {
		lines, err := writer.readDefinition(target.Table)
		if err != nil {
			return err
		}
		writer.add(phaseCreateViews, endStatement(strings.Join(lines, "\n")))
		return nil
	}
	if writer.isSqlite() {
		created, err := writer.readCreateTable(target)
		if err != nil {
			return err
		}
		writer.add(phaseCreateTables, created)
		writer.createIndexes(target)
		return nil
	}

	for _, constraint := range target.Constraints {
		if constraint.Kind == db.ConstraintForeignKey {
			writer.add(phaseAddForeignKeys, writer.writeAddConstraint(target.Table, constraint))
		}
	}
	if writer.isMysql() {
		lines, err := writer.readDefinition(target.Table)
		if err != nil {
			return err
		}
		writer.add(phaseCreateTables, endStatement(strings.Join(removeForeignKeyLines(lines), "\n")))
		return nil
	}

	for _, column := range target.Columns {
		writer.createSequence(column)
	}
	detail := db.TableDetail{
		Table:   db.TableRef{Schema: writer.request.Source.Schema, Name: target.Table.Name},
		Columns: target.Columns,
	}
	constraints := []db.ConstraintDetail{}
	for _, constraint := range target.Constraints {
		if constraint.Kind != db.ConstraintForeignKey {
			constraints = append(constraints, constraint)
		}
	}
	writer.add(phaseCreateTables, strings.Join(db.RenderTableDDL(detail,
		listOwnIndexes(target), constraints, writer.request.Dialect, false), "\n"))
	return nil
}

// removeForeignKeyLines removes the foreign keys of a CREATE TABLE, and the comma the line
// before them then has too many.
func removeForeignKeyLines(lines []string) []string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(strings.ToUpper(line), " FOREIGN KEY ") {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), ")") && len(kept) > 0 {
			kept[len(kept)-1] = strings.TrimSuffix(kept[len(kept)-1], ",")
		}
		kept = append(kept, line)
	}
	return kept
}

// readDefinition reads the definition of a target relation, without the target schema name.
func (writer *scriptWriter) readDefinition(table db.TableRef) ([]string, error) {
	if lines, read := writer.definitions[table.Name]; read {
		return lines, nil
	}
	lines, err := writer.request.TargetCatalog.BuildTableDDL(writer.ctx, table)
	if err != nil {
		return nil, err
	}
	for at, line := range lines {
		lines[at] = stripText(line, writer.request.Target.Schema)
	}
	writer.definitions[table.Name] = lines
	return lines, nil
}

func endStatement(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasSuffix(text, ";") {
		return text
	}
	return text + ";"
}

func (writer *scriptWriter) alterTable(table db.TableRef) string {
	return "ALTER TABLE " + writer.quote(table.Name) + " "
}

func findColumn(table TableSnapshot, name string) db.ColumnDetail {
	for _, column := range table.Columns {
		if column.Name == name {
			return column
		}
	}
	return db.ColumnDetail{}
}

func (writer *scriptWriter) writeColumn(difference Difference, source, target TableSnapshot) error {
	alter := writer.alterTable(source.Table)
	name := writer.quote(difference.Name)
	switch difference.Change {
	case ChangeRemoved:
		if writer.isSqlserver() && findColumn(source, difference.Name).HasDefault {
			writer.add(phaseDropColumns, writer.dropSqlserverDefault(source.Table, difference.Name))
		}
		writer.add(phaseDropColumns, alter+"DROP COLUMN "+name+";")
		return nil
	case ChangeAdded:
		definition, err := writer.writeColumnDefinition(target, difference.Name)
		if err != nil {
			return err
		}
		if writer.isSqlserver() {
			writer.add(phaseAddColumns, alter+"ADD "+definition+";")
			return nil
		}
		writer.add(phaseAddColumns, alter+"ADD COLUMN "+definition+";")
		return nil
	}

	from, to := findColumn(source, difference.Name), findColumn(target, difference.Name)
	if writer.isSqlserver() {
		writer.alterSqlserverColumn(source, target, from, to)
		return nil
	}
	if writer.isMysql() {
		definition, err := writer.writeColumnDefinition(target, difference.Name)
		if err != nil {
			return err
		}
		writer.add(phaseAlterColumns, alter+"MODIFY COLUMN "+definition+";")
		return nil
	}
	if from.IsGenerated != to.IsGenerated || from.IsIdentityAlways != to.IsIdentityAlways {
		writer.notes = append(writer.notes, "not written: the identity or generation of "+
			source.Table.Name+"."+difference.Name)
	}
	column := alter + "ALTER COLUMN " + name + " "
	if from.DataType != to.DataType {
		writer.add(phaseAlterColumns, column+"TYPE "+to.DataType+";")
	}
	if from.Nullable != to.Nullable {
		if to.Nullable {
			writer.add(phaseAlterColumns, column+"DROP NOT NULL;")
		} else {
			writer.add(phaseAlterColumns, column+"SET NOT NULL;")
		}
	}
	if !to.IsGenerated && !to.IsIdentityAlways &&
		(from.HasDefault != to.HasDefault || from.DefaultValue != to.DefaultValue) {
		if to.HasDefault {
			writer.add(phaseAlterColumns, column+"SET DEFAULT "+to.DefaultValue+";")
		} else {
			writer.add(phaseAlterColumns, column+"DROP DEFAULT;")
		}
	}
	return nil
}

// writeColumnDefinition returns the target column as a CREATE TABLE writes it. On MySQL it is
// the line of SHOW CREATE TABLE, which quotes the default as the server does.
func (writer *scriptWriter) writeColumnDefinition(target TableSnapshot, name string) (string, error) {
	if !writer.isMysql() {
		column := findColumn(target, name)
		writer.createSequence(column)
		return db.RenderColumnDefinition(column, writer.request.Dialect, false), nil
	}
	lines, err := writer.readDefinition(target.Table)
	if err != nil {
		return "", err
	}
	prefix := writer.request.Dialect.QuoteIdentifier(name) + " "
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSuffix(trimmed, ","), nil
		}
	}
	return "", db.NewDatabaseError("the definition of %s has no column %s", target.Table.Name, name)
}

// createSequence creates the sequence a PostgreSQL `nextval('name'::regclass)` default reads.
func (writer *scriptWriter) createSequence(column db.ColumnDetail) {
	match := nextvalPattern.FindStringSubmatch(column.DefaultValue)
	if !column.HasDefault || match == nil {
		return
	}
	statement := "CREATE SEQUENCE IF NOT EXISTS " + match[1] + ";"
	if !slices.Contains(writer.phases[phaseCreateTables], statement) {
		writer.add(phaseCreateTables, statement)
	}
}

var nextvalPattern = regexp.MustCompile(`^nextval\('([^']+)'(?:::regclass)?\)$`)

// listOwnIndexes returns the indexes that belong to no constraint.
func listOwnIndexes(table TableSnapshot) []db.IndexDetail {
	kept := []db.IndexDetail{}
	for _, index := range table.Indexes {
		if !index.IsPrimary && !holdsConstraint(table, index.Name) {
			kept = append(kept, index)
		}
	}
	return kept
}

func holdsConstraint(table TableSnapshot, name string) bool {
	for _, constraint := range table.Constraints {
		if constraint.Name == name {
			return true
		}
	}
	return false
}

func findIndex(table TableSnapshot, name string) (db.IndexDetail, bool) {
	for _, index := range table.Indexes {
		if index.Name == name {
			return index, true
		}
	}
	return db.IndexDetail{}, false
}

func (writer *scriptWriter) writeIndex(difference Difference, source, target TableSnapshot) {
	if writer.recreated[difference.Table+"."+difference.Name] {
		return
	}
	if difference.Change != ChangeAdded {
		if index, held := findIndex(source, difference.Name); held &&
			!index.IsPrimary && !holdsConstraint(source, index.Name) {
			drop := "DROP INDEX " + writer.quote(index.Name)
			if writer.isMysql() || writer.isSqlserver() {
				drop += " ON " + writer.quote(source.Table.Name)
			}
			writer.add(phaseDropIndexes, drop+";")
		}
	}
	if difference.Change != ChangeRemoved {
		if index, held := findIndex(target, difference.Name); held &&
			!index.IsPrimary && !holdsConstraint(target, index.Name) {
			writer.add(phaseCreateIndexes, endStatement(index.Definition))
		}
	}
}

func findConstraint(table TableSnapshot, name string) (db.ConstraintDetail, bool) {
	for _, constraint := range table.Constraints {
		if constraint.Name == name {
			return constraint, true
		}
	}
	return db.ConstraintDetail{}, false
}

func (writer *scriptWriter) writeConstraint(difference Difference, source, target TableSnapshot) {
	if writer.recreated[difference.Table+"."+difference.Name] {
		return
	}
	if difference.Change != ChangeAdded {
		if constraint, held := findConstraint(source, difference.Name); held {
			writer.add(phaseDropConstraints, writer.writeDropConstraint(source.Table, constraint))
		}
	}
	if difference.Change != ChangeRemoved {
		if constraint, held := findConstraint(target, difference.Name); held {
			phase := phaseAddConstraints
			if constraint.Kind == db.ConstraintForeignKey {
				phase = phaseAddForeignKeys
			}
			writer.add(phase, writer.writeAddConstraint(target.Table, constraint))
		}
	}
}

func (writer *scriptWriter) writeDropConstraint(table db.TableRef, constraint db.ConstraintDetail) string {
	alter := writer.alterTable(table)
	name := writer.quote(constraint.Name)
	if !writer.isMysql() {
		return alter + "DROP CONSTRAINT " + name + ";"
	}
	switch constraint.Kind {
	case db.ConstraintPrimaryKey:
		return alter + "DROP PRIMARY KEY;"
	case db.ConstraintForeignKey:
		return alter + "DROP FOREIGN KEY " + name + ";"
	case db.ConstraintUnique:
		return alter + "DROP INDEX " + name + ";"
	}
	return alter + "DROP CONSTRAINT " + name + ";"
}

func (writer *scriptWriter) writeAddConstraint(table db.TableRef, constraint db.ConstraintDetail) string {
	alter := writer.alterTable(table)
	definition := constraint.Definition
	if constraint.Kind == db.ConstraintCheck &&
		!strings.HasPrefix(strings.ToUpper(strings.TrimSpace(definition)), "CHECK") {
		definition = "CHECK " + definition
	}
	if writer.isMysql() && constraint.Kind == db.ConstraintPrimaryKey {
		return alter + "ADD " + definition + ";"
	}
	return alter + "ADD CONSTRAINT " + writer.quote(constraint.Name) + " " + definition + ";"
}

// alterSqlserverColumn changes the type, nullability and default of one column. SQL Server
// changes the type and the nullability in one statement, and keeps a default as a
// constraint, which also blocks a change of the type.
func (writer *scriptWriter) alterSqlserverColumn(source, target TableSnapshot, from, to db.ColumnDetail) {
	table := source.Table
	if from.IsGenerated != to.IsGenerated || from.IsIdentityAlways != to.IsIdentityAlways {
		writer.notes = append(writer.notes, "not written: the identity or generation of "+
			table.Name+"."+to.Name)
	}
	alter := writer.alterTable(table)
	changesType := from.DataType != to.DataType || from.Nullable != to.Nullable
	resetsDefault := from.HasDefault != to.HasDefault || from.DefaultValue != to.DefaultValue ||
		(changesType && from.HasDefault)
	if resetsDefault && from.HasDefault {
		writer.add(phaseAlterColumns, writer.dropSqlserverDefault(table, from.Name))
	}
	if changesType {
		writer.recreateColumnUsers(source, target, to.Name)
		nullability := " NOT NULL;"
		if to.Nullable {
			nullability = " NULL;"
		}
		writer.add(phaseAlterColumns,
			alter+"ALTER COLUMN "+writer.quote(to.Name)+" "+to.DataType+nullability)
	}
	if resetsDefault && to.HasDefault && !to.IsGenerated && !to.IsIdentityAlways {
		writer.add(phaseAlterColumns,
			alter+"ADD DEFAULT "+to.DefaultValue+" FOR "+writer.quote(to.Name)+";")
	}
}

// dropSqlserverDefault returns the statement that drops the default constraint of one column.
// The server names that constraint.
func (writer *scriptWriter) dropSqlserverDefault(table db.TableRef, column string) string {
	literal := func(text string) string { return "N'" + strings.ReplaceAll(text, "'", "''") + "'" }
	return "DECLARE @statement nvarchar(max) = " +
		literal("ALTER TABLE "+writer.quote(table.Name)+" DROP CONSTRAINT ") + " + QUOTENAME((\n" +
		"    SELECT d.name FROM sys.default_constraints d\n" +
		"      JOIN sys.columns c ON c.object_id = d.parent_object_id AND c.column_id = d.parent_column_id\n" +
		"     WHERE d.parent_object_id = OBJECT_ID(" + literal(writer.quote(table.Name)) + ")" +
		" AND c.name = " + literal(column) + "))\n" +
		"EXEC sp_executesql @statement;"
}

// rebuildTables creates each SQLite table with a changed column or constraint again from
// the target definition, and copies the rows of the columns both definitions have. SQLite
// changes no column and no constraint in place.
func (writer *scriptWriter) rebuildTables(found []Difference) error {
	for _, difference := range found {
		if difference.Part != PartColumn && difference.Part != PartConstraint {
			continue
		}
		source, inSource := writer.sources[difference.Table]
		target, inTarget := writer.targets[difference.Table]
		if !inSource || !inTarget || writer.rebuilt[difference.Table] ||
			source.Table.Kind != db.RelationTable || target.Table.Kind != db.RelationTable {
			continue
		}
		writer.rebuilt[difference.Table] = true
		if err := writer.rebuildTable(source, target); err != nil {
			return err
		}
	}
	return nil
}

func (writer *scriptWriter) rebuildTable(source, target TableSnapshot) error {
	created, err := writer.readCreateTable(target)
	if err != nil {
		return err
	}
	name := writer.quote(target.Table.Name)
	copied := writer.quote("masume_new_" + target.Table.Name)
	renamed, err := renameCreatedTable(created, copied, writer.request.Dialect.Syntax)
	if err != nil {
		return err
	}
	writer.add(phaseAlterColumns, renamed)

	columns := []string{}
	for _, column := range target.Columns {
		if !column.IsGenerated && findColumn(source, column.Name).Name != "" {
			columns = append(columns, writer.quote(column.Name))
		}
	}
	if len(columns) > 0 {
		list := strings.Join(columns, ", ")
		writer.add(phaseAlterColumns,
			"INSERT INTO "+copied+" ("+list+") SELECT "+list+" FROM "+name+";")
	}
	writer.add(phaseAlterColumns, "DROP TABLE "+name+";")
	writer.add(phaseAlterColumns, "ALTER TABLE "+copied+" RENAME TO "+name+";")
	writer.createIndexes(target)
	return nil
}

// readCreateTable returns the CREATE TABLE of a SQLite target table, without the CREATE
// INDEX statements the definition also has.
func (writer *scriptWriter) readCreateTable(target TableSnapshot) (string, error) {
	lines, err := writer.readDefinition(target.Table)
	if err != nil {
		return "", err
	}
	text := strings.Join(lines, "\n")
	for _, index := range target.Indexes {
		text = strings.Replace(text, "\n"+index.Definition+";", "", 1)
	}
	return endStatement(text), nil
}

// renameCreatedTable returns the CREATE TABLE with another table name.
func renameCreatedTable(created, name string, flavour syntax.SyntaxFlavour) (string, error) {
	tokens := syntax.ReadCodeTokens(created, flavour)
	at := slices.IndexFunc(tokens, func(token syntax.CodeToken) bool { return token.Text == "table" })
	if at < 0 {
		return "", db.NewDatabaseError("no table name in %q", created)
	}
	at++
	for at < len(tokens) && slices.Contains([]string{"if", "not", "exists"}, tokens[at].Text) {
		at++
	}
	last := at
	if syntax.IsOperator(tokens, at+1, ".") {
		last = at + 2
	}
	if last >= len(tokens) {
		return "", db.NewDatabaseError("no table name in %q", created)
	}
	return created[:tokens[at].Start] + name + created[tokens[last].End:], nil
}

// createIndexes creates the indexes of a target table that belong to no constraint.
func (writer *scriptWriter) createIndexes(target TableSnapshot) {
	for _, index := range listOwnIndexes(target) {
		writer.add(phaseCreateIndexes, endStatement(index.Definition))
	}
}

// recreateColumnUsers drops the indexes and constraints of a source table that use one
// column, and creates the target ones of the same name. SQL Server refuses to change a
// column that an index, a key or a check uses.
func (writer *scriptWriter) recreateColumnUsers(source, target TableSnapshot, column string) {
	reads := regexp.MustCompile(`(?i)(^|[^\w])` + regexp.QuoteMeta(column) + `($|[^\w])`)
	recreates := func(name, definition string) bool {
		key := source.Table.Name + "." + name
		if writer.recreated[key] || !reads.MatchString(definition) {
			return false
		}
		writer.recreated[key] = true
		return true
	}
	for _, index := range listOwnIndexes(source) {
		if !recreates(index.Name, index.Definition) {
			continue
		}
		writer.add(phaseDropIndexes,
			"DROP INDEX "+writer.quote(index.Name)+" ON "+writer.quote(source.Table.Name)+";")
		if created, held := findIndex(target, index.Name); held {
			writer.add(phaseCreateIndexes, endStatement(created.Definition))
		}
	}
	for _, constraint := range source.Constraints {
		if !recreates(constraint.Name, constraint.Definition) {
			continue
		}
		writer.add(phaseDropConstraints, writer.writeDropConstraint(source.Table, constraint))
		if added, held := findConstraint(target, constraint.Name); held {
			phase := phaseAddConstraints
			if added.Kind == db.ConstraintForeignKey {
				phase = phaseAddForeignKeys
			}
			writer.add(phase, writer.writeAddConstraint(target.Table, added))
		}
	}
}
