package elib

type pool struct {
	// Vector of free indices
	freeIndices []uint32 // Uint32Vec
	// Bitmap of free indices
	freeBitmap Bitmap
}

// Get first free pool index if available.
func (p *pool) getIndex(max uint) (i uint) {
	i = max
	l := uint(len(p.freeIndices))
	if l != 0 {
		i = uint(p.freeIndices[l-1])
		p.freeIndices = p.freeIndices[:l-1]
		p.freeBitmap = p.freeBitmap.AndNotx(i)
	}
	return
}

// Put (free) given pool index.
func (p *pool) putIndex(i uint) (ok bool) {
	if ok = !p.freeBitmap.Get(i); ok {
		p.freeIndices = append(p.freeIndices, uint32(i))
		p.freeBitmap = p.freeBitmap.Orx(i)
	}
	return
}
