package parse

// Boolean parser accepting yes/no 0/1
type Bool bool

func (b *Bool) Parse(in *Input, args *Args) {
	switch text := in.Token(); text {
	case "true", "yes", "1":
		*b = true
	case "false", "no", "0":
		*b = false
	default:
		panic(ErrInput)
	}
	return
}

// Boolean parser accepting enable/disable yes/no
type Enable bool

func (b *Enable) Parse(in *Input, args *Args) {
	switch text := in.Token(); text {
	case "enable", "yes", "1":
		*b = true
	case "disable", "no", "0":
		*b = false
	default:
		panic(ErrInput)
	}
	return
}

// Boolean parser accepting up/down yes/no
type UpDown bool

func (b *UpDown) Parse(in *Input, args *Args) {
	switch text := in.Token(); text {
	case "up", "yes", "1":
		*b = true
	case "down", "no", "0":
		*b = false
	default:
		panic(ErrInput)
	}
	return
}

type StringMap map[string]uint

func (sm *StringMap) Set(v string, i uint) {
	m := *sm
	if m == nil {
		m = make(StringMap)
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

func (m StringMap) ParseWithArgs(in *Input, args *Args) {
	text := in.Token()
	if v, ok := m[text]; ok {
		args.SetNextInt(uint64(v))
	} else {
		panic(ErrInput)
	}
	return
}
