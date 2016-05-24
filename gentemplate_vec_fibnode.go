// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id fibNode -d VecType=fibNodeVec -d Type=fibNode vec.tmpl]

package elib

type fibNodeVec []fibNode

func (p *fibNodeVec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]fibNode, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *fibNodeVec) Validate(i uint) *fibNode {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]fibNode, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
	return &(*p)[i]
}

func (p fibNodeVec) Len() uint { return uint(len(p)) }
