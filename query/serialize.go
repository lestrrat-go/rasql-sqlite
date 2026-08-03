package query

import (
	"fmt"
	"slices"
	"strings"
)

// Serialize serializes a query to canonical SQLite SQL. Each statement ends
// with a semicolon, and statements are separated by a newline.
func Serialize(query *Query) (string, error) {
	if query == nil {
		return "", fmt.Errorf("query: cannot serialize a nil query")
	}

	var result strings.Builder
	for statement := range slices.Values(query.Statements) {
		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		serialized, err := SerializeStatement(statement)
		if err != nil {
			return "", err
		}
		result.WriteString(serialized)
		result.WriteByte(';')
	}
	return result.String(), nil
}

// SerializeStatement serializes one statement to canonical SQLite SQL without
// a trailing semicolon.
func SerializeStatement(statement Statement) (string, error) {
	if statement == nil {
		return "", fmt.Errorf("query: cannot serialize a nil statement")
	}

	serializer := serializer{}
	if err := serializer.writeStatement(statement); err != nil {
		return "", err
	}
	return serializer.String(), nil
}

type serializer struct {
	strings.Builder
}

func (serializer *serializer) writeStatement(statement Statement) error {
	switch statement := statement.(type) {
	case *SelectStatement:
		if statement == nil {
			return fmt.Errorf("query: cannot serialize a nil SELECT statement")
		}
		return serializer.writeSelect(statement)
	case *CreateTableStatement:
		if statement == nil {
			return fmt.Errorf("query: cannot serialize a nil CREATE TABLE statement")
		}
		return serializer.writeCreateTable(statement)
	case *AlterTableStatement:
		if statement == nil {
			return fmt.Errorf("query: cannot serialize a nil ALTER TABLE statement")
		}
		return serializer.writeAlterTable(statement)
	case *DropStatement:
		if statement == nil {
			return fmt.Errorf("query: cannot serialize a nil DROP statement")
		}
		return serializer.writeDrop(statement)
	case *CreateIndexStatement:
		if statement == nil {
			return fmt.Errorf("query: cannot serialize a nil CREATE INDEX statement")
		}
		return serializer.writeCreateIndex(statement)
	case *CreateViewStatement:
		if statement == nil {
			return fmt.Errorf("query: cannot serialize a nil CREATE VIEW statement")
		}
		return serializer.writeCreateView(statement)
	default:
		return fmt.Errorf("query: cannot serialize unsupported statement %T", statement)
	}
}

func (serializer *serializer) writeSelect(statement *SelectStatement) error {
	if statement.All && statement.Distinct {
		return fmt.Errorf("query: SELECT cannot be both ALL and DISTINCT")
	}
	if len(statement.Targets) == 0 {
		return fmt.Errorf("query: SELECT requires at least one target")
	}
	serializer.WriteString("SELECT")
	if statement.All {
		serializer.WriteString(" ALL")
	}
	if statement.Distinct {
		serializer.WriteString(" DISTINCT")
	}
	serializer.WriteByte(' ')
	if err := writeSeparated(serializer, statement.Targets, ", ", serializer.writeTarget); err != nil {
		return err
	}
	if len(statement.FromItems) > 0 {
		serializer.WriteString(" FROM ")
		if err := writeSeparated(serializer, statement.FromItems, ", ", serializer.writeFromItem); err != nil {
			return err
		}
	}
	if statement.Where != nil {
		serializer.WriteString(" WHERE ")
		if err := serializer.writeExpression(statement.Where, 0); err != nil {
			return fmt.Errorf("query: serialize WHERE expression: %w", err)
		}
	}
	if len(statement.OrderTerms) > 0 {
		serializer.WriteString(" ORDER BY ")
		if err := writeSeparated(serializer, statement.OrderTerms, ", ", serializer.writeOrderTerm); err != nil {
			return err
		}
	}
	if statement.Limit != nil {
		if statement.Limit.Count == nil {
			return fmt.Errorf("query: LIMIT requires a count")
		}
		serializer.WriteString(" LIMIT ")
		if err := serializer.writeExpression(statement.Limit.Count, 0); err != nil {
			return fmt.Errorf("query: serialize LIMIT count: %w", err)
		}
		if statement.Limit.Offset != nil {
			serializer.WriteString(" OFFSET ")
			if err := serializer.writeExpression(statement.Limit.Offset, 0); err != nil {
				return fmt.Errorf("query: serialize LIMIT offset: %w", err)
			}
		}
	}
	return nil
}

