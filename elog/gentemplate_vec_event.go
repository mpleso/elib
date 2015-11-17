// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elog -id Event -d Type=Event github.com/platinasystems/elib/vec.tmpl]

package elog

import (
	. "github.com/platinasystems/elib"
)

type EventVec []Event

func (p *EventVec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]Event, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *EventVec) Validate(i uint) {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]Event, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
}
