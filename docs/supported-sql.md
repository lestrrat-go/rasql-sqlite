# Supported SQL

`query.Parse` accepts a focused, editable subset of SQLite 3.53 SQL. It
returns an error for every other grammar branch.

## SELECT

`SELECT` supports `ALL` and `DISTINCT`; expression targets; `AS` and implicit
aliases; simple comma-separated table references; `WHERE`; `ORDER BY` with
`COLLATE`, `ASC`, and `DESC`; and `LIMIT count OFFSET offset` or
`LIMIT offset, count`.

The supported expressions are qualified identifiers, qualified stars, string,
decimal and hexadecimal number, blob, boolean, null, and current-time literals,
parameters, function calls, parentheses, unary `+`, `-`, `~`, and `NOT`, plus binary arithmetic,
comparison, `IS`, `IS NOT`, `LIKE`, `GLOB`, `MATCH`, `AND`, `OR`, and `IN` or
`NOT IN` over a parenthesized value list. An empty value list is accepted and
kept, because SQLite reads `x IN ()` as false and `x NOT IN ()` as true for
every `x`. The subquery and table-name forms on the right of `IN` are reported
as unsupported rather than parsed as a value list.

Joins, common table expressions, compound queries, grouping, windows,
subqueries, `BETWEEN`, `CASE`, and `ESCAPE` are outside the subset.

## DDL

`CREATE TABLE` supports `TEMP` and `TEMPORARY`, `IF NOT EXISTS`, a qualified
name, columns, supported column and table constraints, `WITHOUT ROWID`,
`STRICT`, and `AS SELECT`. Declared type names are optional, as SQLite permits.

The supported column constraints are named constraints, `NOT NULL`, `NULL`,
`DEFAULT`, `PRIMARY KEY`, `UNIQUE`, `CHECK`, `REFERENCES`, `COLLATE`, and
generated columns. The supported table constraints are named `PRIMARY KEY`,
`UNIQUE`, `CHECK`, and `FOREIGN KEY` constraints. Conflict clauses preserve
SQLite's `ROLLBACK`, `ABORT`, `FAIL`, `IGNORE`, and `REPLACE` resolutions.

`REFERENCES` records a table and optional referenced columns. Foreign-key
actions, deferrability, and other reference clauses are outside the subset.

`ALTER TABLE` supports `ADD [COLUMN]`, `DROP [COLUMN]`, `RENAME COLUMN`, and
`RENAME TO`. `CREATE [UNIQUE] INDEX` supports `IF NOT EXISTS`, indexed
expressions, `COLLATE`, `ASC`, `DESC`, and a `WHERE` predicate. `CREATE VIEW`
supports temporary views, `IF NOT EXISTS`, optional column names, and a
supported `SELECT` body. `DROP` supports `TABLE`, `INDEX`, `VIEW`, and
`TRIGGER`, with `IF EXISTS`.

Triggers, virtual tables, pragmas, DML, transaction control, `ATTACH`, and
other SQLite statements are outside the subset.

## Editing

The public AST is mutable. `Statement` and `Expression` are sealed interfaces,
so values created by this module retain a supported concrete shape. Repeated
children use ordinary slices and can be changed directly. `Serialize` validates
manually constructed ASTs before writing SQL.
