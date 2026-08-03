package query_test

import (
	"strings"
	"testing"

	"github.com/lestrrat-go/rasql-sqlite/query"
)

func TestSerializeRoundTrip(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  string
	}{
		"select": {
			input: "select all a + b * c result, x'affe' from main.items where a like :pattern or b is not null order by result collate nocase desc limit 5, ?2",
			want:  "SELECT ALL a + b * c AS result, X'AFFE' FROM main.items WHERE a LIKE :pattern OR b IS NOT NULL ORDER BY result COLLATE nocase DESC LIMIT ?2 OFFSET 5;",
		},
		"create table": {
			input: "create temporary table if not exists users (id integer primary key autoincrement, email text not null on conflict abort unique on conflict ignore, score numeric(10,2) default (1 + 2) check (score >= 0), parent_id integer references parents(id), constraint email_unique unique(email collate nocase desc) on conflict replace) without rowid, strict",
			want:  "CREATE TEMP TABLE IF NOT EXISTS users (id integer PRIMARY KEY AUTOINCREMENT, email text NOT NULL ON CONFLICT ABORT UNIQUE ON CONFLICT IGNORE, score numeric(10, 2) DEFAULT 1 + 2 CHECK (score >= 0), parent_id integer REFERENCES parents (id), CONSTRAINT email_unique UNIQUE (email COLLATE nocase DESC) ON CONFLICT REPLACE) WITHOUT ROWID, STRICT;",
		},
		"create table as": {
			input: "create table archived as select id from users where id >= ?1",
			want:  "CREATE TABLE archived AS SELECT id FROM users WHERE id >= ?1;",
		},
		"index": {
			input: "create unique index if not exists users_email on users(email collate nocase desc) where email is not null",
			want:  "CREATE UNIQUE INDEX IF NOT EXISTS users_email ON users (email COLLATE nocase DESC) WHERE email IS NOT NULL;",
		},
		"view": {
			input: "create temp view if not exists active_users(id) as select id from users where active = true",
			want:  "CREATE TEMP VIEW IF NOT EXISTS active_users (id) AS SELECT id FROM users WHERE active = TRUE;",
		},
		"alter": {
			input: "alter table users rename column email to address",
			want:  "ALTER TABLE users RENAME COLUMN email TO address;",
		},
		"drop": {
			input: "drop view if exists main.active_users",
			want:  "DROP VIEW IF EXISTS main.active_users;",
		},
		"multiple statements": {
			input: "select id from users; drop index if exists users_email",
			want:  "SELECT id FROM users;\nDROP INDEX IF EXISTS users_email;",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parsed, err := query.Parse(test.input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got, err := query.Serialize(parsed)
			if err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("Serialize() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSerializeRejectsInvalidAST(t *testing.T) {
	t.Parallel()

	tests := map[string]query.Statement{
		"empty select": &query.SelectStatement{},
		"both select modifiers": &query.SelectStatement{
			All: true, Distinct: true, Targets: []query.Target{{Expression: identifierExpression("id")}},
		},
		"invalid parameter": &query.SelectStatement{
			Targets: []query.Target{{Expression: &query.Parameter{Name: "?0"}}},
		},
		"missing table name": &query.CreateTableStatement{
			Columns: []query.ColumnDefinition{{Name: identifierValue("id")}},
		},
		"invalid conflict": &query.CreateTableStatement{
			Name: qualifiedName("users"),
			Columns: []query.ColumnDefinition{{
				Name:        identifierValue("id"),
				Constraints: []query.ColumnConstraint{{Kind: query.ConstraintNotNull, Conflict: "skip"}},
			}},
		},
	}

	for name, statement := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := query.SerializeStatement(statement)
			if err == nil {
				t.Fatal("SerializeStatement() error = nil")
			}
		})
	}
}

func TestSerializeNil(t *testing.T) {
	t.Parallel()

	_, err := query.Serialize(nil)
	if err == nil || !strings.Contains(err.Error(), "nil query") {
		t.Fatalf("Serialize(nil) error = %v, want nil-query error", err)
	}
}
