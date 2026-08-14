package query

func (parser *parser) parseCreate() (DDLStatement, error) {
	persistence := parser.parseRelationPersistence()
	switch {
	case parser.matchKeyword("table"):
		return parser.parseCreateTable(persistence)
	case parser.matchKeyword("view"):
		return parser.parseCreateView(persistence)
	case parser.matchKeyword("unique"):
		if persistence != PermanentRelation {
			return nil, parser.errorf(parser.previous(), "temporary CREATE INDEX is not supported")
		}
		if err := parser.expectKeyword("index"); err != nil {
			return nil, err
		}
		return parser.parseCreateIndex(true)
	case parser.matchKeyword("index"):
		if persistence != PermanentRelation {
			return nil, parser.errorf(parser.previous(), "temporary CREATE INDEX is not supported")
		}
		return parser.parseCreateIndex(false)
	default:
		return nil, parser.errorf(parser.current(), "unsupported CREATE statement")
	}
}

func (parser *parser) parseCreateTable(persistence RelationPersistence) (*CreateTableStatement, error) {
	ifNotExists, err := parser.parseIfNotExists()
	if err != nil {
		return nil, err
	}
	name, err := parser.parseTableName()
	if err != nil {
		return nil, err
	}
	table := &CreateTableStatement{
		Persistence: persistence,
		IfNotExists: ifNotExists,
		Name:        name,
	}
	if parser.matchKeyword("as") {
		if !parser.matchKeyword("select") {
			return nil, parser.errorf(parser.current(), "CREATE TABLE AS requires a SELECT query")
		}
		selectStatement, err := parser.parseSelect()
		if err != nil {
			return nil, err
		}
		table.As = selectStatement
		return table, nil
	}
	if !parser.match(tokenLeftParen) {
		return nil, parser.errorf(parser.current(), "expected opening parenthesis for table definition")
	}
	if parser.match(tokenRightParen) {
		return nil, parser.errorf(parser.previous(), "CREATE TABLE requires a column or constraint")
	}
	for {
		if parser.tableConstraintStart() {
			constraint, err := parser.parseTableConstraint()
			if err != nil {
				return nil, err
			}
			table.Constraints = append(table.Constraints, constraint)
		} else {
			column, err := parser.parseColumnDefinition()
			if err != nil {
				return nil, err
			}
			table.Columns = append(table.Columns, column)
		}
		if parser.match(tokenRightParen) {
			break
		}
		if !parser.match(tokenComma) {
			return nil, parser.errorf(parser.current(), "expected comma or closing parenthesis in table definition")
		}
	}
	options, err := parser.parseTableOptions()
	if err != nil {
		return nil, err
	}
	table.Options = options
	if err := parser.expectDDLBoundary("CREATE TABLE clause"); err != nil {
		return nil, err
	}
	return table, nil
}

func (parser *parser) parseTableOptions() (TableOptions, error) {
	options := TableOptions{}
	for {
		switch {
		case parser.matchKeyword("without"):
			if options.WithoutRowID {
				return TableOptions{}, parser.errorf(parser.previous(), "WITHOUT ROWID appears more than once")
			}
			if err := parser.expectWord("rowid"); err != nil {
				return TableOptions{}, err
			}
			options.WithoutRowID = true
		case parser.matchKeyword("strict"):
			if options.Strict {
				return TableOptions{}, parser.errorf(parser.previous(), "STRICT appears more than once")
			}
			options.Strict = true
		default:
			return options, nil
		}
		if !parser.match(tokenComma) {
			return options, nil
		}
	}
}

func (parser *parser) parseCreateView(persistence RelationPersistence) (*CreateViewStatement, error) {
	ifNotExists, err := parser.parseIfNotExists()
	if err != nil {
		return nil, err
	}
	name, err := parser.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	view := &CreateViewStatement{Persistence: persistence, IfNotExists: ifNotExists, Name: name}
	if parser.current().kind == tokenLeftParen {
		columns, err := parser.parseIdentifierList()
		if err != nil {
			return nil, err
		}
		view.Columns = columns
	}
	if err := parser.expectKeyword("as"); err != nil {
		return nil, err
	}
	if !parser.matchKeyword("select") {
		return nil, parser.errorf(parser.current(), "CREATE VIEW requires a SELECT query")
	}
	selectStatement, err := parser.parseSelect()
	if err != nil {
		return nil, err
	}
	view.Query = selectStatement
	return view, nil
}

