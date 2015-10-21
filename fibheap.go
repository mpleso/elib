package elib

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime/pprof"
	"time"
)

type fibNode struct {
	// Index of parent or MaxIndex if no parent.
	// Having no parent means this node is a root node.
	sup Index

	// Links to doubly linked list of siblings
	next, prev Index

	// Index of first child node or MaxIndex if no children.
	sub Index

	// Number of children.
	nSub uint16

	// Set when at least one child has been cut since this node was made child of another node.
	// Roots are never marked.
	isMarked bool
}

//go:generate gentemplate -d Package=elib -id fibNode -d Type=fibNode vec.tmpl

const (
	fibRootIndex = MaxIndex - 1
	MaxNSub      = 32
)

type Ordered interface {
	// Returns negative, zero, positive if element i is less, equal, greater than element j
	Compare(i, j int) int
}

type FibHeap struct {
	root  fibNode
	nodes fibNodeVec

	// Cached min index in heap.
	minIndex Index
	minValid bool
}

func (f *FibHeap) node(ni Index) *fibNode {
	if ni == fibRootIndex {
		return &f.root
	} else {
		return &f.nodes[ni]
	}
}

func (f *FibHeap) linkAfter(pi, xi Index) {
	p := f.node(pi)
	ni := p.next
	n := f.node(ni)
	x := f.node(xi)
	p.next = xi
	x.prev, x.next = pi, ni
	n.prev = xi
}

func (f *FibHeap) addRoot(xi Index) {
	f.linkAfter(fibRootIndex, xi)
}

func (f *FibHeap) unlink(xi Index) {
	x := &f.nodes[xi]
	p := f.node(x.prev)
	n := f.node(x.next)
	p.next = x.next
	n.prev = x.prev
}

// Add a new index to heap.
func (f *FibHeap) Add(xi Index) {
	if len(f.nodes) == 0 {
		f.root.next, f.root.prev = fibRootIndex, fibRootIndex
		f.root.sup = MaxIndex
		f.root.sub = MaxIndex
	}
	f.minValid = false
	f.nodes.Validate(uint(xi))
	x := &f.nodes[xi]
	x.sup = MaxIndex
	x.sub = MaxIndex
	x.nSub = 0
	x.isMarked = false
	f.addRoot(xi)
}

func (f *FibHeap) cutChildren(xi Index) {
	x := &f.nodes[xi]
	bi := x.sub
	if bi == MaxIndex {
		return
	}
	ci := bi
	for {
		c := &f.nodes[ci]
		ni := c.next
		c.sup = MaxIndex
		f.addRoot(ci)
		if ni == bi {
			break
		}
		ci = ni
	}
}

// Del deletes given index from heap.
func (f *FibHeap) Del(xi Index) {
	f.unlink(xi)
	f.cutChildren(xi)

	x := &f.nodes[xi]
	supi := x.sup
	f.minValid = f.minValid && xi != f.minIndex
	if supi == MaxIndex {
		return
	}

	// Adjust parent for deletion of child.
	ni := x.next
	for {
		sup := &f.nodes[supi]
		sup.nSub -= 1
		wasMarked := sup.isMarked
		sup.isMarked = true
		sup.sub = ni
		if sup.nSub == 0 {
			sup.sub = MaxIndex
		}
		sup2i := sup.sup
		if !wasMarked || sup2i == MaxIndex {
			break
		}
		ni = sup.next
		f.unlink(supi)
		sup.sup = MaxIndex
		f.addRoot(supi)
		supi = sup2i
	}
}

// Update node when data ordering changes (lower key)
func (f *FibHeap) Update(xi Index) {
	f.Del(xi)
	f.Add(xi)
}

func (f *FibHeap) Min(data Ordered) (min Index) {
	min = f.minIndex
	if f.minValid {
		return
	}

	// Degrees seen so far
	var deg [MaxNSub]Index
	// Bitmap of valid degrees, initially zero
	var degValid Word

	ri := f.root.next
	r := f.node(ri)
	ni := r.next

	for ri != fibRootIndex {
		r = &f.nodes[ri]
		n := f.node(ni)
		ns := r.nSub

		m := Word(1) << ns
		nsDegrees := 0 != degValid&m
		degValid ^= m
		if !nsDegrees {
			deg[ns] = ri
			ri = ni
			ni = n.next
		} else {
			ri0 := deg[ns]
			if data.Compare(int(ri0), int(ri)) <= 0 {
				ri, ri0 = ri0, ri
			}
			f.unlink(ri0)
			r0 := &f.nodes[ri0]
			r0.isMarked = false
			r0.sup = ri

			r = &f.nodes[ri]
			if r.sub != MaxIndex {
				r.nSub += 1
				f.linkAfter(r.sub, ri0)
			} else {
				r.sub = ri0
				r.nSub = 1
				r.isMarked = false
				r0.next = ri0
				r0.prev = ri0
			}
		}
	}

	min = MaxIndex
	for degValid != 0 {
		var ns int
		degValid, ns = NextSet(degValid)
		ri := deg[ns]
		if min == MaxIndex || data.Compare(int(ri), int(min)) < 0 {
			min = ri
		}
	}

	f.minValid = true
	f.minIndex = min

	return
}

func (n *fibNode) reloc(i int) {
	d := Index(i)
	if n.sup != MaxIndex {
		n.sup += d
	}
	if n.sub != MaxIndex {
		n.sub += d
	}
	if n.next != fibRootIndex {
		n.next += d
	}
	if n.prev != fibRootIndex {
		n.prev += d
	}
}

