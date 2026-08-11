package query

import (
	"fmt"
	"strings"
)

// Parse parses supported SQLite statements into an editable Query.
//
// It follows the core SELECT, CREATE TABLE, ALTER TABLE, DROP, CREATE INDEX,
// and CREATE VIEW productions from SQLite's language specification. Unsupported
// grammar branches return an error instead of an incomplete result.
func Parse(input string) (*Query, error) {
	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}

	parser := parser{tokens: tokens}
	return parser.parseQuery()
}

// ParseStatement parses exactly one supported SQLite statement.
func ParseStatement(input string) (Statement, error) {
	query, err := Parse(input)
	if err != nil {
		return nil, err
	}
	if len(query.Statements) != 1 {
		return nil, newParseError(len(input), "expected exactly one statement")
	}
	return query.Statements[0], nil
}

// ParseError describes a syntax error and its zero-based byte offset.
type ParseError struct {
	Offset  int
	Message string
}

// Error returns the error in a form suitable for users.
func (err *ParseError) Error() string {
	return fmt.Sprintf("query: %s at byte %d", err.Message, err.Offset)
}

type parser struct {
	tokens []token
	index  int
}

func (parser *parser) parseQuery() (*Query, error) {
	query := &Query{}
	for {
		for parser.match(tokenSemicolon) {
		}
		if parser.current().kind == tokenEOF {
			return query, nil
		}

		statement, err := parser.parseStatement()
		if err != nil {
			return nil, err
		}
		query.Statements = append(query.Statements, statement)
		if parser.current().kind == tokenEOF {
			return query, nil
		}
		if !parser.match(tokenSemicolon) {
			return nil, parser.errorf(parser.current(), "expected semicolon or end of input")
		}
	}
}

func (parser *parser) parseStatement() (Statement, error) {
	switch {
	case parser.matchKeyword("select"):
		return parser.parseSelect()
	case parser.matchKeyword("create"):
		return parser.parseCreate()
	case parser.matchKeyword("alter"):
		return parser.parseAlter()
	case parser.matchKeyword("drop"):
		return parser.parseDrop()
	default:
		return nil, parser.errorf(parser.current(), "unsupported statement")
	}
}

func (parser *parser) parseSelect() (*SelectStatement, error) {
	statement := &SelectStatement{}
	switch {
	case parser.matchKeyword("all"):
		statement.All = true
	case parser.matchKeyword("distinct"):
		statement.Distinct = true
	}

	if parser.selectClauseBoundary(parser.current()) {
		return nil, parser.errorf(parser.current(), "expected SELECT target")
	}
	targets, err := parser.parseTargets()
	if err != nil {
		return nil, err
	}
	statement.Targets = targets
	if parser.matchKeyword("from") {
		from, err := parser.parseFrom()
		if err != nil {
			return nil, err
		}
		statement.FromItems = from
	}
	if parser.matchKeyword("where") {
		where, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		statement.Where = where
	}
	if parser.matchKeyword("order") {
		if err := parser.expectKeyword("by"); err != nil {
			return nil, err
		}
		orderBy, err := parser.parseOrderBy()
		if err != nil {
			return nil, err
		}
		statement.OrderTerms = orderBy
	}

	limit, err := parser.parseLimit()
	if err != nil {
		return nil, err
	}
	statement.Limit = limit
	if parser.selectClauseBoundary(parser.current()) {
		return statement, nil
	}
	return nil, parser.errorf(parser.current(), "unsupported SELECT clause")
}

func (parser *parser) parseTargets() ([]Target, error) {
	var targets []Target
	for {
		expression, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		target := Target{Expression: expression}
		if parser.matchKeyword("as") {
			alias, err := parser.parseIdentifier()
			if err != nil {
				return nil, err
			}
			target.Alias = &alias
		} else if parser.current().kind == tokenIdentifier {
			alias, err := parser.parseIdentifier()
			if err != nil {
				return nil, err
			}
			target.Alias = &alias
		}
		targets = append(targets, target)
		if !parser.match(tokenComma) {
			return targets, nil
		}
	}
}

func (parser *parser) parseFrom() ([]FromItem, error) {
	var from []FromItem
	for {
		relation, err := parser.parseQualifiedName()
		if err != nil {
			return nil, err
		}
		item := FromItem{Relation: relation}
		if parser.matchKeyword("as") {
			alias, err := parser.parseIdentifier()
			if err != nil {
				return nil, err
			}
			item.Alias = &alias
		} else if parser.current().kind == tokenIdentifier {
			alias, err := parser.parseIdentifier()
			if err != nil {
				return nil, err
			}
			item.Alias = &alias
		}
		from = append(from, item)
		if !parser.match(tokenComma) {
			return from, nil
		}
	}
}

func (parser *parser) parseOrderBy() ([]OrderTerm, error) {
	var orderBy []OrderTerm
	for {
		expression, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		term := OrderTerm{Expression: expression}
		if parser.matchKeyword("collate") {
			collation, err := parser.parseIdentifier()
			if err != nil {
				return nil, err
			}
			term.Collation = &collation
		}
		switch {
		case parser.matchKeyword("asc"):
			term.Direction = SortAscending
		case parser.matchKeyword("desc"):
			term.Direction = SortDescending
		}
		orderBy = append(orderBy, term)
		if !parser.match(tokenComma) {
			return orderBy, nil
		}
	}
}