func (parser *parser) parseCreateIndex(unique bool) (*CreateIndexStatement, error) {
	ifNotExists, err := parser.parseIfNotExists()
	if err != nil {
		return nil, err
	}
	name, err := parser.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	if err := parser.expectKeyword("on"); err != nil {
		return nil, err
	}
	table, err := parser.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	elements, err := parser.parseIndexedColumnList()
	if err != nil {
		return nil, err
	}
	index := &CreateIndexStatement{
		Unique:      unique,
		IfNotExists: ifNotExists,
		Name:        name,
		Table:       table,
		Elements:    elements,
	}
	if parser.matchKeyword("where") {
		where, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		index.Where = where
	}
	if err := parser.expectDDLBoundary("CREATE INDEX clause"); err != nil {
		return nil, err
	}
	return index, nil
}

func (parser *parser) parseAlter() (DDLStatement, error) {
	if err := parser.expectKeyword("table"); err != nil {
		return nil, err
	}
	name, err := parser.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	action, err := parser.parseAlterTableAction()
	if err != nil {
		return nil, err
	}
	if err := parser.expectDDLBoundary("ALTER TABLE action"); err != nil {
		return nil, err
	}
	return &AlterTableStatement{Name: name, Action: action}, nil
}

func (parser *parser) parseAlterTableAction() (AlterTableAction, error) {
	switch {
	case parser.matchKeyword("add"):
		parser.matchKeyword("column")
		column, err := parser.parseColumnDefinition()
		if err != nil {
			return AlterTableAction{}, err
		}
		return AlterTableAction{Kind: AlterTableAddColumn, Column: &column}, nil
	case parser.matchKeyword("drop"):
		parser.matchKeyword("column")
		name, err := parser.parseIdentifier()
		if err != nil {
			return AlterTableAction{}, err
		}
		return AlterTableAction{Kind: AlterTableDropColumn, Name: &name}, nil
	case parser.matchKeyword("rename"):
		if parser.matchKeyword("column") {
			name, err := parser.parseIdentifier()
			if err != nil {
				return AlterTableAction{}, err
			}
			if err := parser.expectKeyword("to"); err != nil {
				return AlterTableAction{}, err
			}
			newName, err := parser.parseIdentifier()
			if err != nil {
				return AlterTableAction{}, err
			}
			return AlterTableAction{Kind: AlterTableRenameColumn, Name: &name, NewName: &newName}, nil
		}
		if err := parser.expectKeyword("to"); err != nil {
			return AlterTableAction{}, err
		}
		newName, err := parser.parseIdentifier()
		if err != nil {
			return AlterTableAction{}, err
		}
		return AlterTableAction{Kind: AlterTableRenameTable, NewName: &newName}, nil
	default:
		return AlterTableAction{}, parser.errorf(parser.current(), "unsupported ALTER TABLE action")
	}
}

func (parser *parser) parseDrop() (DDLStatement, error) {
	objectType, err := parser.parseDropObjectType()
	if err != nil {
		return nil, err
	}
	ifExists, err := parser.parseIfExists()
	if err != nil {
		return nil, err
	}
	name, err := parser.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	if err := parser.expectDDLBoundary("DROP clause"); err != nil {
		return nil, err
	}
	return &DropStatement{ObjectType: objectType, IfExists: ifExists, Name: name}, nil
}

func (parser *parser) parseColumnDefinition() (ColumnDefinition, error) {
	name, err := parser.parseIdentifier()
	if err != nil {
		return ColumnDefinition{}, err
	}
	dataType, err := parser.parseDataType()
	if err != nil {
		return ColumnDefinition{}, err
	}
	column := ColumnDefinition{Name: name, Type: dataType}
	for {
		constraint, found, err := parser.parseColumnConstraint()
		if err != nil {
			return ColumnDefinition{}, err
		}
		if !found {
			return column, nil
		}
		column.Constraints = append(column.Constraints, constraint)
	}
}

