package scan

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/scanner"
)

const (
	// Must be same as in text/scanner
	EOF = -(iota + 1)
	Ident
	Int
	Float
	Char
	String
	RawString
	Comment
	// Local additions
	Whitespace
)

// Maps scanner.X to local X
var tokMap = [...]rune{
	-scanner.EOF:       EOF,
	-scanner.Ident:     Ident,
	-scanner.Int:       Int,
	-scanner.Float:     Float,
	-scanner.Char:      Char,
	-scanner.String:    String,
	-scanner.RawString: RawString,
	-scanner.Comment:   Comment,
}

var tokNames = [...]string{
	-EOF:        "EOF",
	-Ident:      "Ident",
	-Int:        "Int",
	-Float:      "Float",
	-Char:       "Char",
	-String:     "String",
	-RawString:  "RawString",
	-Comment:    "Comment",
	-Whitespace: "Whitespace",
}

type savedToken struct {
	tok        rune
	start, end uint
	pos        Position
}

type save struct {
	saveIndex int
}

type Scanner struct {
	scanner     scanner.Scanner
	buf         []byte
	savedTokens []savedToken
	saveIndex   int
	saves       []save
	whitespace  uint64
}

type Position scanner.Position

func (s *Scanner) Init(src io.Reader) *Scanner {
	s.scanner.Init(src)
	// Pass comments.
	s.scanner.Mode &^= scanner.SkipComments
	// We'll handle white space in next() below.
	s.whitespace = s.scanner.Whitespace
	s.scanner.Whitespace = 0
	return s
}

func (s *Scanner) Save() (v int) {
	v = len(s.saves)
	s.saves = append(s.saves, save{saveIndex: s.saveIndex})
	return
}

func (s *Scanner) Restore(v int) (tok rune) {
	s.saveIndex = s.saves[v].saveIndex
	s.saves = s.saves[:v]
	tok = s.savedTokens[s.saveIndex].tok
	return
}

func (s *Scanner) Advance() {
	l := len(s.saves) - 1
	s.saves = s.saves[:l]
	if l == 0 {
		s.saveIndex = 0
		s.buf = s.buf[:0]
		s.savedTokens = s.savedTokens[:0]
	}
}

func (s *Scanner) next() (tok rune, text string) {
	nWhite := 0
	for {
		tok = s.scanner.Peek()
		if tok < 0 || s.whitespace&(1<<uint(tok)) == 0 {
			break
		}
		nWhite++
		s.scanner.Next()
	}

	if nWhite > 0 {
		tok = Whitespace
		text = ""
	} else {
		tok = s.scanner.Scan()
		text = s.scanner.TokenText()
		if tok < 0 && int(-tok) < len(tokMap) {
			tok = tokMap[-tok]
		}
	}
	return
}

func (s *Scanner) Scan() (tok rune, text string) {
	if s.saveIndex < len(s.savedTokens) {
		t := &s.savedTokens[s.saveIndex]
		tok, text = t.tok, string(s.buf[t.start:t.end])
	} else {
		pos := s.scanner.Pos()
		tok, text = s.next()
		if len(s.saves) > 0 {
			start := uint(len(s.buf))
			l := uint(len(text))
			s.buf = append(s.buf, text...)
			s.savedTokens = append(s.savedTokens, savedToken{tok: tok, start: start, end: start + l, pos: Position(pos)})
		}
	}
	s.saveIndex++
	return
}

// Like Scan but skips white space.
func (s *Scanner) Next() (tok rune, text string) {
	tok, text = s.Scan()
	for tok == Whitespace {
		tok, text = s.Scan()
	}
	return
}

func (s *Scanner) Pos() Position {
	if s.saveIndex < len(s.savedTokens) {
		return s.savedTokens[s.saveIndex].pos
	} else {
		return Position(s.scanner.Pos())
	}
}
func (pos Position) String() string { return scanner.Position(pos).String() }

var NoMatch = errors.New("no match")

func (s *Scanner) Parse(template string, args ...interface{}) (err error) {
	fs := strings.Fields(template)
	ai := 0
	v := s.Save()
	defer func() {
		if err != nil {
			s.Restore(v)
		} else {
			s.Advance()
		}
	}()
	for _, f := range fs {
		tok, text := s.Scan()
		for tok == Whitespace {
			tok, text = s.Scan()
		}
		switch {
		case f == "%":
			a := args[ai]
			ai++
			if p, ok := a.(Parser); ok {
				err = p.Parse(text)
				if err != nil {
					return
				}
			} else {
				err = fmt.Errorf("%s: %v does not implement Parser interface", s.Pos(), a)
				return
			}

		case f[0] == '%':
			switch tok {
			case Ident, Int, Float, String:
				break
			default:
				err = fmt.Errorf("%s: expected identifier got `%s'", s.Pos(), text)
				return
			}
			_, err = fmt.Sscanf(text, f, args[ai:ai+1]...)
			if err != nil {
				err = fmt.Errorf("%s: `%s' %s", s.Pos(), text, err)
				return
			}
			ai++

		default:
			// Match exact text or for X*Y match up to X and only match Y if present.
			if ok := f == text; !ok {
				if star := strings.Index(f, "*"); star > 0 {
					x := strings.Index(text, f[:star])
					if x == 0 {
						x = strings.Index(f[star+1:], text[star:])
					}
					ok = x == 0
				}
				if !ok {
					err = NoMatch
					return
				}
			}
		}
	}
	return nil
}

type Parser interface {
	Parse(text string) error
}
