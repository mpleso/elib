// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=loop -id error -d VecType=errVec -d Type=err github.com/platinasystems/elib/vec.tmpl]

package loop

import (
	"github.com/platinasystems/elib"
)

type errVec []err

func (p *errVec) Resize(n uint) {
	c := elib.Index(cap(*p))
	l := elib.Index(len(*p)) + elib.Index(n)
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]err, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *errVec) Validate(i uint) *err {
	c := elib.Index(cap(*p))
	l := elib.Index(i) + 1
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]err, l, c)
		copy(q, *p)
		*p = q
	}
	if l > elib.Index(len(*p)) {
		*p = (*p)[:l]
	}
	return &(*p)[i]
}

func (p errVec) Len() uint { return uint(len(p)) }