func (parser *parser) parseDataType() (DataType, error) {
	dataType := DataType{}
	for parser.current().kind == tokenIdentifier {
		dataType.Words = append(dataType.Words, parser.current().text)
		parser.advance()
		if parser.match(tokenLeftParen) {
			if len(dataType.Modifiers) > 0 {
				return DataType{}, parser.errorf(parser.previous(), "data type can have one modifier list")
			}
			modifiers, err := parser.parseTypeModifiers()
			if err != nil {
				return DataType{}, err
			}
			dataType.Modifiers = modifiers
		}
	}
	return dataType, nil
}

func (parser *parser) parseTypeModifiers() ([]Expression, error) {
	var modifiers []Expression
	for {
		modifier, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		modifiers = append(modifiers, modifier)
		if parser.match(tokenRightParen) {
			return modifiers, nil
		}
		if !parser.match(tokenComma) {
			return nil, parser.errorf(parser.current(), "expected comma or closing parenthesis in type modifier")
		}
	}
}

func (parser *parser) parseColumnConstraint() (ColumnConstraint, bool, error) {
	var name *Identifier
	if parser.matchKeyword("constraint") {
		constraintName, err := parser.parseIdentifier()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		name = &constraintName
	}

	switch {
	case parser.matchKeyword("not"):
		if err := parser.expectKeyword("null"); err != nil {
			return ColumnConstraint{}, false, err
		}
		conflict, err := parser.parseConflictClause()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		return ColumnConstraint{Name: name, Kind: ConstraintNotNull, Conflict: conflict}, true, nil
	case parser.matchKeyword("null"):
		return ColumnConstraint{Name: name, Kind: ConstraintNull}, true, nil
	case parser.matchKeyword("default"):
		expression, err := parser.parseDefaultExpression()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		return ColumnConstraint{Name: name, Kind: ConstraintDefault, Expression: expression}, true, nil
	case parser.matchKeyword("primary"):
		if err := parser.expectKeyword("key"); err != nil {
			return ColumnConstraint{}, false, err
		}
		constraint := ColumnConstraint{Name: name, Kind: ConstraintPrimaryKey}
		switch {
		case parser.matchKeyword("asc"):
			constraint.Direction = SortAscending
		case parser.matchKeyword("desc"):
			constraint.Direction = SortDescending
		}
		conflict, err := parser.parseConflictClause()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		constraint.Conflict = conflict
		constraint.Autoincrement = parser.matchKeyword("autoincrement")
		return constraint, true, nil
	case parser.matchKeyword("unique"):
		conflict, err := parser.parseConflictClause()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		return ColumnConstraint{Name: name, Kind: ConstraintUnique, Conflict: conflict}, true, nil
	case parser.matchKeyword("check"):
		expression, err := parser.parseParenthesizedExpression()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		return ColumnConstraint{Name: name, Kind: ConstraintCheck, Expression: expression}, true, nil
	case parser.matchKeyword("references"):
		reference, err := parser.parseReference()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		return ColumnConstraint{Name: name, Kind: ConstraintReferences, References: &reference}, true, nil
	case parser.matchKeyword("collate"):
		collation, err := parser.parseIdentifier()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		return ColumnConstraint{Name: name, Kind: ConstraintCollate, Collation: &collation}, true, nil
	case parser.currentKeyword("generated") || parser.currentKeyword("always") || parser.currentKeyword("as"):
		if parser.matchKeyword("generated") {
			parser.matchKeyword("always")
		}
		if err := parser.expectKeyword("as"); err != nil {
			return ColumnConstraint{}, false, err
		}
		expression, err := parser.parseParenthesizedExpression()
		if err != nil {
			return ColumnConstraint{}, false, err
		}
		generated := &GeneratedColumn{Expression: expression}
		switch {
		case parser.matchKeyword("virtual"):
			generated.Storage = GeneratedVirtual
		case parser.matchKeyword("stored"):
			generated.Storage = GeneratedStored
		}
		return ColumnConstraint{Name: name, Kind: ConstraintGenerated, Generated: generated}, true, nil
	default:
		if name != nil {
			return ColumnConstraint{}, false, parser.errorf(parser.current(), "expected column constraint after CONSTRAINT")
		}
		return ColumnConstraint{}, false, nil
	}
}

