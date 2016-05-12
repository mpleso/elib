// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id freeElt -d VecType=freeEltVec -d Type=freeElt vec.tmpl]

package elib

type freeEltVec []freeElt

func (p *freeEltVec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]freeElt, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *freeEltVec) Validate(i uint) {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]freeElt, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
}

func (p freeEltVec) Len() uint { return uint(len(p)) }
