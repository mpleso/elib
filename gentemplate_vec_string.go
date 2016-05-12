// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id String -d VecType=StringVec -d Type=string vec.tmpl]

package elib

type StringVec []string

func (p *StringVec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]string, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *StringVec) Validate(i uint) {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]string, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
}

func (p StringVec) Len() uint { return uint(len(p)) }
