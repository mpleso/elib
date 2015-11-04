package elib

import (
	"fmt"
)

func Stringer(n []string, i int) string {
	if i < len(n) && len(n[i]) > 0 {
		return n[i]
	} else {
		return fmt.Sprintf("%d", i)
	}
}

func FlagStringer(n []string, x Word) (s string) {
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
			s += fmt.Sprintf("%d", i)
		}
		x ^= f
	}
	return
}
