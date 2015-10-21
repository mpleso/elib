package elib

import (
	"flag"
	"fmt"
	"math/rand"
	"strconv"
	"time"
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

func (p *Heap) Get(size uint) (id Index, index uint) {
	// Keep track of largest size caller asks for.
	if Index(size) > p.maxSize {
		p.maxSize = Index(size)
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

	if p.len == 0 {
		p.head = 0
		p.tail = MaxIndex
	}

	ei := p.newElt()
	e := &p.elts[ei]

	index = uint(p.len)
	p.len += Index(size)
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

type visited uint8

const (
	unvisited visited = iota
	visitedAlloc
	visitedFree
	removed
)

var visitedElts []visited

func (p *Heap) validate() (err error) {
	if cap(visitedElts) < len(p.elts) {
		visitedElts = make([]visited, len(p.elts))
	}
	nVisited := 0
	visitedElts = visitedElts[:len(p.elts)]
	for i := range visitedElts {
		visitedElts[i] = unvisited
	}

	ei := p.head
	prev := MaxIndex
	index := Index(0)
	for ei != MaxIndex {
		e := &p.elts[ei]

		if visitedElts[ei] != unvisited {
			err = fmt.Errorf("duplicate visit %+v", e)
			return
		}

		if visitedElts[ei] != unvisited {
			err = fmt.Errorf("already visited %+v", e)
			return
		}

		if e.prev != prev {
			err = fmt.Errorf("bad prev pointer %+v", e)
			return
		}

		if e.index != index {
			err = fmt.Errorf("bad index %d %+v", index, e)
			return
		}

		size := p.size(ei)
		if size == 0 {
			err = fmt.Errorf("zero size %d %+v", ei, e)
			return
		}
		index += size

		visitedElts[ei] = visitedAlloc
		nVisited++

		if e.isFree() {
			visitedElts[ei] = visitedFree
			if size > p.maxSize {
				size = 0
			}
			if size >= Index(len(p.free)) {
				err = fmt.Errorf("size %d >= len free %d", size, len(p.free))
				return
			}
			if e.free >= Index(len(p.free[size])) {
				err = fmt.Errorf("free %d >= len free[%d] %d, size %d %+v", e.free, size, len(p.free[size]), p.size(ei), e)
				return
			}
			if ei != p.free[size][e.free].elt {
				err = fmt.Errorf("corrupt free list %d != free[%d][%d] %d", ei, size, e.free, p.free[size][e.free])
				return
			}
		}

		prev = ei
		ei = e.next
	}
	if prev != p.tail {
		err = fmt.Errorf("corrupt tail %d != %d", prev, p.tail)
		return
	}

	for i := range p.removed {
		ei := p.removed[i]
		e := &p.elts[ei]
		if visitedElts[ei] != unvisited {
			err = fmt.Errorf("removed visited %+v", e)
			return
		}
		visitedElts[ei] = removed
		nVisited++
	}

	for ei := range p.elts {
		if visitedElts[ei] == unvisited {
			err = fmt.Errorf("unvisited elt %d", ei)
			return
		}
	}

	// Make sure all elts have been visited
	if nVisited != len(visitedElts) {
		err = fmt.Errorf("visited %d != %d", nVisited, len(visitedElts))
		return
	}

	return
}

func (p *Heap) String() string {
	return fmt.Sprintf("%d elts", len(p.elts))
}

// IterInt implements the Value interface so flags can be specified as
// either integer (1000000) or better with floating point (1e6).
type IterInt int

func (t *IterInt) Set(s string) (err error) {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		v, e := strconv.ParseFloat(s, 64)
		*t = IterInt(v)
		if e == nil {
			err = nil
			return
		}
	}
	*t = IterInt(v)
	return
}

func (t *IterInt) String() string { return fmt.Sprintf("%v", *t) }

type testHeap struct {
	// Number of iterations to run
	iterations IterInt

	// Validate/print every so many iterations (zero means never).
	validateEvery IterInt
	printEvery    IterInt

	// Seed to make randomness deterministic.  0 means choose seed.
	seed int64

	// Number of objects to create.
	nObjects IterInt

	// log2 max size of objects.
	log2MaxLen int

	verbose int
}

func HeapTest() {
	t := testHeap{
		iterations: 10,
		nObjects:   10,
		verbose:    1,
	}
	flag.Var(&t.iterations, "iter", "Number of iterations")
	flag.Var(&t.validateEvery, "valid", "Number of iterations per validate")
	flag.Var(&t.printEvery, "print", "Number of iterations per print")
	flag.Int64Var(&t.seed, "seed", 0, "Seed for random number generator")
	flag.IntVar(&t.log2MaxLen, "len", 8, "Log2 max length of object to allocate")
	flag.Var(&t.nObjects, "objects", "Number of random objects")
	flag.IntVar(&t.verbose, "verbose", 0, "Be verbose")
	flag.Parse()

	err := runHeapTest(&t)
	if err != nil {
		panic(err)
	}
}

type randHeapObj struct {
	id   Index
	n, i uint
}

//go:generate gentemplate -d Package=elib -id randHeapObj -d Type=randHeapObj vec.tmpl

func runHeapTest(t *testHeap) (err error) {
	var p Heap
	var s Uint64Vec
	var objs randHeapObjVec

	if t.seed == 0 {
		t.seed = int64(time.Now().Nanosecond())
	}

	rand.Seed(t.seed)
	if t.verbose != 0 {
		fmt.Printf("%#v\n", t)
	}
	objs.Resize(uint(t.nObjects))
	var iter int

	validate := func() (err error) {
		if t.validateEvery != 0 && iter%int(t.validateEvery) == 0 {
			if err = p.validate(); err != nil {
				if t.verbose != 0 {
					fmt.Printf("iter %d: %s\n%+v\n", iter, err, p)
				}
				return
			}
		}
		if t.printEvery != 0 && iter%int(t.printEvery) == 0 {
			fmt.Printf("%10g iterations: %s\n", float64(iter), &p)
		}
		return
	}

	for iter = 0; iter < int(t.iterations); iter++ {
		o := &objs[rand.Int()%len(objs)]
		if o.n != 0 {
			if l := p.Len(o.id); l != o.n {
				err = fmt.Errorf("len mismatch %d != %d", l, o.n)
				return
			}
			err = p.Put(o.id)
			if err != nil {
				return
			}
			o.n = 0
		} else {
			o.n = 1 + uint(rand.Int()&(1<<uint(t.log2MaxLen)-1))
			o.id, o.i = p.Get(o.n)
			s.Validate(o.i + o.n - 1)
			for j := uint(0); j < o.n; j++ {
				s[o.i+j] = uint64(o.id)<<uint(t.log2MaxLen) + uint64(o.i+j)
			}
		}
		err = validate()
		if err != nil {
			return
		}
	}
	if t.verbose != 0 {
		fmt.Printf("%d iterations: %+v\n", iter, p)
	}
	for i := range objs {
		o := &objs[i]
		if o.n > 0 {
			p.Put(o.id)
		}
		err = validate()
		if err != nil {
			return
		}
		iter++
	}
	if t.verbose != 0 {
		fmt.Printf("%d iterations: %+v\n", iter, p)
		fmt.Printf("No errors: %d iterations\n", t.iterations)
	}
	return
}