func (serializer *serializer) writeTarget(target Target) error {
	if err := serializer.writeExpression(target.Expression, 0); err != nil {
		return fmt.Errorf("query: serialize SELECT target: %w", err)
	}
	if target.Alias != nil {
		serializer.WriteString(" AS ")
		return serializer.writeIdentifier(*target.Alias)
	}
	return nil
}

func (serializer *serializer) writeFromItem(item FromItem) error {
	if err := serializer.writeQualifiedName(item.Relation); err != nil {
		return fmt.Errorf("query: serialize FROM relation: %w", err)
	}
	if item.Alias != nil {
		serializer.WriteString(" AS ")
		return serializer.writeIdentifier(*item.Alias)
	}
	return nil
}

func (serializer *serializer) writeOrderTerm(term OrderTerm) error {
	if err := serializer.writeExpression(term.Expression, 0); err != nil {
		return fmt.Errorf("query: serialize ORDER BY expression: %w", err)
	}
	if term.Collation != nil {
		serializer.WriteString(" COLLATE ")
		if err := serializer.writeIdentifier(*term.Collation); err != nil {
			return err
		}
	}
	return serializer.writeSortDirection(term.Direction)
}

func (serializer *serializer) writeCreateTable(statement *CreateTableStatement) error {
	if statement.As != nil {
		if len(statement.Columns) != 0 || len(statement.Constraints) != 0 || statement.Options != (TableOptions{}) {
			return fmt.Errorf("query: CREATE TABLE AS cannot have columns, constraints, or options")
		}
		serializer.WriteString("CREATE")
		if err := serializer.writePersistence(statement.Persistence); err != nil {
			return err
		}
		serializer.WriteString(" TABLE")
		if statement.IfNotExists {
			serializer.WriteString(" IF NOT EXISTS")
		}
		serializer.WriteByte(' ')
		if err := serializer.writeQualifiedName(statement.Name); err != nil {
			return fmt.Errorf("query: serialize CREATE TABLE name: %w", err)
		}
		serializer.WriteString(" AS ")
		return serializer.writeSelect(statement.As)
	}
	if len(statement.Columns) == 0 && len(statement.Constraints) == 0 {
		return fmt.Errorf("query: CREATE TABLE requires a column or constraint")
	}
	serializer.WriteString("CREATE")
	if err := serializer.writePersistence(statement.Persistence); err != nil {
		return err
	}
	serializer.WriteString(" TABLE")
	if statement.IfNotExists {
		serializer.WriteString(" IF NOT EXISTS")
	}
	serializer.WriteByte(' ')
	if err := serializer.writeQualifiedName(statement.Name); err != nil {
		return fmt.Errorf("query: serialize CREATE TABLE name: %w", err)
	}
	serializer.WriteString(" (")
	first := true
	for column := range slices.Values(statement.Columns) {
		if !first {
			serializer.WriteString(", ")
		}
		first = false
		if err := serializer.writeColumnDefinition(column); err != nil {
			return err
		}
	}
	for constraint := range slices.Values(statement.Constraints) {
		if !first {
			serializer.WriteString(", ")
		}
		first = false
		if err := serializer.writeTableConstraint(constraint); err != nil {
			return err
		}
	}
	serializer.WriteByte(')')
	if statement.Options.WithoutRowID {
		serializer.WriteString(" WITHOUT ROWID")
	}
	if statement.Options.Strict {
		if statement.Options.WithoutRowID {
			serializer.WriteByte(',')
		}
		serializer.WriteString(" STRICT")
	}
	return nil
}

func (serializer *serializer) writePersistence(persistence RelationPersistence) error {
	switch persistence {
	case PermanentRelation:
		return nil
	case TemporaryRelation:
		serializer.WriteByte(' ')
		serializer.WriteString(string(persistence))
		return nil
	default:
		return fmt.Errorf("query: unsupported relation persistence %q", persistence)
	}
}

func (serializer *serializer) writeColumnDefinition(column ColumnDefinition) error {
	if err := serializer.writeIdentifier(column.Name); err != nil {
		return fmt.Errorf("query: serialize column name: %w", err)
	}
	if len(column.Type.Words) > 0 {
		serializer.WriteByte(' ')
		if err := serializer.writeDataType(column.Type); err != nil {
			return err
		}
	}
	for constraint := range slices.Values(column.Constraints) {
		serializer.WriteByte(' ')
		if err := serializer.writeColumnConstraint(constraint); err != nil {
			return err
		}
	}
	return nil
}

