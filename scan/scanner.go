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
	-EOF:        "eof",
	-Ident:      "identifier",
	-Int:        "integer",
	-Float:      "float",
	-Char:       "char",
	-String:     "string",
	-RawString:  "raw string",
	-Comment:    "comment",
	-Whitespace: "whitespace",
}

func tokString(tok rune) string {
	if tok < 0 {
		return tokNames[-tok]
	}
	return string(tok)
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
	if s.saveIndex == 0 && len(s.savedTokens) == 0 {
		tok = EOF
	} else {
		tok = s.savedTokens[s.saveIndex].tok
	}
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

func (s *Scanner) peekWhite() (tok rune, nWhite uint) {
	for {
		tok = s.scanner.Peek()
		if tok < 0 || s.whitespace&(1<<uint(tok)) == 0 {
			break
		}
		nWhite++
		s.scanner.Next()
	}
	return
}

func (s *Scanner) Peek() (tok rune) {
	var nWhite uint
	if tok, nWhite = s.peekWhite(); nWhite > 0 {
		tok = Whitespace
	} else {
		tok = s.scanner.Peek()
		if tok < 0 && int(-tok) < len(tokMap) {
			tok = tokMap[-tok]
		}
	}
	return
}

func (s *Scanner) nextNonWhite() (tok rune, text string) {
	tok, text = s.Scan()
	for tok == Whitespace {
		tok, text = s.Scan()
	}
	return
}

func (s *Scanner) SkipWhite() { s.peekWhite() }

func (s *Scanner) next() (tok rune, text string) {
	var nWhite uint
	if tok, nWhite = s.peekWhite(); nWhite > 0 {
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
	if len(s.saves) > 0 {
		s.saveIndex++
	}
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

func (s *Scanner) UnexpectedError(tok rune, text string) (err error) {
	return fmt.Errorf("%s: expected %s found `%s'", s.Pos(), tokString(tok), text)
}

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
		if f == "%" {
			a := args[ai]
			ai++
			if p, ok := a.(Parser); ok {
				s.peekWhite()
				err = p.Parse(s)
				if err != nil {
					return
				}
			} else {
				err = fmt.Errorf("%s: %T does not implement Parser interface", s.Pos(), a)
				return
			}
		} else {
			tok, text := s.nextNonWhite()
			switch {
			case strings.IndexByte(f, '%') >= 0:
				switch tok {
				case Ident, Int, Float, String:
					break
				default:
					err = s.UnexpectedError(Ident, text)
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
	}
	return nil
}

type Parser interface {
	Parse(input *Scanner) error
}
