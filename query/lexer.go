package query

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenIdentifier
	tokenKeyword
	tokenString
	tokenBlob
	tokenNumber
	tokenParameter
	tokenComma
	tokenDot
	tokenStar
	tokenLeftParen
	tokenRightParen
	tokenSemicolon
	tokenOperator
)

type token struct {
	kind   tokenKind
	text   string
	offset int
	quoted bool
}

var keywords = map[string]struct{}{
	"abort": {}, "add": {}, "all": {}, "alter": {}, "always": {}, "and": {}, "as": {}, "asc": {},
	"autoincrement": {}, "by": {}, "cascade": {}, "check": {}, "collate": {}, "column": {}, "conflict": {},
	"constraint": {}, "create": {}, "current_date": {}, "current_time": {}, "current_timestamp": {},
	"default": {}, "desc": {}, "distinct": {}, "drop": {}, "exists": {}, "fail": {}, "false": {}, "foreign": {},
	"from": {}, "generated": {}, "glob": {}, "group": {}, "ignore": {}, "if": {}, "in": {}, "index": {},
	"is": {}, "key": {}, "like": {}, "limit": {}, "match": {}, "no": {}, "not": {}, "null": {}, "offset": {},
	"on": {}, "or": {}, "order": {}, "primary": {}, "references": {}, "rename": {}, "replace": {}, "rollback": {},
	"select": {}, "stored": {}, "strict": {}, "table": {}, "temp": {}, "temporary": {}, "to": {}, "trigger": {},
	"true": {}, "unique": {}, "using": {}, "virtual": {}, "view": {}, "where": {}, "without": {},
}

func lex(input string) ([]token, error) {
	lexer := lexer{input: input}
	return lexer.tokens()
}

type lexer struct {
	input string
	pos   int
	items []token
}

func (lexer *lexer) tokens() ([]token, error) {
	for lexer.pos < len(lexer.input) {
		runeValue, size := utf8.DecodeRuneInString(lexer.input[lexer.pos:])
		if runeValue == utf8.RuneError && size == 1 {
			return nil, newParseError(lexer.pos, "invalid UTF-8")
		}
		if unicode.IsSpace(runeValue) {
			lexer.pos += size
			continue
		}

		skipped, err := lexer.skipComment()
		if err != nil {
			return nil, err
		}
		if skipped {
			continue
		}
		if err := lexer.token(runeValue, size); err != nil {
			return nil, err
		}
	}
	lexer.items = append(lexer.items, token{kind: tokenEOF, offset: lexer.pos})
	return lexer.items, nil
}

func (lexer *lexer) skipComment() (bool, error) {
	if strings.HasPrefix(lexer.input[lexer.pos:], "--") {
		newline := strings.IndexByte(lexer.input[lexer.pos:], '\n')
		if newline == -1 {
			lexer.pos = len(lexer.input)
			return true, nil
		}
		lexer.pos += newline + 1
		return true, nil
	}
	if !strings.HasPrefix(lexer.input[lexer.pos:], "/*") {
		return false, nil
	}

	start := lexer.pos
	end := strings.Index(lexer.input[lexer.pos+2:], "*/")
	if end == -1 {
		return false, newParseError(start, "unterminated block comment")
	}
	lexer.pos += end + 4
	return true, nil
}

func (lexer *lexer) token(runeValue rune, size int) error {
	start := lexer.pos
	switch {
	case isIdentifierStart(runeValue):
		if (runeValue == 'x' || runeValue == 'X') && lexer.nextByteIsQuote(size) {
			lexer.pos += size
			return lexer.blobLiteral(start)
		}
		lexer.identifier(start)
	case unicode.IsDigit(runeValue):
		lexer.number(start)
	case runeValue == '.' && lexer.nextIsDigit(size):
		lexer.number(start)
	case runeValue == '\'':
		return lexer.stringLiteral(start)
	case runeValue == '"' || runeValue == '`':
		return lexer.quotedIdentifier(start, byte(runeValue))
	case runeValue == '[':
		return lexer.bracketIdentifier(start)
	case runeValue == '?' || runeValue == ':' || runeValue == '@' || runeValue == '$':
		return lexer.parameter(start)
	case runeValue == ',':
		lexer.emit(tokenComma, ",", start)
		lexer.pos += size
	case runeValue == '.':
		lexer.emit(tokenDot, ".", start)
		lexer.pos += size
	case runeValue == '*':
		lexer.emit(tokenStar, "*", start)
		lexer.pos += size
	case runeValue == '(':
		lexer.emit(tokenLeftParen, "(", start)
		lexer.pos += size
	case runeValue == ')':
		lexer.emit(tokenRightParen, ")", start)
		lexer.pos += size
	case runeValue == ';':
		lexer.emit(tokenSemicolon, ";", start)
		lexer.pos += size
	case isOperator(runeValue):
		lexer.operator(start)
	default:
		return newParseError(start, fmt.Sprintf("unexpected character %q", runeValue))
	}
	return nil
}

