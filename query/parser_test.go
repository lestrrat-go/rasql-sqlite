package query_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lestrrat-go/rasql-sqlite/query"
)

func TestParseSelect(t *testing.T) {
	t.Parallel()

	parsed, err := query.Parse(`
		SELECT DISTINCT u.id AS user_id, count(*) total, X'abcd' data
		FROM main.users AS u
		WHERE u.active = TRUE AND u.balance >= ?1
		ORDER BY u.id COLLATE nocase DESC, total
		LIMIT ?2 OFFSET 3;
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := []query.Statement{&query.SelectStatement{
		Distinct: true,
		Targets: []query.Target{
			{Expression: identifierExpression("u", "id"), Alias: identifier("user_id")},
			{Expression: &query.CallExpression{Function: qualifiedName("count"), Arguments: []query.Expression{&query.StarExpression{}}}, Alias: identifier("total")},
			{Expression: &query.Literal{Kind: query.BlobLiteral, Value: "ABCD"}, Alias: identifier("data")},
		},
		FromItems: []query.FromItem{{Relation: qualifiedName("main", "users"), Alias: identifier("u")}},
		Where: &query.BinaryExpression{
			Left: &query.BinaryExpression{
				Left:     identifierExpression("u", "active"),
				Operator: "=",
				Right:    &query.Literal{Kind: query.BooleanLiteral, Value: "true"},
			},
			Operator: "AND",
			Right: &query.BinaryExpression{
				Left:     identifierExpression("u", "balance"),
				Operator: ">=",
				Right:    &query.Parameter{Name: "?1"},
			},
		},
		OrderTerms: []query.OrderTerm{
			{Expression: identifierExpression("u", "id"), Collation: identifier("nocase"), Direction: query.SortDescending},
			{Expression: identifierExpression("total")},
		},
		Limit: &query.LimitClause{Count: &query.Parameter{Name: "?2"}, Offset: &query.Literal{Kind: query.NumberLiteral, Value: "3"}},
	}}
	if !reflect.DeepEqual(parsed.Statements, want) {
		t.Fatalf("Parse() = %#v, want %#v", parsed.Statements, want)
	}
}

func TestParseDDL(t *testing.T) {
	t.Parallel()

	statement, err := query.ParseStatement(`
		CREATE TEMP TABLE IF NOT EXISTS main.users (
			id INTEGER PRIMARY KEY,
			email TEXT CONSTRAINT email_required NOT NULL ON CONFLICT ABORT UNIQUE,
			parent_id INTEGER REFERENCES parents (id),
			score NUMERIC(10, 2) DEFAULT (1 + 2) CHECK (score >= 0),
			doubled INTEGER GENERATED ALWAYS AS (score * 2) STORED,
			CONSTRAINT user_email UNIQUE (email COLLATE nocase DESC) ON CONFLICT REPLACE,
			FOREIGN KEY (parent_id) REFERENCES parents (id)
		) WITHOUT ROWID, STRICT
	`)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	want := &query.CreateTableStatement{
		Persistence: query.TemporaryRelation,
		IfNotExists: true,
		Name:        qualifiedName("main", "users"),
		Columns: []query.ColumnDefinition{
			{
				Name:        identifierValue("id"),
				Type:        query.DataType{Words: []string{"integer"}},
				Constraints: []query.ColumnConstraint{{Kind: query.ConstraintPrimaryKey}},
			},
			{
				Name: identifierValue("email"),
				Type: query.DataType{Words: []string{"text"}},
				Constraints: []query.ColumnConstraint{
					{Name: identifier("email_required"), Kind: query.ConstraintNotNull, Conflict: query.ConflictAbort},
					{Kind: query.ConstraintUnique},
				},
			},
			{
				Name: identifierValue("parent_id"),
				Type: query.DataType{Words: []string{"integer"}},
				Constraints: []query.ColumnConstraint{{
					Kind:       query.ConstraintReferences,
					References: &query.Reference{Table: qualifiedName("parents"), Columns: []query.Identifier{identifierValue("id")}},
				}},
			},
			{
				Name: identifierValue("score"),
				Type: query.DataType{Words: []string{"numeric"}, Modifiers: []query.Expression{
					&query.Literal{Kind: query.NumberLiteral, Value: "10"},
					&query.Literal{Kind: query.NumberLiteral, Value: "2"},
				}},
				Constraints: []query.ColumnConstraint{
					{Kind: query.ConstraintDefault, Expression: &query.BinaryExpression{Left: number("1"), Operator: "+", Right: number("2")}},
					{Kind: query.ConstraintCheck, Expression: &query.BinaryExpression{Left: identifierExpression("score"), Operator: ">=", Right: number("0")}},
				},
			},
			{
				Name: identifierValue("doubled"),
				Type: query.DataType{Words: []string{"integer"}},
				Constraints: []query.ColumnConstraint{{
					Kind: query.ConstraintGenerated,
					Generated: &query.GeneratedColumn{
						Expression: &query.BinaryExpression{Left: identifierExpression("score"), Operator: "*", Right: number("2")},
						Storage:    query.GeneratedStored,
					},
				}},
			},
		},
		Constraints: []query.TableConstraint{
			{
				Name:     identifier("user_email"),
				Kind:     query.ConstraintUnique,
				Conflict: query.ConflictReplace,
				Columns: []query.IndexedColumn{{
					Expression: identifierExpression("email"),
					Collation:  identifier("nocase"),
					Direction:  query.SortDescending,
				}},
			},
			{
				Kind:       query.ConstraintForeignKey,
				Columns:    []query.IndexedColumn{{Expression: identifierExpression("parent_id")}},
				References: &query.Reference{Table: qualifiedName("parents"), Columns: []query.Identifier{identifierValue("id")}},
			},
		},
		Options: query.TableOptions{WithoutRowID: true, Strict: true},
	}
	if !reflect.DeepEqual(statement, want) {
		t.Fatalf("ParseStatement() = %#v, want %#v", statement, want)
	}
}

func TestParseSQLiteDDLForms(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  query.Statement
	}{
		"table as select": {
			input: "CREATE TABLE archived AS SELECT id FROM users",
			want: &query.CreateTableStatement{
				Name: qualifiedName("archived"),
				As:   &query.SelectStatement{Targets: []query.Target{{Expression: identifierExpression("id")}}, FromItems: []query.FromItem{{Relation: qualifiedName("users")}}},
			},
		},
		"index": {
			input: "CREATE UNIQUE INDEX IF NOT EXISTS users_email ON users (email COLLATE nocase DESC) WHERE email IS NOT NULL",
			want: &query.CreateIndexStatement{
				Unique:      true,
				IfNotExists: true,
				Name:        qualifiedName("users_email"),
				Table:       qualifiedName("users"),
				Elements:    []query.IndexedColumn{{Expression: identifierExpression("email"), Collation: identifier("nocase"), Direction: query.SortDescending}},
				Where:       &query.BinaryExpression{Left: identifierExpression("email"), Operator: "IS NOT", Right: &query.Literal{Kind: query.NullLiteral}},
			},
		},
		"view": {
			input: "CREATE TEMP VIEW IF NOT EXISTS active_users (id) AS SELECT id FROM users WHERE active = TRUE",
			want: &query.CreateViewStatement{
				Persistence: query.TemporaryRelation,
				IfNotExists: true,
				Name:        qualifiedName("active_users"),
				Columns:     []query.Identifier{identifierValue("id")},
				Query: &query.SelectStatement{
					Targets:   []query.Target{{Expression: identifierExpression("id")}},
					FromItems: []query.FromItem{{Relation: qualifiedName("users")}},
					Where:     &query.BinaryExpression{Left: identifierExpression("active"), Operator: "=", Right: &query.Literal{Kind: query.BooleanLiteral, Value: "true"}},
				},
			},
		},
		"alter add": {
			input: "ALTER TABLE users ADD COLUMN nickname TEXT",
			want: &query.AlterTableStatement{
				Name: qualifiedName("users"),
				Action: query.AlterTableAction{Kind: query.AlterTableAddColumn, Column: &query.ColumnDefinition{
					Name: identifierValue("nickname"), Type: query.DataType{Words: []string{"text"}},
				}},
			},
		},
		"alter rename column": {
			input: "ALTER TABLE users RENAME COLUMN nickname TO display_name",
			want:  &query.AlterTableStatement{Name: qualifiedName("users"), Action: query.AlterTableAction{Kind: query.AlterTableRenameColumn, Name: identifier("nickname"), NewName: identifier("display_name")}},
		},
		"alter rename table": {
			input: "ALTER TABLE users RENAME TO members",
			want:  &query.AlterTableStatement{Name: qualifiedName("users"), Action: query.AlterTableAction{Kind: query.AlterTableRenameTable, NewName: identifier("members")}},
		},
		"alter drop": {
			input: "ALTER TABLE users DROP COLUMN nickname",
			want:  &query.AlterTableStatement{Name: qualifiedName("users"), Action: query.AlterTableAction{Kind: query.AlterTableDropColumn, Name: identifier("nickname")}},
		},
		"drop": {
			input: "DROP TRIGGER IF EXISTS main.users_audit",
			want:  &query.DropStatement{ObjectType: query.DropTrigger, IfExists: true, Name: qualifiedName("main", "users_audit")},
		},
		"single-quoted table name": {
			// SQLite persists this exact form for FTS5's own shadow
			// tables: sqlite_master.sql for an fts5 table's "_data" table
			// reads CREATE TABLE 'posts_fts_data'(...), single-quoting the
			// table name even though single quotes otherwise always
			// delimit a string literal. SQLite's own compatibility
			// fallback reads a string constant as an identifier wherever
			// the grammar allows only a name; this proves rasql-sqlite's
			// parser now does the same in table-name position.
			input: "CREATE TABLE 'posts_fts_data'(id INTEGER PRIMARY KEY, block BLOB)",
			want: &query.CreateTableStatement{
				Name: query.QualifiedName{{Name: "posts_fts_data", Quoted: true}},
				Columns: []query.ColumnDefinition{
					{Name: identifierValue("id"), Type: query.DataType{Words: []string{"integer"}}, Constraints: []query.ColumnConstraint{{Kind: query.ConstraintPrimaryKey}}},
					{Name: identifierValue("block"), Type: query.DataType{Words: []string{"blob"}}},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := query.ParseStatement(test.input)
			if err != nil {
				t.Fatalf("ParseStatement() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ParseStatement() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseSQLiteLexicalForms(t *testing.T) {
	t.Parallel()

	parsed, err := query.ParseStatement("SELECT 0xCAFE AS value, [display name] FROM `users` WHERE value = $value")
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}
	got, err := query.SerializeStatement(parsed)
	if err != nil {
		t.Fatalf("SerializeStatement() error = %v", err)
	}
	want := "SELECT 0xCAFE AS value, \"display name\" FROM \"users\" WHERE value = $value"
	if got != want {
		t.Fatalf("SerializeStatement() = %q, want %q", got, want)
	}
}

func TestParseInExpression(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  query.Expression
	}{
		"value list": {
			input: "SELECT id FROM tasks WHERE status IN ('todo', 'done')",
			want: &query.InExpression{
				Expression: identifierExpression("status"),
				Values:     []query.Expression{text("todo"), text("done")},
			},
		},
		"negated": {
			input: "SELECT id FROM tasks WHERE status NOT IN ('done')",
			want: &query.InExpression{
				Expression: identifierExpression("status"),
				Negated:    true,
				Values:     []query.Expression{text("done")},
			},
		},
		"empty list": {
			input: "SELECT id FROM tasks WHERE status IN ()",
			want: &query.InExpression{
				Expression: identifierExpression("status"),
				Values:     []query.Expression{},
			},
		},
		"binds tighter than AND": {
			input: "SELECT id FROM tasks WHERE done = FALSE AND status IN ('todo')",
			want: &query.BinaryExpression{
				Left: &query.BinaryExpression{
					Left:     identifierExpression("done"),
					Operator: "=",
					Right:    &query.Literal{Kind: query.BooleanLiteral, Value: "false"},
				},
				Operator: "AND",
				Right: &query.InExpression{
					Expression: identifierExpression("status"),
					Values:     []query.Expression{text("todo")},
				},
			},
		},
		"arithmetic operand": {
			input: "SELECT id FROM tasks WHERE priority + 1 IN (2, 3)",
			want: &query.InExpression{
				Expression: &query.BinaryExpression{
					Left:     identifierExpression("priority"),
					Operator: "+",
					Right:    number("1"),
				},
				Values: []query.Expression{number("2"), number("3")},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parsed, err := query.ParseStatement(test.input)
			if err != nil {
				t.Fatalf("ParseStatement() error = %v", err)
			}
			statement, ok := query.As[*query.SelectStatement](parsed)
			if !ok {
				t.Fatalf("ParseStatement() = %T, want *SelectStatement", parsed)
			}
			if !reflect.DeepEqual(statement.Where, test.want) {
				t.Fatalf("Where = %#v, want %#v", statement.Where, test.want)
			}
		})
	}
}

func TestParseInCheckConstraint(t *testing.T) {
	t.Parallel()

	parsed, err := query.ParseStatement(`CREATE TABLE "tasks" ("status" TEXT NOT NULL, CHECK (status IN ('todo', 'in_progress', 'done')))`)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}
	statement, ok := query.As[*query.CreateTableStatement](parsed)
	if !ok {
		t.Fatalf("ParseStatement() = %T, want *CreateTableStatement", parsed)
	}
	if len(statement.Constraints) != 1 {
		t.Fatalf("Constraints length = %d, want 1", len(statement.Constraints))
	}
	constraint := statement.Constraints[0]
	if constraint.Kind != query.ConstraintCheck {
		t.Fatalf("Constraints[0].Kind = %q, want %q", constraint.Kind, query.ConstraintCheck)
	}
	want := &query.InExpression{
		Expression: identifierExpression("status"),
		Values:     []query.Expression{text("todo"), text("in_progress"), text("done")},
	}
	if !reflect.DeepEqual(constraint.Expression, want) {
		t.Fatalf("Constraints[0].Expression = %#v, want %#v", constraint.Expression, want)
	}
}

// TestParseSingleQuotedTableNameKeepsOtherStringLiterals proves that
// accepting a single-quoted table name did not loosen single-quoted string
// literal parsing anywhere else in the same statement: a DEFAULT and a
// CHECK constraint, each spelling a single-quoted value, still parse as
// ordinary string literal expressions rather than as identifiers.
func TestParseSingleQuotedTableNameKeepsOtherStringLiterals(t *testing.T) {
	t.Parallel()

	parsed, err := query.ParseStatement(`CREATE TABLE 'docs_config'(k PRIMARY KEY, status TEXT DEFAULT 'pending' CHECK (status != 'deleted'))`)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}
	statement, ok := query.As[*query.CreateTableStatement](parsed)
	if !ok {
		t.Fatalf("ParseStatement() = %T, want *CreateTableStatement", parsed)
	}

	wantName := query.QualifiedName{{Name: "docs_config", Quoted: true}}
	if !reflect.DeepEqual(statement.Name, wantName) {
		t.Fatalf("Name = %#v, want %#v", statement.Name, wantName)
	}

	status := statement.Columns[1]
	wantConstraints := []query.ColumnConstraint{
		{Kind: query.ConstraintDefault, Expression: text("pending")},
		{Kind: query.ConstraintCheck, Expression: &query.BinaryExpression{Left: identifierExpression("status"), Operator: "!=", Right: text("deleted")}},
	}
	if !reflect.DeepEqual(status.Constraints, wantConstraints) {
		t.Fatalf("Columns[1].Constraints = %#v, want %#v", status.Constraints, wantConstraints)
	}
}

func TestParseFailures(t *testing.T) {
	t.Parallel()

	tests := []string{
		"SELECT",
		"SELECT id FROM users JOIN roles ON roles.user_id = users.id",
		"CREATE TABLE users ()",
		"CREATE INDEX users_email ON users ()",
		"DROP DATABASE app",
		"ALTER TABLE users ALTER COLUMN id TYPE TEXT",
		"SELECT ?0",
		"SELECT id; SELECT name",
		"SELECT id FROM users WHERE status IN",
		"SELECT id FROM users WHERE status IN 'todo'",
		"SELECT id FROM users WHERE status IN ('todo'",
		"SELECT id FROM users WHERE status IN ('todo' 'done')",
		"SELECT id FROM users WHERE status IN (SELECT status FROM archived)",
		"SELECT id FROM users WHERE status NOT IN",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, err := query.ParseStatement(input)
			if err == nil {
				t.Fatal("ParseStatement() error = nil")
			}
			var parseError *query.ParseError
			if !errors.As(err, &parseError) {
				t.Fatalf("ParseStatement() error = %T, want *ParseError", err)
			}
		})
	}
}

func TestAs(t *testing.T) {
	t.Parallel()

	statement, err := query.ParseStatement("SELECT id")
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}
	if _, ok := query.As[*query.SelectStatement](statement); !ok {
		t.Fatal("As[*SelectStatement]() ok = false, want true")
	}
	if _, ok := query.As[*query.CreateTableStatement](statement); ok {
		t.Fatal("As[*CreateTableStatement]() ok = true, want false")
	}
}

func identifier(name string) *query.Identifier {
	value := identifierValue(name)
	return &value
}

func identifierValue(name string) query.Identifier {
	return query.Identifier{Name: name}
}

func qualifiedName(parts ...string) query.QualifiedName {
	name := make(query.QualifiedName, len(parts))
	for index, part := range parts {
		name[index] = identifierValue(part)
	}
	return name
}

func identifierExpression(parts ...string) *query.IdentifierExpression {
	return &query.IdentifierExpression{Name: qualifiedName(parts...)}
}

func number(value string) *query.Literal {
	return &query.Literal{Kind: query.NumberLiteral, Value: value}
}

func text(value string) *query.Literal {
	return &query.Literal{Kind: query.StringLiteral, Value: value}
}
