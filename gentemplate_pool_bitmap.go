// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elib -id Bitmap -d Type=[]BitmapVec -d Data=bitmaps pool.tmpl]

package elib

type BitmapPool struct {
	Pool
	bitmaps []BitmapVec
}

func (p *BitmapPool) GetIndex() (i uint) {
	l := uint(len(p.bitmaps))
	i = p.Pool.GetIndex(l)
	if i >= l {
		p.Validate(i)
	}
	return i
}

func (p *BitmapPool) PutIndex(i uint) (ok bool) {
	return p.Pool.PutIndex(i)
}

func (p *BitmapPool) IsFree(i uint) (ok bool) {
	return p.Pool.IsFree(i)
}

func (p *BitmapPool) Resize(n int) {
	c := Index(cap(p.bitmaps))
	l := Index(len(p.bitmaps) + n)
	if l > c {
		c = NextResizeCap(l)
		q := make([]BitmapVec, l, c)
		copy(q, p.bitmaps)
		p.bitmaps = q
	}
	p.bitmaps = p.bitmaps[:l]
}

func (p *BitmapPool) Validate(i uint) {
	c := Index(cap(p.bitmaps))
	l := Index(i) + 1
	if l > c {
		c = NextResizeCap(l)
		q := make([]BitmapVec, l, c)
		copy(q, p.bitmaps)
		p.bitmaps = q
	}
	if l > Index(len(p.bitmaps)) {
		p.bitmaps = p.bitmaps[:l]
	}
}

func (p *BitmapPool) Elts() int {
	return len(p.bitmaps) - p.FreeLen()
}
