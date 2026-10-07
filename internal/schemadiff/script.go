package schemadiff

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
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
	if request.Family != core.FamilyPostgres && request.Family != core.FamilyMysql {
		return Script{}, db.NewDatabaseError(
			"an ALTER script is written for the PostgreSQL and MySQL families only")
	}
	writer := scriptWriter{
		ctx: ctx, request: request, definitions: map[string][]string{},
		sources: indexTables(request.Source), targets: indexTables(request.Target),
	}
	found := Compare(request.Source, request.Target)
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
		writer.add(phaseDropColumns, alter+"DROP COLUMN "+name+";")
		return nil
	case ChangeAdded:
		definition, err := writer.writeColumnDefinition(target, difference.Name)
		if err != nil {
			return err
		}
		writer.add(phaseAddColumns, alter+"ADD COLUMN "+definition+";")
		return nil
	}

	from, to := findColumn(source, difference.Name), findColumn(target, difference.Name)
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
	if difference.Change != ChangeAdded {
		if index, held := findIndex(source, difference.Name); held &&
			!index.IsPrimary && !holdsConstraint(source, index.Name) {
			drop := "DROP INDEX " + writer.quote(index.Name)
			if writer.isMysql() {
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
