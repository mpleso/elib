// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=mctree -id pair_offset -d VecType=pair_offset_vec -d Type=pair_offset github.com/platinasystems/elib/vec.tmpl]

package mctree

import (
	"github.com/platinasystems/elib"
)

type pair_offset_vec []pair_offset

func (p *pair_offset_vec) Resize(n uint) {
	c := elib.Index(cap(*p))
	l := elib.Index(len(*p)) + elib.Index(n)
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]pair_offset, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *pair_offset_vec) validate(i uint, zero *pair_offset) *pair_offset {
	c := elib.Index(cap(*p))
	l := elib.Index(i) + 1
	if l > c {
		cNext := elib.NextResizeCap(l)
		q := make([]pair_offset, cNext, cNext)
		copy(q, *p)
		if zero != nil {
			for i := c; i < cNext; i++ {
				q[i] = *zero
			}
		}
		*p = q[:l]
	}
	if l > elib.Index(len(*p)) {
		*p = (*p)[:l]
	}
	return &(*p)[i]
}
func (p *pair_offset_vec) Validate(i uint) *pair_offset { return p.validate(i, (*pair_offset)(nil)) }
func (p *pair_offset_vec) ValidateInit(i uint, zero pair_offset) *pair_offset {
	return p.validate(i, &zero)
}

func (p pair_offset_vec) Len() uint { return uint(len(p)) }