func (parser *parser) parseDefaultExpression() (Expression, error) {
	if parser.current().kind == tokenLeftParen {
		return parser.parseParenthesizedExpression()
	}
	return parser.parseExpression(1)
}

func (parser *parser) parseTableConstraint() (TableConstraint, error) {
	var name *Identifier
	if parser.matchKeyword("constraint") {
		constraintName, err := parser.parseIdentifier()
		if err != nil {
			return TableConstraint{}, err
		}
		name = &constraintName
	}

	switch {
	case parser.matchKeyword("primary"):
		if err := parser.expectKeyword("key"); err != nil {
			return TableConstraint{}, err
		}
		columns, err := parser.parseIndexedColumnList()
		if err != nil {
			return TableConstraint{}, err
		}
		conflict, err := parser.parseConflictClause()
		if err != nil {
			return TableConstraint{}, err
		}
		return TableConstraint{Name: name, Kind: ConstraintPrimaryKey, Columns: columns, Conflict: conflict}, nil
	case parser.matchKeyword("unique"):
		columns, err := parser.parseIndexedColumnList()
		if err != nil {
			return TableConstraint{}, err
		}
		conflict, err := parser.parseConflictClause()
		if err != nil {
			return TableConstraint{}, err
		}
		return TableConstraint{Name: name, Kind: ConstraintUnique, Columns: columns, Conflict: conflict}, nil
	case parser.matchKeyword("check"):
		expression, err := parser.parseParenthesizedExpression()
		if err != nil {
			return TableConstraint{}, err
		}
		return TableConstraint{Name: name, Kind: ConstraintCheck, Expression: expression}, nil
	case parser.matchKeyword("foreign"):
		if err := parser.expectKeyword("key"); err != nil {
			return TableConstraint{}, err
		}
		columns, err := parser.parseIndexedColumnList()
		if err != nil {
			return TableConstraint{}, err
		}
		if err := parser.expectKeyword("references"); err != nil {
			return TableConstraint{}, err
		}
		reference, err := parser.parseReference()
		if err != nil {
			return TableConstraint{}, err
		}
		return TableConstraint{Name: name, Kind: ConstraintForeignKey, Columns: columns, References: &reference}, nil
	default:
		return TableConstraint{}, parser.errorf(parser.current(), "expected table constraint")
	}
}

func (parser *parser) parseReference() (Reference, error) {
	table, err := parser.parseQualifiedName()
	if err != nil {
		return Reference{}, err
	}
	reference := Reference{Table: table}
	if parser.current().kind == tokenLeftParen {
		columns, err := parser.parseIdentifierList()
		if err != nil {
			return Reference{}, err
		}
		reference.Columns = columns
	}
	return reference, nil
}

func (parser *parser) parseIndexedColumnList() ([]IndexedColumn, error) {
	if !parser.match(tokenLeftParen) {
		return nil, parser.errorf(parser.current(), "expected opening parenthesis for indexed columns")
	}
	var columns []IndexedColumn
	for {
		expression, err := parser.parseExpression(1)
		if err != nil {
			return nil, err
		}
		column := IndexedColumn{Expression: expression}
		if parser.matchKeyword("collate") {
			collation, err := parser.parseIdentifier()
			if err != nil {
				return nil, err
			}
			column.Collation = &collation
		}
		switch {
		case parser.matchKeyword("asc"):
			column.Direction = SortAscending
		case parser.matchKeyword("desc"):
			column.Direction = SortDescending
		}
		columns = append(columns, column)
		if parser.match(tokenRightParen) {
			return columns, nil
		}
		if !parser.match(tokenComma) {
			return nil, parser.errorf(parser.current(), "expected comma or closing parenthesis in indexed columns")
		}
	}
}

