package elib

import (
	"fmt"
)

// Index gives common type for indices in Heaps, Pools, Fifos, ...
type Index uint32

const MaxIndex Index = ^Index(0)

// A Heap maintains an allocator for arbitrary sized blocks of an underlying array.
// The array is not part of the Heap.
type Heap struct {
	elts []heapElt

	// Slices of free elts indices indexed by size.
	// "Size" 0 is for large sized chunks.
	free freeEltsVec

	removed []Index

	head, tail Index

	// Total number of indices allocated
	len Index

	// Largest size ever allocated
	maxSize Index

	maxLen Index
}

type freeElt struct {
	elt Index
}

//go:generate gentemplate -d Package=elib -id freeElt -d Type=freeElt vec.tmpl
//go:generate gentemplate -d Package=elib -id freeElts -d Type=freeEltVec vec.tmpl

type heapElt struct {
	// Index of this element
	index Index

	// Index on free list for this size or ^uint32(0) if not free
	free Index

	// Index of next and previous elements
	next, prev Index
}

func (e *heapElt) isFree() bool {
	return e.free != MaxIndex
}

func (p *Heap) freeAfter(ei, size, d Index) {
	fi := p.newElt()
	e, f := &p.elts[ei], &p.elts[fi]
	f.index = e.index + Index(size-d)
	f.next = e.next
	f.prev = ei
	if f.next != MaxIndex {
		n := &p.elts[f.next]
		n.prev = fi
	}
	e.next = fi
	if ei == p.tail {
		p.tail = fi
	}
	p.freeElt(fi, d)
}

func (p *Heap) freeElt(ei, size Index) {
	if size > p.maxSize {
		size = 0
	}
	p.free.Validate(uint(size))
	p.elts[ei].free = Index(len(p.free[size]))
	p.free[size] = append(p.free[size], freeElt{ei})
}

var poison heapElt = heapElt{
	index: MaxIndex,
	free:  MaxIndex,
	next:  MaxIndex,
	prev:  MaxIndex,
}

func (p *Heap) removeFreeElt(ei, size Index) {
	e := &p.elts[ei]
	fi := e.free
	if size >= Index(len(p.free)) {
		size = 0
	}
	if l := Index(len(p.free[size])); fi < l && p.free[size][fi].elt == ei {
		if fi < l-1 {
			gi := p.free[size][l-1].elt
			p.free[size][fi].elt = gi
			p.elts[gi].free = fi
		}
		p.free[size] = p.free[size][:l-1]
		*e = poison
		p.removed = append(p.removed, ei)
		return
	}
	panic("corrupt free list")
}

func (p *Heap) size(ei Index) Index {
	e := &p.elts[ei]
	i := Index(p.len)
	if e.next != MaxIndex {
		i = p.elts[e.next].index
	}
	return i - e.index
}

// Recycle previously removed elts.
func (p *Heap) newElt() (ei Index) {
	if l := len(p.removed); l > 0 {
		ei = p.removed[l-1]
		p.removed = p.removed[:l-1]
		p.elts[ei] = poison
	} else {
		ei = Index(len(p.elts))
		p.elts = append(p.elts, poison)
	}
	return
}

func (p *Heap) Get(sizeArg uint) (id Index, index uint) {
	size := Index(sizeArg)

	// Keep track of largest size caller asks for.
	if size > p.maxSize {
		p.maxSize = size
	}

	if size <= 0 {
		panic("size")
	}

	if int(size) < len(p.free) {
		if l := len(p.free[size]); l > 0 {
			ei := p.free[size][l-1].elt
			e := &p.elts[ei]
			p.free[size] = p.free[size][:l-1]
			e.free = MaxIndex
			index = uint(e.index)
			id = ei
			return
		}
	}

	if len(p.free) > 0 {
		l := Index(len(p.free[0]))
		for fi := Index(0); fi < l; fi++ {
			ei := p.free[0][fi].elt
			e := &p.elts[ei]
			es := p.size(ei)
			fs := int(es) - int(size)
			if fs < 0 {
				continue
			}
			if fi < l-1 {
				gi := p.free[0][l-1].elt
				p.free[0][fi].elt = gi
				p.elts[gi].free = fi
			}
			p.free[0] = p.free[0][:l-1]

			index = uint(e.index)
			e.free = MaxIndex
			id = ei

			if fs > 0 {
				p.freeAfter(ei, es, Index(fs))
			}
			return
		}
	}

	if p.maxSize != 0 && p.len+size > p.maxSize {
		panic(fmt.Errorf("heap overflow allocating object of length %d", size))
	}

	if p.len == 0 {
		p.head = 0
		p.tail = MaxIndex
	}

	ei := p.newElt()
	e := &p.elts[ei]

	index = uint(p.len)
	p.len += size
	e.index = Index(index)

	e.next = MaxIndex
	e.prev = p.tail
	e.free = MaxIndex

	p.tail = ei

	if e.prev != MaxIndex {
		p.elts[e.prev].next = ei
	}

	id = ei
	return
}

func (p *Heap) Len(ei Index) uint {
	return uint(p.size(ei))
}

func (p *Heap) Put(ei Index) (err error) {
	e := &p.elts[ei]

	if e.isFree() {
		err = fmt.Errorf("duplicate free %d", ei)
		return
	}

	if e.prev != MaxIndex {
		prev := &p.elts[e.prev]
		if prev.isFree() {
			ps := e.index - prev.index
			e.index = prev.index
			pi := e.prev
			e.prev = prev.prev
			if e.prev != MaxIndex {
				p.elts[e.prev].next = ei
			}
			p.removeFreeElt(pi, ps)
			if pi == p.head {
				p.head = ei
			}
		}
	}

	if e.next != MaxIndex {
		next := &p.elts[e.next]
		if next.isFree() {
			ni := e.next
			ns := p.size(ni)
			e.next = next.next
			if e.next != MaxIndex {
				p.elts[e.next].prev = ei
			}
			p.removeFreeElt(ni, ns)
			if ni == p.tail {
				p.tail = ei
			}
		}
	}

	es := p.size(ei)
	p.freeElt(ei, es)

	return
}

func (p *Heap) String() string {
	return fmt.Sprintf("%d elts", len(p.elts))
}
