package scan

import (
	"errors"
	"fmt"
	"strings"
)

var NoMatch = errors.New("no match")

func tokTextString(tok rune, text string) (v string) {
	v = tokString(tok)
	if len(text) > 0 {
		v += " `" + text + "'"
	}
	return
}

func (s *Scanner) UnexpectedError(want, got rune, gotText string) (err error) {
	return fmt.Errorf("%s: expected %s found %s", s.Pos(), tokString(want), tokTextString(got, gotText))
}
func (s *Scanner) UnexpectedText(want, got string) (err error) {
	return fmt.Errorf("%s: expected `%s' found `%s'", s.Pos(), want, got)
}

type Parser interface {
	Parse(input *Scanner) error
}

func (s *Scanner) ErrRestore(err error, save int) {
	if err != nil {
		s.Restore(save)
	} else {
		s.Advance()
	}
}

func (s *Scanner) ParseFormat(format string, args ...Parser) (err error) {
	v := s.Save()
	defer s.ErrRestore(err, v)
	i := 0
	for _, f := range format {
		switch f {
		case ' ':
			s.SkipWhite()
		case '%':
			if err = args[i].Parse(s); err != nil {
				return
			}
			i++
		default:
			if tok, text := s.Scan(); tok != f {
				err = s.UnexpectedError(f, tok, text)
				return
			}
		}
	}
	return
}

func (s *Scanner) Parse(template string, args ...interface{}) (err error) {
	fs := strings.Fields(template)
	ai := 0
	v := s.Save()
	defer s.ErrRestore(err, v)
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
					err = s.UnexpectedError(Ident, tok, text)
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
