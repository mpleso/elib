package scan

import (
	"github.com/platinasystems/elib"

	"fmt"
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

// Boolean parser accepting up/down yes/no
type UpDown bool

func (b *UpDown) Parse(s *Scanner) (err error) {
	_, text := s.Scan()
	switch text {
	case "up", "yes", "1":
		*b = true
	case "down", "no", "0":
		*b = false
	default:
		err = NoMatch
	}
	return
}

func parseUint(s *Scanner, base, bitsize int) (v uint64, err error) {
	tok, text := s.Scan()
	if tok == Int || (base > 10 && tok == Ident) {
		v, err = strconv.ParseUint(text, base, bitsize)
	} else {
		err = s.UnexpectedError(Int, tok, text)
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

type Base16Uint8 uint8
type Base16Uint16 uint16
type Base16Uint32 uint32
type Base16Uint64 uint64

func (x *Base16Uint8) Parse(s *Scanner) error {
	v, err := parseUint(s, 16, 8)
	if err == nil {
		*x = Base16Uint8(v)
	}
	return err
}

func (x *Base16Uint16) Parse(s *Scanner) error {
	v, err := parseUint(s, 16, 16)
	if err == nil {
		*x = Base16Uint16(v)
	}
	return err
}

func (x *Base16Uint32) Parse(s *Scanner) error {
	v, err := parseUint(s, 16, 32)
	if err == nil {
		*x = Base16Uint32(v)
	}
	return err
}

func (x *Base16Uint64) Parse(s *Scanner) error {
	v, err := parseUint(s, 16, 64)
	if err == nil {
		*x = Base16Uint64(v)
	}
	return err
}

type Float64 float64

func (x *Float64) Parse(s *Scanner) (err error) {
	tok, text := s.Scan()
	if tok != Int {
		err = s.UnexpectedError(Int, tok, text)
		return
	}
	if s.AdvanceIf('.') {
		tok, subtext := s.Scan()
		if tok != Int {
			err = s.UnexpectedError(Int, tok, text)
			return
		}
		text += "." + subtext
	}
	if tok = s.Peek(); tok == 'e' || tok == 'E' {
		// Skip past - or +
		if tok = s.Peek(); tok == '-' || tok == '+' {
			s.Next()
		}
		tok, subtext := s.Scan()
		if tok != Int {
			err = s.UnexpectedError(Int, tok, text)
			return
		}
		text += "e" + subtext
	}

	var v float64
	v, err = strconv.ParseFloat(text, 64)
	*x = Float64(v)

	return
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

type StringMap map[string]uint

func (sm *StringMap) Set(v string, i uint) {
	m := *sm
	if m == nil {
		m = make(map[string]uint)
	}
	m[v] = i
	*sm = m
}

func NewStringMap(a []string) (m StringMap) {
	m = make(map[string]uint)
	for i := range a {
		if len(a[i]) > 0 {
			m[a[i]] = uint(i)
		}
	}
	return m
}

func (m StringMap) Parse(s *Scanner) (v uint, err error) {
	_, text := s.Next()
	var ok bool
	if v, ok = m[text]; !ok {
		err = fmt.Errorf("%s", text)
	}
	return
}
