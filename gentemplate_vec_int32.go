// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id Int32 -d Type=int32 vec.tmpl]

package elib

type Int32Vec []int32

func (p *Int32Vec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]int32, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *Int32Vec) Validate(i uint) {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]int32, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
}
