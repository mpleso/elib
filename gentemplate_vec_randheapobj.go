// Copyright 2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id randHeapObj -d VecType=randHeapObjVec -tags debug -d Type=randHeapObj vec.tmpl]

//+build debug

package elib

type randHeapObjVec []randHeapObj

func (p *randHeapObjVec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]randHeapObj, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *randHeapObjVec) Validate(i uint) {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]randHeapObj, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
}

func (p randHeapObjVec) Len() uint { return uint(len(p)) }