func (lexer *lexer) identifier(start int) {
	lexer.advanceRune()
	for lexer.pos < len(lexer.input) {
		runeValue, _ := utf8.DecodeRuneInString(lexer.input[lexer.pos:])
		if !isIdentifierPart(runeValue) {
			break
		}
		lexer.advanceRune()
	}
	text := strings.ToLower(lexer.input[start:lexer.pos])
	if _, ok := keywords[text]; ok {
		lexer.emit(tokenKeyword, text, start)
		return
	}
	lexer.emit(tokenIdentifier, text, start)
}

func (lexer *lexer) number(start int) {
	if lexer.input[start] == '0' && lexer.pos+1 < len(lexer.input) && (lexer.input[lexer.pos+1] == 'x' || lexer.input[lexer.pos+1] == 'X') {
		lexer.pos += 2
		digits := lexer.pos
		for lexer.pos < len(lexer.input) && isHexDigit(rune(lexer.input[lexer.pos])) {
			lexer.pos++
		}
		if lexer.pos > digits {
			lexer.emit(tokenNumber, lexer.input[start:lexer.pos], start)
			return
		}
		lexer.pos = start
	}
	lexer.consumeDigits()
	if lexer.pos < len(lexer.input) && lexer.input[lexer.pos] == '.' {
		lexer.pos++
		lexer.consumeDigits()
	}
	if lexer.pos < len(lexer.input) && (lexer.input[lexer.pos] == 'e' || lexer.input[lexer.pos] == 'E') {
		exponent := lexer.pos
		lexer.pos++
		if lexer.pos < len(lexer.input) && (lexer.input[lexer.pos] == '+' || lexer.input[lexer.pos] == '-') {
			lexer.pos++
		}
		digits := lexer.pos
		lexer.consumeDigits()
		if lexer.pos == digits {
			lexer.pos = exponent
		}
	}
	lexer.emit(tokenNumber, lexer.input[start:lexer.pos], start)
}

func (lexer *lexer) stringLiteral(start int) error {
	lexer.pos++
	value, err := lexer.quotedValue(start, '\'')
	if err != nil {
		return err
	}
	lexer.items = append(lexer.items, token{kind: tokenString, text: value, offset: start})
	return nil
}

func (lexer *lexer) blobLiteral(start int) error {
	lexer.pos++
	value, err := lexer.quotedValue(start, '\'')
	if err != nil {
		return err
	}
	for _, runeValue := range value {
		if !isHexDigit(runeValue) {
			return newParseError(start, "blob literal must contain hexadecimal digits")
		}
	}
	if len(value)%2 != 0 {
		return newParseError(start, "blob literal must contain an even number of hexadecimal digits")
	}
	lexer.items = append(lexer.items, token{kind: tokenBlob, text: strings.ToUpper(value), offset: start})
	return nil
}

func (lexer *lexer) quotedIdentifier(start int, quote byte) error {
	lexer.pos++
	value, err := lexer.quotedValue(start, quote)
	if err != nil {
		return err
	}
	lexer.items = append(lexer.items, token{kind: tokenIdentifier, text: value, offset: start, quoted: true})
	return nil
}

func (lexer *lexer) bracketIdentifier(start int) error {
	lexer.pos++
	end := strings.IndexByte(lexer.input[lexer.pos:], ']')
	if end == -1 {
		return newParseError(start, "unterminated bracketed identifier")
	}
	value := lexer.input[lexer.pos : lexer.pos+end]
	lexer.pos += end + 1
	lexer.items = append(lexer.items, token{kind: tokenIdentifier, text: value, offset: start, quoted: true})
	return nil
}

