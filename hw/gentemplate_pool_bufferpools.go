// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=hw -id bufferPools -d PoolType=bufferPools -d Type=*BufferPool -d Data=elts github.com/platinasystems/elib/pool.tmpl]

package hw

import (
	"github.com/platinasystems/elib"
)

type bufferPools struct {
	elib.Pool
	elts []*BufferPool
}

func (p *bufferPools) GetIndex() (i uint) {
	l := uint(len(p.elts))
	i = p.Pool.GetIndex(l)
	if i >= l {
		p.Validate(i)
	}
	return i
}

func (p *bufferPools) PutIndex(i uint) (ok bool) {
	return p.Pool.PutIndex(i)
}

func (p *bufferPools) IsFree(i uint) (v bool) {
	v = i >= uint(len(p.elts))
	if !v {
		v = p.Pool.IsFree(i)
	}
	return
}

func (p *bufferPools) Resize(n uint) {
	c := elib.Index(cap(p.elts))
	l := elib.Index(len(p.elts) + int(n))
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]*BufferPool, l, c)
		copy(q, p.elts)
		p.elts = q
	}
	p.elts = p.elts[:l]
}

func (p *bufferPools) Validate(i uint) {
	c := elib.Index(cap(p.elts))
	l := elib.Index(i) + 1
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]*BufferPool, l, c)
		copy(q, p.elts)
		p.elts = q
	}
	if l > elib.Index(len(p.elts)) {
		p.elts = p.elts[:l]
	}
}

func (p *bufferPools) Elts() uint {
	return uint(len(p.elts)) - p.FreeLen()
}

func (p *bufferPools) Len() uint {
	return uint(len(p.elts))
}

func (p *bufferPools) Foreach(f func(x *BufferPool)) {
	for i := range p.elts {
		if !p.Pool.IsFree(uint(i)) {
			f(p.elts[i])
		}
	}
}