func (f *FibHeap) Merge(g *FibHeap) {
	l := len(f.nodes)
	f.nodes.Resize(uint(len(g.nodes)))
	copy(f.nodes[l:], g.nodes)
	for i := l; i < len(f.nodes); i++ {
		f.nodes[i].reloc(l)
	}
	r := g.root
	r.reloc(l)
	for ri := r.next; ri != fibRootIndex; {
		r := &f.nodes[ri]
		f.Add(ri)
		ri = r.next
	}
}

func (f *FibHeap) String() string {
	return fmt.Sprintf("%d elts", len(f.nodes))
}

func (f *FibHeap) validateNode(xi Index) (err error) {
	x := &f.nodes[xi]
	nSub := uint16(0)
	if x.sub != MaxIndex {
		subi := x.sub
		for {
			sub := &f.nodes[subi]

			if sub.sup != xi {
				err = fmt.Errorf("node.sub.sup %d != node %d", sub.sup, xi)
				return
			}

			n := &f.nodes[sub.next]
			p := &f.nodes[sub.prev]
			if n.prev != subi {
				err = fmt.Errorf("next.prev %d != node %d", n.prev, subi)
				return
			}
			if p.next != subi {
				err = fmt.Errorf("prev.next %d != node %d", p.next, subi)
				return
			}

			err = f.validateNode(subi)
			if err != nil {
				return
			}

			nSub++
			subi = sub.next
			if subi == x.sub {
				break
			}
		}

		if nSub != x.nSub {
			err = fmt.Errorf("n children %d != %d", nSub, x.nSub)
			return
		}
	}
	return
}

func (f *FibHeap) validate() (err error) {
	for ri := f.root.next; ri != fibRootIndex; {
		r := &f.nodes[ri]
		n := f.node(r.next)
		if n.prev != ri {
			err = fmt.Errorf("root next.prev %d != %d", n.prev, ri)
			return
		}
		if n.sup != MaxIndex {
			err = fmt.Errorf("root sup not empty")
			return
		}
		err = f.validateNode(ri)
		if err != nil {
			return
		}
		ri = r.next
	}

	return
}

type testFibHeap struct {
	// Number of iterations to run
	iterations IterInt

	// Validate/print every so many iterations (zero means never).
	validateEvery IterInt
	printEvery    IterInt

	// Seed to make randomness deterministic.  0 means choose seed.
	seed int64

	// Number of objects to create.
	nObjects IterInt

	verbose int

	profile string
}

type fibHeapTestObj []int64

func (data fibHeapTestObj) Compare(i, j int) int {
	return int(data[i] - data[j])
}

func FibHeapTest() {
	t := testFibHeap{
		iterations: 10,
		nObjects:   10,
		verbose:    1,
	}
	flag.Var(&t.iterations, "iter", "Number of iterations")
	flag.Var(&t.validateEvery, "valid", "Number of iterations per validate")
	flag.Var(&t.printEvery, "print", "Number of iterations per print")
	flag.Int64Var(&t.seed, "seed", 0, "Seed for random number generator")
	flag.Var(&t.nObjects, "objects", "Number of random objects")
	flag.IntVar(&t.verbose, "verbose", 0, "Be verbose")
	flag.StringVar(&t.profile, "profile", "", "Write CPU profile to file")
	flag.Parse()

	err := runFibHeapTest(&t)
	if err != nil {
		panic(err)
	}
}

func runFibHeapTest(t *testFibHeap) (err error) {
	var f FibHeap

	if t.seed == 0 {
		t.seed = int64(time.Now().Nanosecond())
	}

	rand.Seed(t.seed)
	if t.verbose != 0 {
		fmt.Printf("%#v\n", t)
	}
	objs := fibHeapTestObj(make([]int64, t.nObjects))

	var iter int

	validate := func() (err error) {
		fmin := f.Min(objs)
		omin := MaxIndex
		if t.validateEvery != 0 && iter%int(t.validateEvery) == 0 {
			for i := range objs {
				if objs[i] == 0 {
					continue
				}
				if omin == MaxIndex {
					omin = Index(i)
				} else if objs[i] < objs[omin] {
					omin = Index(i)
				}
			}
		} else {
			omin = fmin
		}
		if omin != fmin {
			err = fmt.Errorf("iter %d: min %d != %d", iter, omin, fmin)
			return
		}

		if t.validateEvery != 0 && iter%int(t.validateEvery) == 0 {
			if err = f.validate(); err != nil {
				if t.verbose != 0 {
					fmt.Printf("iter %d: %s\n%+v\n", iter, err, f)
				}
				return
			}
		}
		if t.printEvery != 0 && iter%int(t.printEvery) == 0 {
			fmt.Printf("%10g iterations: %s\n", float64(iter), &f)
		}
		return
	}

	if t.profile != "" {
		var f *os.File
		f, err = os.Create(t.profile)
		if err != nil {
			return
		}
		pprof.StartCPUProfile(f)
		defer func() { pprof.StopCPUProfile() }()
	}

	for iter = 0; iter < int(t.iterations); iter++ {
		x := Index(rand.Int() % len(objs))
		if objs[x] == 0 {
			objs[x] = 1 + rand.Int63()
			f.Add(x)
		} else {
			if rand.Int()%10 < 5 {
				objs[x] = 1 + rand.Int63()
				f.Update(x)
			} else {
				objs[x] = 0
				f.Del(x)
			}
		}
		err = validate()
		if err != nil {
			return
		}
	}
	if t.verbose != 0 {
		fmt.Printf("%d iterations: %+v\n", iter, f)
	}
	for i := range objs {
		if objs[i] != 0 {
			f.Del(Index(i))
			objs[i] = 0
		}
		err = validate()
		if err != nil {
			return
		}
		iter++
	}
	if t.verbose != 0 {
		fmt.Printf("%d iterations: %+v\n", iter, f)
		fmt.Printf("No errors: %d iterations\n", t.iterations)
	}
	return
}