func (serializer *serializer) writeDataType(dataType DataType) error {
	if len(dataType.Words) == 0 {
		if len(dataType.Modifiers) > 0 {
			return fmt.Errorf("query: data type modifiers require a type name")
		}
		return nil
	}
	for word := range slices.Values(dataType.Words) {
		if err := validateDataTypeWord(word); err != nil {
			return err
		}
	}
	serializer.WriteString(strings.Join(dataType.Words, " "))
	if len(dataType.Modifiers) > 0 {
		serializer.WriteByte('(')
		if err := writeSeparated(serializer, dataType.Modifiers, ", ", func(expression Expression) error {
			return serializer.writeExpression(expression, 0)
		}); err != nil {
			return fmt.Errorf("query: serialize type modifier: %w", err)
		}
		serializer.WriteByte(')')
	}
	return nil
}

func (serializer *serializer) writeColumnConstraint(constraint ColumnConstraint) error {
	if err := serializer.writeConstraintName(constraint.Name); err != nil {
		return err
	}
	switch constraint.Kind {
	case ConstraintNotNull:
		serializer.WriteString("NOT NULL")
		return serializer.writeConflictResolution(constraint.Conflict)
	case ConstraintNull:
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: NULL cannot have a conflict resolution")
		}
		serializer.WriteString("NULL")
	case ConstraintDefault:
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: DEFAULT cannot have a conflict resolution")
		}
		serializer.WriteString("DEFAULT ")
		if err := serializer.writeExpression(constraint.Expression, 0); err != nil {
			return fmt.Errorf("query: serialize DEFAULT expression: %w", err)
		}
	case ConstraintPrimaryKey:
		serializer.WriteString("PRIMARY KEY")
		if err := serializer.writeSortDirection(constraint.Direction); err != nil {
			return err
		}
		if err := serializer.writeConflictResolution(constraint.Conflict); err != nil {
			return err
		}
		if constraint.Autoincrement {
			serializer.WriteString(" AUTOINCREMENT")
		}
	case ConstraintUnique:
		serializer.WriteString("UNIQUE")
		return serializer.writeConflictResolution(constraint.Conflict)
	case ConstraintCheck:
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: CHECK cannot have a conflict resolution")
		}
		serializer.WriteString("CHECK (")
		if err := serializer.writeExpression(constraint.Expression, 0); err != nil {
			return fmt.Errorf("query: serialize CHECK expression: %w", err)
		}
		serializer.WriteByte(')')
	case ConstraintReferences:
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: REFERENCES cannot have a conflict resolution")
		}
		serializer.WriteString("REFERENCES ")
		if err := serializer.writeReference(constraint.References); err != nil {
			return err
		}
	case ConstraintCollate:
		if constraint.Collation == nil {
			return fmt.Errorf("query: COLLATE requires a collation name")
		}
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: COLLATE cannot have a conflict resolution")
		}
		serializer.WriteString("COLLATE ")
		if err := serializer.writeIdentifier(*constraint.Collation); err != nil {
			return err
		}
	case ConstraintGenerated:
		if constraint.Generated == nil {
			return fmt.Errorf("query: GENERATED requires an expression")
		}
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: GENERATED cannot have a conflict resolution")
		}
		serializer.WriteString("GENERATED ALWAYS AS (")
		if err := serializer.writeExpression(constraint.Generated.Expression, 0); err != nil {
			return fmt.Errorf("query: serialize generated expression: %w", err)
		}
		serializer.WriteByte(')')
		switch constraint.Generated.Storage {
		case GeneratedVirtual:
			serializer.WriteString(" VIRTUAL")
		case GeneratedStored:
			serializer.WriteString(" STORED")
		case "":
		default:
			return fmt.Errorf("query: unsupported generated column storage %q", constraint.Generated.Storage)
		}
	default:
		return fmt.Errorf("query: unsupported column constraint %q", constraint.Kind)
	}
	return nil
}