func (parser *parser) parseIdentifierList() ([]Identifier, error) {
	if !parser.match(tokenLeftParen) {
		return nil, parser.errorf(parser.current(), "expected opening parenthesis for identifier list")
	}
	var identifiers []Identifier
	for {
		identifier, err := parser.parseIdentifier()
		if err != nil {
			return nil, err
		}
		identifiers = append(identifiers, identifier)
		if parser.match(tokenRightParen) {
			return identifiers, nil
		}
		if !parser.match(tokenComma) {
			return nil, parser.errorf(parser.current(), "expected comma or closing parenthesis in identifier list")
		}
	}
}

func (parser *parser) parseParenthesizedExpression() (Expression, error) {
	if !parser.match(tokenLeftParen) {
		return nil, parser.errorf(parser.current(), "expected opening parenthesis")
	}
	expression, err := parser.parseExpression(1)
	if err != nil {
		return nil, err
	}
	if !parser.match(tokenRightParen) {
		return nil, parser.errorf(parser.current(), "expected closing parenthesis")
	}
	return expression, nil
}

func (parser *parser) parseRelationPersistence() RelationPersistence {
	switch {
	case parser.matchKeyword("temporary"), parser.matchKeyword("temp"):
		return TemporaryRelation
	default:
		return PermanentRelation
	}
}

func (parser *parser) parseIfNotExists() (bool, error) {
	if !parser.matchKeyword("if") {
		return false, nil
	}
	if err := parser.expectKeyword("not"); err != nil {
		return false, err
	}
	if err := parser.expectKeyword("exists"); err != nil {
		return false, err
	}
	return true, nil
}

func (parser *parser) parseIfExists() (bool, error) {
	if !parser.matchKeyword("if") {
		return false, nil
	}
	if err := parser.expectKeyword("exists"); err != nil {
		return false, err
	}
	return true, nil
}

func (parser *parser) parseConflictClause() (ConflictResolution, error) {
	if !parser.matchKeyword("on") {
		return ConflictDefault, nil
	}
	if err := parser.expectKeyword("conflict"); err != nil {
		return ConflictDefault, err
	}
	current := parser.current()
	if current.kind != tokenKeyword {
		return ConflictDefault, parser.errorf(current, "expected conflict resolution")
	}
	parser.advance()
	switch current.text {
	case "rollback":
		return ConflictRollback, nil
	case "abort":
		return ConflictAbort, nil
	case "fail":
		return ConflictFail, nil
	case "ignore":
		return ConflictIgnore, nil
	case "replace":
		return ConflictReplace, nil
	default:
		return ConflictDefault, parser.errorf(current, "unsupported conflict resolution")
	}
}

func (parser *parser) parseDropObjectType() (DropObjectType, error) {
	current := parser.current()
	if current.kind != tokenKeyword {
		return "", parser.errorf(current, "expected SQLite DROP object type")
	}
	parser.advance()
	switch current.text {
	case "table":
		return DropTable, nil
	case "index":
		return DropIndex, nil
	case "view":
		return DropView, nil
	case "trigger":
		return DropTrigger, nil
	default:
		return "", parser.errorf(current, "unsupported SQLite DROP object type")
	}
}

func (parser *parser) tableConstraintStart() bool {
	return parser.currentKeyword("constraint") || parser.currentKeyword("primary") || parser.currentKeyword("unique") || parser.currentKeyword("check") || parser.currentKeyword("foreign")
}

func (parser *parser) currentKeyword(keyword string) bool {
	current := parser.current()
	return current.kind == tokenKeyword && current.text == keyword
}

func (parser *parser) expectDDLBoundary(clause string) error {
	current := parser.current()
	if current.kind == tokenEOF || current.kind == tokenSemicolon {
		return nil
	}
	return parser.errorf(current, "unsupported %s", clause)
}
