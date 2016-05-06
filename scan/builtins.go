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

	i := 0
	for {
		tok, text := s.Scan()

		if tok != Int {
			return s.UnexpectedError(Int, text)
		}

		v, err := strconv.ParseUint(text, 0, 0)
		if err != nil {
			return err
		}

		r = r.Set(uint(v))

		i++
		if tok = s.Peek(); tok == EOF {
			break
		}
		if tok != ',' {
			return fmt.Errorf("%s: expected , after %s; got %s", s.Pos(), text, tokString(tok))
		}
		// Skip ,
		s.Scan()
	}

	*b = Bitmap(r)
	return nil
}
