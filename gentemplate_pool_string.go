// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id String -d PoolType=StringPool -d Type=string -d Data=Strings pool.tmpl]

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

func (p *StringPool) IsFree(i uint) (ok bool) {
	return p.Pool.IsFree(i)
}

func (p *StringPool) Resize(n int) {
	c := Index(cap(p.Strings))
	l := Index(len(p.Strings) + n)
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

func (p *StringPool) Elts() int {
	return len(p.Strings) - p.FreeLen()
}
