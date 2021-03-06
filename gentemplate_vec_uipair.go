// Copyright 2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id uiPair -d VecType=uiPairVec -d Type=uiPair -tags debug vec.tmpl]

//+build debug

package elib

type uiPairVec []uiPair

func (p *uiPairVec) Resize(n uint) {
	c := Index(cap(*p))
	l := Index(len(*p)) + Index(n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]uiPair, l, c)
		copy(q, *p)
		*p = q
	}
	*p = (*p)[:l]
}

func (p *uiPairVec) Validate(i uint) {
	c := Index(cap(*p))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]uiPair, l, c)
		copy(q, *p)
		*p = q
	}
	if l > Index(len(*p)) {
		*p = (*p)[:l]
	}
}

func (p uiPairVec) Len() uint { return uint(len(p)) }