func (serializer *serializer) writeTableConstraint(constraint TableConstraint) error {
	if err := serializer.writeConstraintName(constraint.Name); err != nil {
		return err
	}
	switch constraint.Kind {
	case ConstraintPrimaryKey, ConstraintUnique:
		serializer.WriteString(string(constraint.Kind))
		serializer.WriteByte(' ')
		if err := serializer.writeIndexedColumns(constraint.Columns); err != nil {
			return err
		}
		return serializer.writeConflictResolution(constraint.Conflict)
	case ConstraintCheck:
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: CHECK cannot have a conflict resolution")
		}
		serializer.WriteString("CHECK (")
		if err := serializer.writeExpression(constraint.Expression, 0); err != nil {
			return fmt.Errorf("query: serialize CHECK expression: %w", err)
		}
		serializer.WriteByte(')')
	case ConstraintForeignKey:
		if constraint.Conflict != ConflictDefault {
			return fmt.Errorf("query: FOREIGN KEY cannot have a conflict resolution")
		}
		serializer.WriteString("FOREIGN KEY ")
		if err := serializer.writeIndexedColumns(constraint.Columns); err != nil {
			return err
		}
		serializer.WriteString(" REFERENCES ")
		if err := serializer.writeReference(constraint.References); err != nil {
			return err
		}
	default:
		return fmt.Errorf("query: unsupported table constraint %q", constraint.Kind)
	}
	return nil
}

func (serializer *serializer) writeConstraintName(name *Identifier) error {
	if name == nil {
		return nil
	}
	serializer.WriteString("CONSTRAINT ")
	if err := serializer.writeIdentifier(*name); err != nil {
		return err
	}
	serializer.WriteByte(' ')
	return nil
}

func (serializer *serializer) writeReference(reference *Reference) error {
	if reference == nil {
		return fmt.Errorf("query: REFERENCES requires a referenced table")
	}
	if err := serializer.writeQualifiedName(reference.Table); err != nil {
		return err
	}
	if len(reference.Columns) > 0 {
		serializer.WriteString(" (")
		if err := serializer.writeIdentifiers(reference.Columns); err != nil {
			return err
		}
		serializer.WriteByte(')')
	}
	return nil
}

func (serializer *serializer) writeIdentifiers(identifiers []Identifier) error {
	if len(identifiers) == 0 {
		return fmt.Errorf("query: expected at least one identifier")
	}
	return writeSeparated(serializer, identifiers, ", ", serializer.writeIdentifier)
}

func (serializer *serializer) writeIndexedColumns(columns []IndexedColumn) error {
	if len(columns) == 0 {
		return fmt.Errorf("query: expected at least one indexed column")
	}
	serializer.WriteByte('(')
	if err := writeSeparated(serializer, columns, ", ", serializer.writeIndexedColumn); err != nil {
		return err
	}
	serializer.WriteByte(')')
	return nil
}

func (serializer *serializer) writeIndexedColumn(column IndexedColumn) error {
	if err := serializer.writeExpression(column.Expression, 0); err != nil {
		return fmt.Errorf("query: serialize indexed column: %w", err)
	}
	if column.Collation != nil {
		serializer.WriteString(" COLLATE ")
		if err := serializer.writeIdentifier(*column.Collation); err != nil {
			return err
		}
	}
	return serializer.writeSortDirection(column.Direction)
}

func (serializer *serializer) writeConflictResolution(resolution ConflictResolution) error {
	switch resolution {
	case ConflictDefault:
		return nil
	case ConflictRollback, ConflictAbort, ConflictFail, ConflictIgnore, ConflictReplace:
		serializer.WriteString(" ON CONFLICT ")
		serializer.WriteString(string(resolution))
		return nil
	default:
		return fmt.Errorf("query: unsupported conflict resolution %q", resolution)
	}
}

func (serializer *serializer) writeSortDirection(direction SortDirection) error {
	switch direction {
	case SortDefault:
		return nil
	case SortAscending:
		serializer.WriteString(" ASC")
		return nil
	case SortDescending:
		serializer.WriteString(" DESC")
		return nil
	default:
		return fmt.Errorf("query: unsupported sort direction %q", direction)
	}
}

func (serializer *serializer) writeAlterTable(statement *AlterTableStatement) error {
	serializer.WriteString("ALTER TABLE ")
	if err := serializer.writeQualifiedName(statement.Name); err != nil {
		return fmt.Errorf("query: serialize ALTER TABLE name: %w", err)
	}
	serializer.WriteByte(' ')
	return serializer.writeAlterTableAction(statement.Action)
}

