// Package query parses and serializes a supported subset of SQLite queries.
package query

import "strings"

// Query is an editable sequence of parsed SQL statements.
type Query struct {
	Statements []Statement
}

// Statement is one SQL statement. The interface is sealed so that each value
// is a pointer to an editable concrete AST node produced by this package.
type Statement interface {
	statement()
}

// As returns statement as T when it has the requested AST type.
func As[T Statement](statement Statement) (T, bool) {
	value, ok := statement.(T)
	return value, ok
}

// DDLStatement is a SQLite definition statement.
type DDLStatement interface {
	Statement
	ddlStatement()
}

// RelationPersistence specifies whether a relation is temporary.
type RelationPersistence string

const (
	// PermanentRelation is SQLite's default relation persistence.
	PermanentRelation RelationPersistence = ""
	// TemporaryRelation creates a relation in SQLite's temporary database.
	TemporaryRelation RelationPersistence = "TEMP"
)

// CreateTableStatement represents a SQLite CREATE TABLE statement.
type CreateTableStatement struct {
	Persistence RelationPersistence
	IfNotExists bool
	Name        QualifiedName
	Columns     []ColumnDefinition
	Constraints []TableConstraint
	Options     TableOptions
	As          *SelectStatement
}

func (*CreateTableStatement) statement()    {}
func (*CreateTableStatement) ddlStatement() {}

// TableOptions controls SQLite table storage and type enforcement.
type TableOptions struct {
	WithoutRowID bool
	Strict       bool
}

// ColumnDefinition is a SQLite table column and its constraints.
type ColumnDefinition struct {
	Name        Identifier
	Type        DataType
	Constraints []ColumnConstraint
}

// DataType is a SQLite declared type. SQLite permits a type name to be empty.
// Words keeps multiword names, while Modifiers keeps any parenthesized values.
type DataType struct {
	Words     []string
	Modifiers []Expression
}

// ColumnConstraint is a supported constraint attached to one column.
type ColumnConstraint struct {
	Name          *Identifier
	Kind          ConstraintKind
	Conflict      ConflictResolution
	Expression    Expression
	References    *Reference
	Collation     *Identifier
	Direction     SortDirection
	Autoincrement bool
	Generated     *GeneratedColumn
}

// GeneratedColumn describes a generated column expression.
type GeneratedColumn struct {
	Expression Expression
	Storage    GeneratedStorage
}

// GeneratedStorage controls whether SQLite stores a generated column.
type GeneratedStorage string

const (
	// GeneratedVirtual computes a generated column when it is read.
	GeneratedVirtual GeneratedStorage = "VIRTUAL"
	// GeneratedStored writes a generated column into the table.
	GeneratedStored GeneratedStorage = "STORED"
)

// TableConstraint is a supported table-level constraint.
type TableConstraint struct {
	Name       *Identifier
	Kind       ConstraintKind
	Columns    []IndexedColumn
	Expression Expression
	References *Reference
	Conflict   ConflictResolution
}

// ConstraintKind identifies a column or table constraint.
type ConstraintKind string

const (
	// ConstraintNotNull requires a column value.
	ConstraintNotNull ConstraintKind = "NOT NULL"
	// ConstraintNull permits a null value explicitly.
	ConstraintNull ConstraintKind = "NULL"
	// ConstraintDefault supplies a default expression.
	ConstraintDefault ConstraintKind = "DEFAULT"
	// ConstraintPrimaryKey identifies primary-key columns.
	ConstraintPrimaryKey ConstraintKind = "PRIMARY KEY"
	// ConstraintUnique requires unique values.
	ConstraintUnique ConstraintKind = "UNIQUE"
	// ConstraintCheck validates an expression.
	ConstraintCheck ConstraintKind = "CHECK"
	// ConstraintReferences creates a column reference.
	ConstraintReferences ConstraintKind = "REFERENCES"
	// ConstraintForeignKey creates a table-level foreign key.
	ConstraintForeignKey ConstraintKind = "FOREIGN KEY"
	// ConstraintCollate selects a collation for one column.
	ConstraintCollate ConstraintKind = "COLLATE"
	// ConstraintGenerated defines a generated column.
	ConstraintGenerated ConstraintKind = "GENERATED"
)

// ConflictResolution selects SQLite's behavior after a constraint violation.
type ConflictResolution string

const (
	// ConflictDefault lets SQLite select the default behavior.
	ConflictDefault ConflictResolution = ""
	// ConflictRollback rolls back the current transaction.
	ConflictRollback ConflictResolution = "ROLLBACK"
	// ConflictAbort aborts the current statement.
	ConflictAbort ConflictResolution = "ABORT"
	// ConflictFail preserves prior statement changes and returns an error.
	ConflictFail ConflictResolution = "FAIL"
	// ConflictIgnore skips the violating row.
	ConflictIgnore ConflictResolution = "IGNORE"
	// ConflictReplace deletes conflicting rows before inserting or updating.
	ConflictReplace ConflictResolution = "REPLACE"
)

// Reference identifies a referenced table and optional columns.
type Reference struct {
	Table   QualifiedName
	Columns []Identifier
}

// IndexedColumn is one table-constraint or index element.
type IndexedColumn struct {
	Expression Expression
	Collation  *Identifier
	Direction  SortDirection
}

// AlterTableStatement represents a supported SQLite ALTER TABLE action.
type AlterTableStatement struct {
	Name   QualifiedName
	Action AlterTableAction
}

func (*AlterTableStatement) statement()    {}
func (*AlterTableStatement) ddlStatement() {}

// AlterTableAction is one SQLite ALTER TABLE action.
type AlterTableAction struct {
	Kind    AlterTableActionKind
	Column  *ColumnDefinition
	Name    *Identifier
	NewName *Identifier
}

