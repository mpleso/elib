// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=cli -id file -d Data=Files -d PoolType=FilePool -d Type=File github.com/platinasystems/elib/pool.tmpl]

package cli

import (
	"github.com/platinasystems/elib"
)

type FilePool struct {
	elib.Pool
	Files []File
}

func (p *FilePool) GetIndex() (i uint) {
	l := uint(len(p.Files))
	i = p.Pool.GetIndex(l)
	if i >= l {
		p.Validate(i)
	}
	return i
}

func (p *FilePool) PutIndex(i uint) (ok bool) {
	return p.Pool.PutIndex(i)
}

func (p *FilePool) IsFree(i uint) (ok bool) {
	return p.Pool.IsFree(i)
}

func (p *FilePool) Resize(n uint) {
	c := elib.Index(cap(p.Files))
	l := elib.Index(len(p.Files) + int(n))
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]File, l, c)
		copy(q, p.Files)
		p.Files = q
	}
	p.Files = p.Files[:l]
}

func (p *FilePool) Validate(i uint) {
	c := elib.Index(cap(p.Files))
	l := elib.Index(i) + 1
	if l > c {
		c = elib.NextResizeCap(l)
		q := make([]File, l, c)
		copy(q, p.Files)
		p.Files = q
	}
	if l > elib.Index(len(p.Files)) {
		p.Files = p.Files[:l]
	}
}

func (p *FilePool) Elts() uint {
	return uint(len(p.Files)) - p.FreeLen()
}

func (p *FilePool) Len() uint {
	return uint(len(p.Files))
}