func (serializer *serializer) writeAlterTableAction(action AlterTableAction) error {
	switch action.Kind {
	case AlterTableAddColumn:
		if action.Column == nil {
			return fmt.Errorf("query: ADD COLUMN requires a column definition")
		}
		serializer.WriteString("ADD COLUMN ")
		return serializer.writeColumnDefinition(*action.Column)
	case AlterTableDropColumn:
		if action.Name == nil {
			return fmt.Errorf("query: DROP COLUMN requires a column name")
		}
		serializer.WriteString("DROP COLUMN ")
		return serializer.writeIdentifier(*action.Name)
	case AlterTableRenameColumn:
		if action.Name == nil || action.NewName == nil {
			return fmt.Errorf("query: RENAME COLUMN requires current and new names")
		}
		serializer.WriteString("RENAME COLUMN ")
		if err := serializer.writeIdentifier(*action.Name); err != nil {
			return err
		}
		serializer.WriteString(" TO ")
		return serializer.writeIdentifier(*action.NewName)
	case AlterTableRenameTable:
		if action.NewName == nil {
			return fmt.Errorf("query: RENAME TABLE requires a new name")
		}
		serializer.WriteString("RENAME TO ")
		return serializer.writeIdentifier(*action.NewName)
	default:
		return fmt.Errorf("query: unsupported ALTER TABLE action %q", action.Kind)
	}
}

func (serializer *serializer) writeDrop(statement *DropStatement) error {
	serializer.WriteString("DROP ")
	switch statement.ObjectType {
	case DropTable, DropIndex, DropView, DropTrigger:
		serializer.WriteString(string(statement.ObjectType))
	default:
		return fmt.Errorf("query: unsupported DROP object type %q", statement.ObjectType)
	}
	if statement.IfExists {
		serializer.WriteString(" IF EXISTS")
	}
	serializer.WriteByte(' ')
	return serializer.writeQualifiedName(statement.Name)
}

func (serializer *serializer) writeCreateIndex(statement *CreateIndexStatement) error {
	if len(statement.Elements) == 0 {
		return fmt.Errorf("query: CREATE INDEX requires at least one element")
	}
	serializer.WriteString("CREATE")
	if statement.Unique {
		serializer.WriteString(" UNIQUE")
	}
	serializer.WriteString(" INDEX")
	if statement.IfNotExists {
		serializer.WriteString(" IF NOT EXISTS")
	}
	serializer.WriteByte(' ')
	if err := serializer.writeQualifiedName(statement.Name); err != nil {
		return fmt.Errorf("query: serialize index name: %w", err)
	}
	serializer.WriteString(" ON ")
	if err := serializer.writeQualifiedName(statement.Table); err != nil {
		return fmt.Errorf("query: serialize index table: %w", err)
	}
	serializer.WriteByte(' ')
	if err := serializer.writeIndexedColumns(statement.Elements); err != nil {
		return err
	}
	if statement.Where != nil {
		serializer.WriteString(" WHERE ")
		if err := serializer.writeExpression(statement.Where, 0); err != nil {
			return fmt.Errorf("query: serialize index predicate: %w", err)
		}
	}
	return nil
}

func (serializer *serializer) writeCreateView(statement *CreateViewStatement) error {
	if statement.Query == nil {
		return fmt.Errorf("query: CREATE VIEW requires a SELECT query")
	}
	serializer.WriteString("CREATE")
	if err := serializer.writePersistence(statement.Persistence); err != nil {
		return err
	}
	serializer.WriteString(" VIEW")
	if statement.IfNotExists {
		serializer.WriteString(" IF NOT EXISTS")
	}
	serializer.WriteByte(' ')
	if err := serializer.writeQualifiedName(statement.Name); err != nil {
		return fmt.Errorf("query: serialize view name: %w", err)
	}
	if len(statement.Columns) > 0 {
		serializer.WriteString(" (")
		if err := serializer.writeIdentifiers(statement.Columns); err != nil {
			return err
		}
		serializer.WriteByte(')')
	}
	serializer.WriteString(" AS ")
	return serializer.writeSelect(statement.Query)
}

