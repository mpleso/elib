package scan

// Boolean parser accepting yes/no 0/1
type Bool bool

func (b *Bool) Parse(text string) (err error) {
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

func (b *Enable) Parse(text string) (err error) {
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
