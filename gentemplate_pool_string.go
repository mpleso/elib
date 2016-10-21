// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id String -d PoolType=StringPool -d Type=string -d Data=Strings pool.tmpl]

// Copyright 2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elib

type StringPool struct {
	Pool
	Strings []string
}

func (p *StringPool) GetIndex() (i uint) {
	l := uint(len(p.Strings))
	i = p.Pool.GetIndex(l)
	if i >= l {
		p.Validate(i)
	}
	return i
}

func (p *StringPool) PutIndex(i uint) (ok bool) {
	return p.Pool.PutIndex(i)
}

func (p *StringPool) IsFree(i uint) (v bool) {
	v = i >= uint(len(p.Strings))
	if !v {
		v = p.Pool.IsFree(i)
	}
	return
}

func (p *StringPool) Resize(n uint) {
	c := Index(cap(p.Strings))
	l := Index(len(p.Strings) + int(n))
	if l > c {
		c = NextResizeCap(l)
		q := make([]string, l, c)
		copy(q, p.Strings)
		p.Strings = q
	}
	p.Strings = p.Strings[:l]
}

func (p *StringPool) Validate(i uint) {
	c := Index(cap(p.Strings))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]string, l, c)
		copy(q, p.Strings)
		p.Strings = q
	}
	if l > Index(len(p.Strings)) {
		p.Strings = p.Strings[:l]
	}
}

func (p *StringPool) Elts() uint {
	return uint(len(p.Strings)) - p.FreeLen()
}

func (p *StringPool) Len() uint {
	return uint(len(p.Strings))
}

func (p *StringPool) Foreach(f func(x string)) {
	for i := range p.Strings {
		if !p.Pool.IsFree(uint(i)) {
			f(p.Strings[i])
		}
	}
}

func (p *StringPool) ForeachIndex(f func(i uint)) {
	for i := range p.Strings {
		if !p.Pool.IsFree(uint(i)) {
			f(uint(i))
		}
	}
}