func (serializer *serializer) writeIdentifier(identifier Identifier) error {
	if identifier.Name == "" {
		return fmt.Errorf("query: identifier cannot be empty")
	}
	serialized := serializeIdentifier(identifier)
	tokens, err := lex(serialized)
	if err != nil || len(tokens) != 2 || tokens[0].kind != tokenIdentifier || tokens[0].text != identifier.Name || tokens[0].quoted != identifier.Quoted {
		return fmt.Errorf("query: invalid identifier %q", identifier.Name)
	}
	serializer.WriteString(serialized)
	return nil
}

func (serializer *serializer) writeQualifiedName(name QualifiedName) error {
	if len(name) == 0 {
		return fmt.Errorf("query: qualified name cannot be empty")
	}
	return writeSeparated(serializer, name, ".", serializer.writeIdentifier)
}

func (serializer *serializer) writeExpression(expression Expression, parentPrecedence int) error {
	if expression == nil {
		return fmt.Errorf("query: expression cannot be nil")
	}

	precedence, err := expressionPrecedence(expression)
	if err != nil {
		return err
	}
	parenthesize := precedence < parentPrecedence
	if parenthesize {
		serializer.WriteByte('(')
	}

	switch expression := expression.(type) {
	case *IdentifierExpression:
		if expression == nil {
			return fmt.Errorf("query: expression cannot be a nil identifier")
		}
		err = serializer.writeQualifiedName(expression.Name)
	case *StarExpression:
		if expression == nil {
			return fmt.Errorf("query: expression cannot be a nil star")
		}
		if len(expression.Qualifier) > 0 {
			err = serializer.writeQualifiedName(expression.Qualifier)
			if err == nil {
				serializer.WriteByte('.')
			}
		}
		if err == nil {
			serializer.WriteByte('*')
		}
	case *Literal:
		if expression == nil {
			return fmt.Errorf("query: expression cannot be a nil literal")
		}
		err = serializer.writeLiteral(expression)
	case *Parameter:
		if expression == nil {
			return fmt.Errorf("query: expression cannot be a nil parameter")
		}
		err = serializer.writeParameter(expression.Name)
	case *UnaryExpression:
		if expression == nil {
			return fmt.Errorf("query: expression cannot be a nil unary expression")
		}
		err = serializer.writeUnaryExpression(expression, precedence)
	case *BinaryExpression:
		if expression == nil {
			return fmt.Errorf("query: expression cannot be a nil binary expression")
		}
		err = serializer.writeBinaryExpression(expression, precedence)
	case *CallExpression:
		if expression == nil {
			return fmt.Errorf("query: expression cannot be a nil call")
		}
		err = serializer.writeCallExpression(expression)
	default:
		err = fmt.Errorf("query: cannot serialize unsupported expression %T", expression)
	}
	if err != nil {
		return err
	}
	if parenthesize {
		serializer.WriteByte(')')
	}
	return nil
}

func (serializer *serializer) writeLiteral(literal *Literal) error {
	switch literal.Kind {
	case StringLiteral:
		serializer.WriteByte('\'')
		serializer.WriteString(strings.ReplaceAll(literal.Value, "'", "''"))
		serializer.WriteByte('\'')
	case NumberLiteral:
		if !isNumberLiteral(literal.Value) {
			return fmt.Errorf("query: invalid number literal %q", literal.Value)
		}
		serializer.WriteString(literal.Value)
	case BlobLiteral:
		if !isBlobLiteral(literal.Value) {
			return fmt.Errorf("query: invalid blob literal %q", literal.Value)
		}
		serializer.WriteString("X'")
		serializer.WriteString(strings.ToUpper(literal.Value))
		serializer.WriteByte('\'')
	case BooleanLiteral:
		switch strings.ToLower(literal.Value) {
		case "true":
			serializer.WriteString("TRUE")
		case "false":
			serializer.WriteString("FALSE")
		default:
			return fmt.Errorf("query: boolean literal must be true or false")
		}
	case NullLiteral:
		if literal.Value != "" {
			return fmt.Errorf("query: NULL literal cannot have a value")
		}
		serializer.WriteString("NULL")
	case CurrentTimeLiteral:
		switch strings.ToLower(literal.Value) {
		case "current_date":
			serializer.WriteString("CURRENT_DATE")
		case "current_time":
			serializer.WriteString("CURRENT_TIME")
		case "current_timestamp":
			serializer.WriteString("CURRENT_TIMESTAMP")
		default:
			return fmt.Errorf("query: invalid current-time literal %q", literal.Value)
		}
	default:
		return fmt.Errorf("query: unsupported literal kind %q", literal.Kind)
	}
	return nil
}

