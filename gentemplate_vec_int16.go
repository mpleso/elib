// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id Int16 -d VecType=Int16Vec -d Type=int16 vec.tmpl]

package elib

type Int16Vec []int16

func (p *Int16Vec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]int16, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *Int16Vec) Validate(i uint) {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]int16, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
}
