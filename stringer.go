package elib

import (
	"fmt"
)

func StringerWithFormat(n []string, i int, unknownFormat string) string {
	if i < len(n) && len(n[i]) > 0 {
		return n[i]
	} else {
		return fmt.Sprintf(unknownFormat, i)
	}
}

func Stringer(n []string, i int) string { return StringerWithFormat(n, i, "%d") }

func FlagStringerWithFormat(n []string, x Word, unknownFormat string) (s string) {
	s = ""
	for x != 0 {
		f := FirstSet(x)
		if len(s) > 0 {
			s += ", "
		}
		i := int(MinLog2(f))
		if i < len(n) && len(n[i]) > 0 {
			s += n[i]
		} else {
			s += fmt.Sprintf(unknownFormat, i)
		}
		x ^= f
	}
	return
}

func FlagStringer(n []string, x Word) string { return FlagStringerWithFormat(n, x, "%d") }