func (serializer *serializer) writeParameter(name string) error {
	if !isValidParameter(name) {
		return fmt.Errorf("query: invalid parameter %q", name)
	}
	serializer.WriteString(name)
	return nil
}

func serializeIdentifier(identifier Identifier) string {
	if !identifier.Quoted {
		return identifier.Name
	}
	return "\"" + strings.ReplaceAll(identifier.Name, "\"", "\"\"") + "\""
}

func validateDataTypeWord(word string) error {
	tokens, err := lex(word)
	if err != nil || len(tokens) != 2 || tokens[0].kind != tokenIdentifier || tokens[0].text != word || tokens[0].quoted {
		return fmt.Errorf("query: invalid data type word %q", word)
	}
	return nil
}

func isNumberLiteral(value string) bool {
	tokens, err := lex(value)
	return err == nil && len(tokens) == 2 && tokens[0].kind == tokenNumber && tokens[0].text == value
}

func isBlobLiteral(value string) bool {
	if len(value)%2 != 0 {
		return false
	}
	for _, runeValue := range value {
		if !isHexDigit(runeValue) {
			return false
		}
	}
	return true
}

func (serializer *serializer) writeUnaryExpression(expression *UnaryExpression, precedence int) error {
	switch expression.Operator {
	case "NOT":
		serializer.WriteString("NOT ")
	case "+", "-", "~":
		serializer.WriteString(expression.Operator)
		if child, ok := expression.Expression.(*UnaryExpression); ok && child != nil {
			serializer.WriteByte(' ')
		}
	default:
		return fmt.Errorf("query: unsupported unary operator %q", expression.Operator)
	}
	return serializer.writeExpression(expression.Expression, precedence)
}

func (serializer *serializer) writeBinaryExpression(expression *BinaryExpression, precedence int) error {
	if err := serializer.writeExpression(expression.Left, precedence); err != nil {
		return err
	}
	serializer.WriteByte(' ')
	serializer.WriteString(expression.Operator)
	serializer.WriteByte(' ')
	return serializer.writeExpression(expression.Right, precedence+1)
}

func (serializer *serializer) writeCallExpression(expression *CallExpression) error {
	if err := serializer.writeQualifiedName(expression.Function); err != nil {
		return fmt.Errorf("query: serialize function name: %w", err)
	}
	serializer.WriteByte('(')
	if err := writeSeparated(serializer, expression.Arguments, ", ", func(argument Expression) error {
		return serializer.writeExpression(argument, 0)
	}); err != nil {
		return fmt.Errorf("query: serialize function argument: %w", err)
	}
	serializer.WriteByte(')')
	return nil
}

func expressionPrecedence(expression Expression) (int, error) {
	switch expression := expression.(type) {
	case *IdentifierExpression, *StarExpression, *Literal, *Parameter, *CallExpression:
		return 8, nil
	case *UnaryExpression:
		if expression == nil {
			return 0, fmt.Errorf("query: expression cannot be a nil unary expression")
		}
		switch expression.Operator {
		case "NOT", "+", "-", "~":
			return 7, nil
		default:
			return 0, fmt.Errorf("query: unsupported unary operator %q", expression.Operator)
		}
	case *BinaryExpression:
		if expression == nil {
			return 0, fmt.Errorf("query: expression cannot be a nil binary expression")
		}
		return binaryPrecedence(expression.Operator)
	default:
		return 0, fmt.Errorf("query: cannot serialize unsupported expression %T", expression)
	}
}

func binaryPrecedence(operator string) (int, error) {
	switch operator {
	case "OR":
		return 1, nil
	case "AND":
		return 2, nil
	case "=", "==", "!=", "<>", "<", "<=", ">", ">=", "IS", "IS NOT", "LIKE", "GLOB", "MATCH":
		return 3, nil
	case "||":
		return 4, nil
	case "+", "-":
		return 5, nil
	case "*", "/", "%":
		return 6, nil
	default:
		return 0, fmt.Errorf("query: unsupported binary operator %q", operator)
	}
}

func writeSeparated[T any](serializer *serializer, values []T, separator string, write func(T) error) error {
	first := true
	for value := range slices.Values(values) {
		if !first {
			serializer.WriteString(separator)
		}
		first = false
		if err := write(value); err != nil {
			return err
		}
	}
	return nil
}
