
package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxInputBytes = 1 << 20
	MaxTokens     = 100_000
	MaxDepth      = 64
	MaxStringSize = 64 << 10
)

type Kind uint8

const (
	EOF Kind = iota
	Ident
	String
	Integer
	True
	False
	LBrace
	RBrace
	LBracket
	RBracket
	Colon
	Comma
)

type Token struct {
	Kind Kind
	Text string
	Pos  int
}

type Lexer struct {
	input string
	pos   int
	count int
}

func NewLexer(input string) (*Lexer, error) {
	if len(input) > MaxInputBytes {
		return nil, errors.New("input too large")
	}

	if !utf8.ValidString(input) {
		return nil, errors.New("invalid UTF-8")
	}

	return &Lexer{input: input}, nil
}

func isIdentStart(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) ||
		(c >= '0' && c <= '9')
}

func (l *Lexer) skipSpace() {
	for l.pos < len(l.input) {
		switch l.input[l.pos] {
		case ' ', '\t', '\r', '\n':
			l.pos++
		default:
			return
		}
	}
}

func (l *Lexer) Next() (Token, error) {
	l.skipSpace()

	if l.count >= MaxTokens {
		return Token{}, errors.New("token limit exceeded")
	}

	if l.pos >= len(l.input) {
		return Token{Kind: EOF, Pos: l.pos}, nil
	}

	start := l.pos
	c := l.input[l.pos]
	l.pos++

	var tok Token

	switch c {
	case '{':
		tok.Kind = LBrace
	case '}':
		tok.Kind = RBrace
	case '[':
		tok.Kind = LBracket
	case ']':
		tok.Kind = RBracket
	case ':':
		tok.Kind = Colon
	case ',':
		tok.Kind = Comma

	case '"':
		var b strings.Builder

		for {
			if l.pos >= len(l.input) {
				return Token{}, errors.New("unterminated string")
			}

			ch := l.input[l.pos]
			l.pos++

			if ch == '"' {
				break
			}

			if ch < 0x20 {
				return Token{}, errors.New("control character in string")
			}

			if ch == '\\' {
				if l.pos >= len(l.input) {
					return Token{}, errors.New("incomplete escape")
				}

				escaped := l.input[l.pos]
				l.pos++

				switch escaped {
				case '"', '\\', '/':
					b.WriteByte(escaped)
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case 'b':
					b.WriteByte('\b')
				case 'f':
					b.WriteByte('\f')
				default:
					return Token{}, errors.New("unsupported escape")
				}
			} else {
				b.WriteByte(ch)
			}

			if b.Len() > MaxStringSize {
				return Token{}, errors.New("string too large")
			}
		}

		tok.Kind = String
		tok.Text = b.String()

	case '0', '1', '2', '3', '4',
		'5', '6', '7', '8', '9', '-':

		for l.pos < len(l.input) {
			ch := l.input[l.pos]
			if (ch >= '0' && ch <= '9') ||
				(ch == '-' && l.pos == start+1) {
				l.pos++
			} else {
				break
			}
		}

		raw := l.input[start:l.pos]

		if _, err := strconv.ParseInt(raw, 10, 64); err != nil {
			return Token{}, errors.New("invalid integer")
		}

		tok.Kind = Integer
		tok.Text = raw

	default:
		if !isIdentStart(c) {
			return Token{}, fmt.Errorf("invalid character at byte %d", start)
		}

		for l.pos < len(l.input) &&
			isIdentPart(l.input[l.pos]) {
			l.pos++
		}

		word := l.input[start:l.pos]

		switch word {
		case "true":
			tok.Kind = True
		case "false":
			tok.Kind = False
		default:
			tok.Kind = Ident
			tok.Text = word
		}
	}

	tok.Pos = start
	l.count++

	return tok, nil
}

type Value interface{}

type Object map[string]Value
type Array []Value

type Parser struct {
	lexer *Lexer
	cur   Token
	depth int
}

func NewParser(input string) (*Parser, error) {
	lexer, err := NewLexer(input)
	if err != nil {
		return nil, err
	}

	p := &Parser{lexer: lexer}

	if err := p.advance(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Parser) advance() error {
	tok, err := p.lexer.Next()
	if err != nil {
		return err
	}

	p.cur = tok
	return nil
}

func (p *Parser) expect(kind Kind) error {
	if p.cur.Kind != kind {
		return fmt.Errorf(
			"unexpected token at byte %d",
			p.cur.Pos,
		)
	}

	return p.advance()
}

func (p *Parser) enter() error {
	p.depth++

	if p.depth > MaxDepth {
		p.depth--
		return errors.New("maximum nesting depth exceeded")
	}

	return nil
}

func (p *Parser) leave() {
	p.depth--
}

func (p *Parser) Parse() (Value, error) {
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	if p.cur.Kind != EOF {
		return nil, fmt.Errorf(
			"trailing input at byte %d",
			p.cur.Pos,
		)
	}

	return value, nil
}

func (p *Parser) parseValue() (Value, error) {
	switch p.cur.Kind {
	case String:
		v := p.cur.Text
		return v, p.advance()

	case Integer:
		v, err := strconv.ParseInt(p.cur.Text, 10, 64)
		if err != nil {
			return nil, err
		}
		return v, p.advance()

	case True:
		return true, p.advance()

	case False:
		return false, p.advance()

	case LBrace:
		return p.parseObject()

	case LBracket:
		return p.parseArray()

	default:
		return nil, fmt.Errorf(
			"expected value at byte %d",
			p.cur.Pos,
		)
	}
}

func (p *Parser) parseObject() (Value, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	if err := p.expect(LBrace); err != nil {
		return nil, err
	}

	result := make(Object)

	if p.cur.Kind == RBrace {
		return result, p.advance()
	}

	for {
		if p.cur.Kind != Ident && p.cur.Kind != String {
			return nil, fmt.Errorf(
				"expected object key at byte %d",
				p.cur.Pos,
			)
		}

		key := p.cur.Text

		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate key: %q", key)
		}

		if err := p.advance(); err != nil {
			return nil, err
		}

		if err := p.expect(Colon); err != nil {
			return nil, err
		}

		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		result[key] = value

		if p.cur.Kind == RBrace {
			return result, p.advance()
		}

		if err := p.expect(Comma); err != nil {
			return nil, err
		}
	}
}

func (p *Parser) parseArray() (Value, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	if err := p.expect(LBracket); err != nil {
		return nil, err
	}

	result := make(Array, 0)

	if p.cur.Kind == RBracket {
		return result, p.advance()
	}

	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		result = append(result, value)

		if p.cur.Kind == RBracket {
			return result, p.advance()
		}

		if err := p.expect(Comma); err != nil {
			return nil, err
		}
	}
}

func main() {
	input := `{
		name: "secure-parser",
		version: 1,
		enabled: true,
		features: [
			"lexer",
			"parser",
			"validation"
		],
		settings: {
			max_depth: 64,
			strict: true
		}
	}`

	parser, err := NewParser(input)
	if err != nil {
		fmt.Println("Parser initialization error:", err)
		return
	}

	result, err := parser.Parse()
	if err != nil {
		fmt.Println("Parse error:", err)
		return
	}

	fmt.Printf("Parsed successfully:\n%#v\n", result)
}
