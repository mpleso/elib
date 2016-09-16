// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=hw -id Ref -d VecType=RefVec -d Type=Ref github.com/platinasystems/elib/vec.tmpl]

package hw

import (
	"github.com/platinasystems/elib"
)

type RefVec []Ref

func (p *RefVec) Resize(n uint) {
	c := elib.Index(cap(*p))
	l := elib.Index(len(*p)) + elib.Index(n)
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]Ref, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *RefVec) validate(new_len uint, zero *Ref) *Ref {
	c := elib.Index(cap(*p))
	lʹ := elib.Index(len(*p))
	l := elib.Index(new_len)
	if l <= c {
		// Need to reslice to larger length?
		if l >= lʹ {
			*p = (*p)[:l]
		}
		return &(*p)[l-1]
	}
	return p.validateSlowPath(zero, c, l, lʹ)
}

func (p *RefVec) validateSlowPath(zero *Ref,
	c, l, lʹ elib.Index) *Ref {
	if l > c {
		cNext := elib.NextResizeCap(l)
		q := make([]Ref, cNext, cNext)
		copy(q, *p)
		if zero != nil {
			for i := c; i < cNext; i++ {
				q[i] = *zero
			}
		}
		*p = q[:l]
	}
	if l > lʹ {
		*p = (*p)[:l]
	}
	return &(*p)[l-1]
}

func (p *RefVec) Validate(i uint) *Ref {
	return p.validate(i+1, (*Ref)(nil))
}

func (p *RefVec) ValidateInit(i uint, zero Ref) *Ref {
	return p.validate(i+1, &zero)
}

func (p *RefVec) ValidateLen(l uint) (v *Ref) {
	if l > 0 {
		v = p.validate(l, (*Ref)(nil))
	}
	return
}

func (p *RefVec) ValidateLenInit(l uint, zero Ref) (v *Ref) {
	if l > 0 {
		v = p.validate(l, &zero)
	}
	return
}

func (p RefVec) Len() uint { return uint(len(p)) }
