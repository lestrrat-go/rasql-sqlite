package examples_test

import (
	"fmt"

	"github.com/lestrrat-go/rasql-sqlite/query"
)

func Example_query_edit() {
	statement, err := query.ParseStatement("SELECT id, name FROM users WHERE active = TRUE")
	if err != nil {
		fmt.Printf("failed to parse query: %s\n", err)
		return
	}
	selectStatement, ok := query.As[*query.SelectStatement](statement)
	if !ok {
		fmt.Println("failed to parse a SELECT statement")
		return
	}
	alias := query.Identifier{Name: "joined_at"}
	selectStatement.Targets = append(selectStatement.Targets, query.Target{
		Expression: &query.IdentifierExpression{Name: query.QualifiedName{{Name: "created_at"}}},
		Alias:      &alias,
	})

	sql, err := query.SerializeStatement(selectStatement)
	if err != nil {
		fmt.Printf("failed to serialize query: %s\n", err)
		return
	}
	fmt.Println(sql)

	// Output:
	// SELECT id, name, created_at AS joined_at FROM users WHERE active = TRUE
}
