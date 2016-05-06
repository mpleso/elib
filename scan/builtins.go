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

type Bitmap elib.Bitmap

func (b *Bitmap) String() string { return elib.Bitmap(*b).String() }

func (b *Bitmap) Parse(s *Scanner) error {
	r := elib.Bitmap(0)

	sep := rune(0)
	last := uint(0)
	for {
		tok, text := s.Scan()

		if tok != Int {
			return s.UnexpectedError(Int, text)
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