func (parser *parser) parseLimit() (*LimitClause, error) {
	if !parser.matchKeyword("limit") {
		return nil, nil
	}
	count, err := parser.parseExpression(1)
	if err != nil {
		return nil, err
	}
	limit := &LimitClause{Count: count}
	if parser.match(tokenComma) {
		offset := limit.Count
		count, err = parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		limit.Count = count
		limit.Offset = offset
		return limit, nil
	}
	if parser.matchKeyword("offset") {
		offset, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		limit.Offset = offset
	}
	return limit, nil
}

func (parser *parser) parseExpression(minimumPrecedence int) (Expression, error) {
	left, err := parser.parsePrefix()
	if err != nil {
		return nil, err
	}
	for {
		if negated, ok := parser.matchIn(minimumPrecedence); ok {
			values, err := parser.parseInValues()
			if err != nil {
				return nil, err
			}
			left = &InExpression{Expression: left, Negated: negated, Values: values}
			continue
		}
		operator, precedence, ok := parser.binaryOperator()
		if !ok || precedence < minimumPrecedence {
			return left, nil
		}
		parser.advance()
		if operator == "IS" && parser.matchKeyword("not") {
			operator = "IS NOT"
		}
		right, err := parser.parseExpression(precedence + 1)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpression{Left: left, Operator: operator, Right: right}
	}
}

// matchIn consumes an IN or NOT IN operator and reports whether it was
// negated. It compares against comparisonPrecedence, which IN shares with the
// other comparison operators, so a caller parsing a tighter-binding right
// operand leaves the operator for its own caller to consume.
func (parser *parser) matchIn(minimumPrecedence int) (bool, bool) {
	if comparisonPrecedence < minimumPrecedence {
		return false, false
	}
	current := parser.current()
	if current.kind != tokenKeyword {
		return false, false
	}
	switch current.text {
	case "in":
		parser.advance()
		return false, true
	case "not":
		next := parser.peek(1)
		if next.kind != tokenKeyword || next.text != "in" {
			return false, false
		}
		parser.advance()
		parser.advance()
		return true, true
	}
	return false, false
}

// parseInValues parses the parenthesized value list on the right of IN. The
// subquery and table-name forms SQLite also accepts there are reported as
// unsupported rather than parsed as something else.
func (parser *parser) parseInValues() ([]Expression, error) {
	if !parser.match(tokenLeftParen) {
		return nil, parser.errorf(parser.current(), "expected value list after IN")
	}
	if parser.match(tokenRightParen) {
		return []Expression{}, nil
	}
	if current := parser.current(); current.kind == tokenKeyword && (current.text == "select" || current.text == "values" || current.text == "with") {
		return nil, parser.errorf(current, "expected value list after IN; subqueries are unsupported")
	}
	var values []Expression
	for {
		value, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if parser.match(tokenRightParen) {
			return values, nil
		}
		if !parser.match(tokenComma) {
			return nil, parser.errorf(parser.current(), "expected comma or closing parenthesis")
		}
	}
}

func (parser *parser) parsePrefix() (Expression, error) {
	current := parser.current()
	if current.kind == tokenOperator && (current.text == "+" || current.text == "-" || current.text == "~") {
		parser.advance()
		expression, err := parser.parseExpression(7)
		if err != nil {
			return nil, err
		}
		return &UnaryExpression{Operator: current.text, Expression: expression}, nil
	}
	if parser.matchKeyword("not") {
		expression, err := parser.parseExpression(7)
		if err != nil {
			return nil, err
		}
		return &UnaryExpression{Operator: "NOT", Expression: expression}, nil
	}

	switch current.kind {
	case tokenStar:
		parser.advance()
		return &StarExpression{}, nil
	case tokenString:
		parser.advance()
		return &Literal{Kind: StringLiteral, Value: current.text}, nil
	case tokenBlob:
		parser.advance()
		return &Literal{Kind: BlobLiteral, Value: current.text}, nil
	case tokenNumber:
		parser.advance()
		return &Literal{Kind: NumberLiteral, Value: current.text}, nil
	case tokenParameter:
		parser.advance()
		if !isValidParameter(current.text) {
			return nil, parser.errorf(current, "invalid parameter")
		}
		return &Parameter{Name: current.text}, nil
	case tokenIdentifier:
		return parser.parseIdentifierExpression()
	case tokenLeftParen:
		parser.advance()
		expression, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		if !parser.match(tokenRightParen) {
			return nil, parser.errorf(parser.current(), "expected closing parenthesis")
		}
		return expression, nil
	case tokenKeyword:
		switch current.text {
		case "true", "false":
			parser.advance()
			return &Literal{Kind: BooleanLiteral, Value: current.text}, nil
		case "null":
			parser.advance()
			return &Literal{Kind: NullLiteral}, nil
		case "current_date", "current_time", "current_timestamp":
			parser.advance()
			return &Literal{Kind: CurrentTimeLiteral, Value: current.text}, nil
		}
	}
	return nil, parser.errorf(current, "expected expression")
}