func (lexer *lexer) quotedValue(start int, quote byte) (string, error) {
	var value strings.Builder
	for lexer.pos < len(lexer.input) {
		if lexer.input[lexer.pos] != quote {
			runeValue, size := utf8.DecodeRuneInString(lexer.input[lexer.pos:])
			if runeValue == utf8.RuneError && size == 1 {
				return "", newParseError(lexer.pos, "invalid UTF-8")
			}
			value.WriteString(lexer.input[lexer.pos : lexer.pos+size])
			lexer.pos += size
			continue
		}
		if lexer.pos+1 < len(lexer.input) && lexer.input[lexer.pos+1] == quote {
			value.WriteByte(quote)
			lexer.pos += 2
			continue
		}
		lexer.pos++
		return value.String(), nil
	}
	return "", newParseError(start, "unterminated quoted value")
}

func (lexer *lexer) parameter(start int) error {
	prefix := lexer.input[lexer.pos]
	lexer.pos++
	if prefix == '?' {
		digits := lexer.pos
		lexer.consumeDigits()
		if lexer.pos == digits {
			lexer.emit(tokenParameter, "?", start)
			return nil
		}
		lexer.emit(tokenParameter, lexer.input[start:lexer.pos], start)
		return nil
	}
	if lexer.pos == len(lexer.input) || !isParameterNameStart(lexer.input[lexer.pos]) {
		return newParseError(start, "expected parameter name")
	}
	for lexer.pos < len(lexer.input) && isParameterNamePart(lexer.input[lexer.pos]) {
		lexer.pos++
	}
	lexer.emit(tokenParameter, lexer.input[start:lexer.pos], start)
	return nil
}

func (lexer *lexer) operator(start int) {
	for lexer.pos < len(lexer.input) {
		runeValue, _ := utf8.DecodeRuneInString(lexer.input[lexer.pos:])
		if !isOperator(runeValue) {
			break
		}
		lexer.advanceRune()
	}
	lexer.emit(tokenOperator, lexer.input[start:lexer.pos], start)
}

func (lexer *lexer) consumeDigits() {
	for lexer.pos < len(lexer.input) && lexer.input[lexer.pos] >= '0' && lexer.input[lexer.pos] <= '9' {
		lexer.pos++
	}
}

func (lexer *lexer) advanceRune() {
	_, size := utf8.DecodeRuneInString(lexer.input[lexer.pos:])
	lexer.pos += size
}

func (lexer *lexer) nextByteIsQuote(size int) bool {
	next := lexer.pos + size
	return next < len(lexer.input) && lexer.input[next] == '\''
}

func (lexer *lexer) nextIsDigit(size int) bool {
	next := lexer.pos + size
	return next < len(lexer.input) && lexer.input[next] >= '0' && lexer.input[next] <= '9'
}

func (lexer *lexer) emit(kind tokenKind, text string, offset int) {
	lexer.items = append(lexer.items, token{kind: kind, text: text, offset: offset})
}

func isIdentifierStart(runeValue rune) bool {
	return runeValue == '_' || unicode.IsLetter(runeValue)
}

func isIdentifierPart(runeValue rune) bool {
	return isIdentifierStart(runeValue) || unicode.IsDigit(runeValue) || runeValue == '$'
}

func isParameterNameStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isParameterNamePart(value byte) bool {
	return isParameterNameStart(value) || value >= '0' && value <= '9' || value == '$'
}

func isHexDigit(runeValue rune) bool {
	return runeValue >= '0' && runeValue <= '9' || runeValue >= 'a' && runeValue <= 'f' || runeValue >= 'A' && runeValue <= 'F'
}

func isValidParameter(value string) bool {
	tokens, err := lex(value)
	if err != nil || len(tokens) != 2 || tokens[0].kind != tokenParameter || tokens[0].text != value {
		return false
	}
	if len(value) == 1 || value[0] != '?' {
		return true
	}
	index, err := strconv.Atoi(value[1:])
	return err == nil && index > 0
}

func isOperator(runeValue rune) bool {
	return strings.ContainsRune("+-/%<>=~!@#^&|`?:", runeValue)
}