// AlterTableActionKind identifies a SQLite ALTER TABLE action.
type AlterTableActionKind string

const (
	// AlterTableAddColumn adds a column definition.
	AlterTableAddColumn AlterTableActionKind = "ADD COLUMN"
	// AlterTableDropColumn drops a column.
	AlterTableDropColumn AlterTableActionKind = "DROP COLUMN"
	// AlterTableRenameColumn renames a column.
	AlterTableRenameColumn AlterTableActionKind = "RENAME COLUMN"
	// AlterTableRenameTable renames a table.
	AlterTableRenameTable AlterTableActionKind = "RENAME TABLE"
)

// DropStatement represents a SQLite DROP TABLE, INDEX, VIEW, or TRIGGER statement.
type DropStatement struct {
	ObjectType DropObjectType
	IfExists   bool
	Name       QualifiedName
}

func (*DropStatement) statement()    {}
func (*DropStatement) ddlStatement() {}

// DropObjectType identifies a SQLite object that can be dropped.
type DropObjectType string

const (
	// DropTable drops a table.
	DropTable DropObjectType = "TABLE"
	// DropIndex drops an index.
	DropIndex DropObjectType = "INDEX"
	// DropView drops a view.
	DropView DropObjectType = "VIEW"
	// DropTrigger drops a trigger.
	DropTrigger DropObjectType = "TRIGGER"
)

// CreateIndexStatement represents a SQLite CREATE INDEX statement.
type CreateIndexStatement struct {
	Unique      bool
	IfNotExists bool
	Name        QualifiedName
	Table       QualifiedName
	Elements    []IndexedColumn
	Where       Expression
}

func (*CreateIndexStatement) statement()    {}
func (*CreateIndexStatement) ddlStatement() {}

// CreateViewStatement represents a SQLite CREATE VIEW statement with a SELECT body.
type CreateViewStatement struct {
	Persistence RelationPersistence
	IfNotExists bool
	Name        QualifiedName
	Columns     []Identifier
	Query       *SelectStatement
}

func (*CreateViewStatement) statement()    {}
func (*CreateViewStatement) ddlStatement() {}

// SelectStatement represents supported SQLite SELECT clauses.
type SelectStatement struct {
	All        bool
	Distinct   bool
	Targets    []Target
	FromItems  []FromItem
	Where      Expression
	OrderTerms []OrderTerm
	Limit      *LimitClause
}

func (*SelectStatement) statement() {}

// Target is one select-list item.
type Target struct {
	Expression Expression
	Alias      *Identifier
}

// FromItem is a relation reference with an optional alias.
type FromItem struct {
	Relation QualifiedName
	Alias    *Identifier
}

// OrderTerm is one ORDER BY item.
type OrderTerm struct {
	Expression Expression
	Collation  *Identifier
	Direction  SortDirection
}

// SortDirection controls an ORDER BY or index item.
type SortDirection string

const (
	// SortDefault leaves ordering to SQLite's default ascending order.
	SortDefault SortDirection = ""
	// SortAscending orders an expression in ascending order.
	SortAscending SortDirection = "ASC"
	// SortDescending orders an expression in descending order.
	SortDescending SortDirection = "DESC"
)

// LimitClause represents SQLite's LIMIT and OFFSET clauses.
type LimitClause struct {
	Count  Expression
	Offset Expression
}

// Identifier is a SQLite identifier. Unquoted identifiers are normalized to lowercase.
type Identifier struct {
	Name   string
	Quoted bool
}

// QualifiedName is an identifier with optional database qualification.
type QualifiedName []Identifier

// String returns the dot-separated identifier names.
func (name QualifiedName) String() string {
	parts := make([]string, len(name))
	for index, part := range name {
		parts[index] = part.Name
	}
	return strings.Join(parts, ".")
}

// Expression is a supported SQL expression. Each concrete value is a pointer
// to an editable AST node.
type Expression interface {
	expression()
}

// IdentifierExpression references a column or other qualified name.
type IdentifierExpression struct {
	Name QualifiedName
}

func (*IdentifierExpression) expression() {}

// StarExpression represents * or a qualified star such as table.*.
type StarExpression struct {
	Qualifier QualifiedName
}

func (*StarExpression) expression() {}

// Literal is a string, number, blob, boolean, or NULL literal.
type Literal struct {
	Kind  LiteralKind
	Value string
}

func (*Literal) expression() {}

// LiteralKind identifies a Literal value.
type LiteralKind string

const (
	// StringLiteral is a single-quoted SQL string literal.
	StringLiteral LiteralKind = "string"
	// NumberLiteral is an integer, decimal, or exponent numeric literal.
	NumberLiteral LiteralKind = "number"
	// BlobLiteral is an SQLite hexadecimal blob literal.
	BlobLiteral LiteralKind = "blob"
	// BooleanLiteral is TRUE or FALSE.
	BooleanLiteral LiteralKind = "boolean"
	// NullLiteral is NULL.
	NullLiteral LiteralKind = "null"
	// CurrentTimeLiteral is CURRENT_TIME, CURRENT_DATE, or CURRENT_TIMESTAMP.
	CurrentTimeLiteral LiteralKind = "current-time"
)

// Parameter is a SQLite parameter such as ?, ?1, :name, @name, or $name.
type Parameter struct {
	Name string
}

func (*Parameter) expression() {}

// UnaryExpression applies an operator to one expression.
type UnaryExpression struct {
	Operator   string
	Expression Expression
}

func (*UnaryExpression) expression() {}

// BinaryExpression applies an operator to two expressions.
type BinaryExpression struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (*BinaryExpression) expression() {}

// CallExpression calls a qualified function name.
type CallExpression struct {
	Function  QualifiedName
	Arguments []Expression
}

func (*CallExpression) expression() {}
