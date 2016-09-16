// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id Int8 -d VecType=Int8Vec -d Type=int8 vec.tmpl]

package elib

type Int8Vec []int8

func (p *Int8Vec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]int8, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *Int8Vec) validate(new_len uint, zero *int8) *int8 {
	c := Index(cap(*p))
	lʹ := Index(len(*p))
	l := Index(new_len)
	if l <= c {
		// Need to reslice to larger length?
		if l >= lʹ {
			*p = (*p)[:l]
		}
		return &(*p)[l-1]
	}
	return p.validateSlowPath(zero, c, l, lʹ)
}

func (p *Int8Vec) validateSlowPath(zero *int8,
	c, l, lʹ Index) *int8 {
	if l > c {
		cNext := NextResizeCap(l)
		q := make([]int8, cNext, cNext)
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

func (p *Int8Vec) Validate(i uint) *int8 {
	return p.validate(i+1, (*int8)(nil))
}

func (p *Int8Vec) ValidateInit(i uint, zero int8) *int8 {
	return p.validate(i+1, &zero)
}

func (p *Int8Vec) ValidateLen(l uint) (v *int8) {
	if l > 0 {
		v = p.validate(l, (*int8)(nil))
	}
	return
}

func (p *Int8Vec) ValidateLenInit(l uint, zero int8) (v *int8) {
	if l > 0 {
		v = p.validate(l, &zero)
	}
	return
}

func (p Int8Vec) Len() uint { return uint(len(p)) }
