package scan

import (
	"fmt"
	"github.com/platinasystems/elib"
	"strconv"
)

// Boolean parser accepting yes/no 0/1
type Bool bool

func (b *Bool) Parse(s *Scanner) (err error) {
	_, text := s.Scan()
	switch text {
	case "true", "yes", "1":
		*b = true
	case "false", "no", "0":
		*b = false
	default:
		err = NoMatch
	}
	return
}

// Boolean parser accepting enable/disable yes/no
type Enable bool

func (b *Enable) Parse(s *Scanner) (err error) {
	_, text := s.Scan()
	switch text {
	case "enable", "yes", "1":
		*b = true
	case "disable", "no", "0":
		*b = false
	default:
		err = NoMatch
	}
	return
}

func parseUint(s *Scanner, base, bitsize int) (v uint64, err error) {
	tok, text := s.Scan()
	if tok != Int {
		err = s.UnexpectedError(Int, tok, text)
	} else {
		v, err = strconv.ParseUint(text, base, bitsize)
	}
	return
}

type Base10Uint8 uint8
type Base10Uint16 uint16
type Base10Uint32 uint32
type Base10Uint64 uint64

func (x *Base10Uint8) Parse(s *Scanner) error {
	v, err := parseUint(s, 10, 8)
	if err == nil {
		*x = Base10Uint8(v)
	}
	return err
}

func (x *Base10Uint16) Parse(s *Scanner) error {
	v, err := parseUint(s, 10, 16)
	if err == nil {
		*x = Base10Uint16(v)
	}
	return err
}

func (x *Base10Uint32) Parse(s *Scanner) error {
	v, err := parseUint(s, 10, 32)
	if err == nil {
		*x = Base10Uint32(v)
	}
	return err
}

func (x *Base10Uint64) Parse(s *Scanner) error {
	v, err := parseUint(s, 10, 64)
	if err == nil {
		*x = Base10Uint64(v)
	}
	return err
}

type Bitmap elib.Bitmap

func (b *Bitmap) String() string { return elib.Bitmap(*b).String() }

func (b *Bitmap) Parse(s *Scanner) error {
	r := elib.Bitmap(0)

	sep := rune(0)
	last := uint(0)
	for {
		tok, text := s.Scan()

		if tok != Int {
			return s.UnexpectedError(Int, tok, text)
		}

		v, err := strconv.ParseUint(text, 0, 0)
		if err != nil {
			return err
		}
		uv := uint(v)

		if sep == '-' {
			if last > uv {
				return fmt.Errorf("%s: expected lo %d > hi %d in range", s.Pos(), last, uv)
			}
			for i := last; i <= uv; i++ {
				r = r.Set(i)
			}
			last = uint(0)
		} else {
			r = r.Set(uv)
			last = uv
		}

		tok = s.Peek()
		switch tok {
		case EOF, Whitespace:
			*b = Bitmap(r)
			return nil
		case ',', '-':
			sep, _ = s.Scan()
			if tok == '-' && last == 0 {
				return fmt.Errorf("%s: expected , after range; got -", s.Pos())
			}
		default:
			return fmt.Errorf("%s: expected , after %s; got %s", s.Pos(), text, tokString(tok))
		}
	}
}

type EltParser interface {
	ParseElt(input *Scanner, i uint) error
}

func (s *Scanner) Expect(want string, skipWhite bool) (err error) {
	if skipWhite {
		s.SkipWhite()
	}
	_, text := s.Scan()
	if text != want {
		err = s.UnexpectedText(want, text)
	}
	return
}

type ParseEltsConfig struct {
	Start, End       string
	Sep              rune
	MinElts, MaxElts uint
	SkipWhite        bool
}

func (s *Scanner) ParseElts(p EltParser, c *ParseEltsConfig) (err error) {
	save := s.Save()
	defer s.ErrRestore(err, save)

	if len(c.Start) > 0 {
		if err = s.Expect(c.Start, c.SkipWhite); err != nil {
			return
		}
	}

	n := uint(0)
	for {
		if c.SkipWhite {
			s.SkipWhite()
		}
		if c.MaxElts != 0 && n+1 > c.MaxElts {
			err = fmt.Errorf("%s: expected at most %d elements", s.Pos(), c.MaxElts)
			return
		}
		if err = p.ParseElt(s, n); err != nil {
			return
		}
		n++
		if c.SkipWhite {
			s.SkipWhite()
		}
		if tok := s.Peek(); tok != c.Sep {
			break
		}
		s.Scan()
	}

	if n < c.MinElts {
		err = fmt.Errorf("%s: expected at least %d elements; found %d", s.Pos(), c.MinElts, n)
		return
	}

	if len(c.End) > 0 {
		if err = s.Expect(c.End, c.SkipWhite); err != nil {
			return
		}
	}

	return
}

type StringMap map[string]int

func (m StringMap) Parse(s *Scanner) (v int, err error) {
	_, text := s.Next()
	var ok bool
	if v, ok = m[text]; !ok {
		err = fmt.Errorf("%s", text)
	}
	return
}
