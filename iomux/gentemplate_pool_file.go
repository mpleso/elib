// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=iomux -id file -d Data=files -d Type=[]Interface github.com/platinasystems/elib/pool.tmpl]

package iomux

import (
	. "github.com/platinasystems/elib"
)

type filePool struct {
	Pool  Pool
	files []Interface
}

func (p *filePool) GetIndex() (i uint) {
	l := uint(len(p.files))
	i = p.Pool.GetIndex(l)
	if i >= l {
		p.Validate(i)
	}
	return i
}

func (p *filePool) PutIndex(i uint) (ok bool) {
	return p.Pool.PutIndex(i)
}

func (p *filePool) Resize(n int) {
	c := Index(cap(p.files))
	l := Index(len(p.files) + n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]Interface, l, c)
		copy(q, p.files)
		p.files = q
	}
	p.files = p.files[:l]
}

func (p *filePool) Validate(i uint) {
	c := Index(cap(p.files))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]Interface, l, c)
		copy(q, p.files)
		p.files = q
	}
	if l > Index(len(p.files)) {
		p.files = p.files[:l]
	}
}
