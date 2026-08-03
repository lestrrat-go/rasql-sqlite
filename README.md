# rasql-sqlite

rasql-sqlite implements SQLite-specific query parsing for rasql. It provides
an editable, pure-Go AST for the supported SQLite language subset on Go 1.26+
without importing rasql itself.

The `query` package targets the SQLite 3.53 language reference. `Parse`
returns a `Query` with concrete editable values in `Query.Statements`.
`ParseStatement` requires one statement. `Serialize` writes canonical SQL with
semicolons, while `SerializeStatement` writes one statement without a trailing
semicolon.

The executable [query editing example](examples/query_edit_example_test.go)
shows how to parse, modify, and serialize a statement.

## Supported SQL

`query` supports these SQLite forms:

- `SELECT` with `ALL` or `DISTINCT`, targets, aliases, simple comma-separated
  relations, `WHERE`, `ORDER BY`, and `LIMIT` with `OFFSET` or comma syntax.
- Core expressions: identifiers, stars, string, decimal or hexadecimal number, blob, boolean, null,
  current-time literals, SQLite parameters, calls, unary operators, arithmetic,
  comparison, `IS`, `LIKE`, `GLOB`, `MATCH`, `AND`, and `OR`.
- `CREATE TABLE`, including temporary tables, `IF NOT EXISTS`, declared types,
  supported column and table constraints, conflict clauses, generated columns,
  `WITHOUT ROWID`, `STRICT`, and `AS SELECT`.
- `ALTER TABLE` add, drop, and rename actions.
- `CREATE [UNIQUE] INDEX`, including `IF NOT EXISTS`, collations, sort order,
  and partial-index predicates.
- `CREATE VIEW`, `DROP TABLE`, `DROP INDEX`, `DROP VIEW`, and `DROP TRIGGER`.

Unsupported syntax returns a `*query.ParseError`; it is never represented as a
different AST node. The exact supported subset and known limits are listed in
[docs/supported-sql.md](docs/supported-sql.md).

## Development

Run the complete checks locally:

```sh
go test ./...
go vet ./...
```

The grammar forms follow SQLite's official [SQL language
reference](https://www.sqlite.org/lang.html).