func (parser *parser) parseIdentifierExpression() (Expression, error) {
	name, err := parser.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	if parser.match(tokenLeftParen) {
		arguments, err := parser.parseCallArguments()
		if err != nil {
			return nil, err
		}
		return &CallExpression{Function: name, Arguments: arguments}, nil
	}
	if len(name) > 0 && name[len(name)-1].Name == "*" {
		return &StarExpression{Qualifier: name[:len(name)-1]}, nil
	}
	return &IdentifierExpression{Name: name}, nil
}

func (parser *parser) parseCallArguments() ([]Expression, error) {
	if parser.match(tokenRightParen) {
		return nil, nil
	}
	var arguments []Expression
	for {
		argument, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, argument)
		if parser.match(tokenRightParen) {
			return arguments, nil
		}
		if !parser.match(tokenComma) {
			return nil, parser.errorf(parser.current(), "expected comma or closing parenthesis")
		}
	}
}

func (parser *parser) parseQualifiedName() (QualifiedName, error) {
	identifier, err := parser.parseIdentifier()
	if err != nil {
		return nil, err
	}
	name := QualifiedName{identifier}
	for parser.match(tokenDot) {
		if parser.match(tokenStar) {
			name = append(name, Identifier{Name: "*"})
			return name, nil
		}
		identifier, err := parser.parseIdentifier()
		if err != nil {
			return nil, err
		}
		name = append(name, identifier)
	}
	return name, nil
}

func (parser *parser) parseIdentifier() (Identifier, error) {
	current := parser.current()
	if current.kind != tokenIdentifier {
		return Identifier{}, parser.errorf(current, "expected identifier")
	}
	parser.advance()
	return Identifier{Name: current.text, Quoted: current.quoted}, nil
}

// comparisonPrecedence is the binding power SQLite gives =, IS, LIKE, GLOB,
// MATCH and IN, which all sit between AND and string concatenation.
const comparisonPrecedence = 3

func (parser *parser) binaryOperator() (string, int, bool) {
	current := parser.current()
	if current.kind == tokenOperator {
		switch current.text {
		case "=", "==", "!=", "<>", "<", "<=", ">", ">=":
			return current.text, comparisonPrecedence, true
		case "||":
			return current.text, 4, true
		case "+", "-":
			return current.text, 5, true
		case "*", "/", "%":
			return current.text, 6, true
		}
	}
	if current.kind == tokenStar {
		return "*", 6, true
	}
	if current.kind != tokenKeyword {
		return "", 0, false
	}
	switch current.text {
	case "or":
		return "OR", 1, true
	case "and":
		return "AND", 2, true
	case "is":
		return "IS", comparisonPrecedence, true
	case "like":
		return "LIKE", comparisonPrecedence, true
	case "glob":
		return "GLOB", comparisonPrecedence, true
	case "match":
		return "MATCH", comparisonPrecedence, true
	}
	return "", 0, false
}

func (parser *parser) selectClauseBoundary(current token) bool {
	if current.kind == tokenEOF || current.kind == tokenSemicolon {
		return true
	}
	if current.kind != tokenKeyword {
		return false
	}
	switch current.text {
	case "from", "where", "order", "limit", "offset":
		return true
	}
	return false
}

func (parser *parser) expectKeyword(keyword string) error {
	if parser.matchKeyword(keyword) {
		return nil
	}
	return parser.errorf(parser.current(), "expected %s", strings.ToUpper(keyword))
}

func (parser *parser) expectWord(word string) error {
	current := parser.current()
	if (current.kind == tokenIdentifier || current.kind == tokenKeyword) && current.text == word {
		parser.advance()
		return nil
	}
	return parser.errorf(current, "expected %s", strings.ToUpper(word))
}

func (parser *parser) matchKeyword(keyword string) bool {
	current := parser.current()
	if current.kind != tokenKeyword || current.text != keyword {
		return false
	}
	parser.advance()
	return true
}

func (parser *parser) match(kind tokenKind) bool {
	if parser.current().kind != kind {
		return false
	}
	parser.advance()
	return true
}

func (parser *parser) current() token {
	return parser.tokens[parser.index]
}

func (parser *parser) previous() token {
	return parser.tokens[parser.index-1]
}

// peek returns the token offset positions ahead of the current one, or the
// final token once offset runs past the end. The token stream always ends in
// tokenEOF, so a caller looking ahead never has to bounds-check.
func (parser *parser) peek(offset int) token {
	index := parser.index + offset
	if index >= len(parser.tokens) {
		return parser.tokens[len(parser.tokens)-1]
	}
	return parser.tokens[index]
}

func (parser *parser) advance() {
	if parser.current().kind != tokenEOF {
		parser.index++
	}
}

func (parser *parser) errorf(current token, format string, arguments ...any) error {
	return newParseError(current.offset, fmt.Sprintf(format, arguments...))
}

func newParseError(offset int, message string) *ParseError {
	return &ParseError{Offset: offset, Message: message}
}
