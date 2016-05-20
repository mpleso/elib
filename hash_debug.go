//+build debug

package elib

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime/pprof"
	"time"
	"unsafe"
)

type uiHash struct {
	Hash
	pairs uiPairVec
}

type uiKey uint64
type uiValue uint64
type uiPair struct {
	k uiKey
	v uiValue
}

func (p *uiPair) Equal(q *uiPair) bool { return p.k == q.k && p.v == q.v }

//go:generate gentemplate -d Package=elib -id uiPair -d VecType=uiPairVec -d Type=uiPair -tags debug vec.tmpl

func (k uiKey) HashKey(s *HashState)               { s.HashPointer(unsafe.Pointer(&k), unsafe.Sizeof(k)) }
func (k uiKey) HashKeyEqual(h Hasher, i uint) bool { return k == h.(*uiHash).pairs[i].k }
func (h *uiHash) HashIndex(s *HashState, i uint)   { h.pairs[i].k.HashKey(s) }
func (h *uiHash) HashResize() {
	rs := h.ResizeRemaps
	i, n := 0, len(rs)
	dst := make([]uiPair, h.Cap())
	src := h.pairs
	for i+4 <= n {
		dst[rs[i+0].dst] = src[rs[i+0].src]
		dst[rs[i+1].dst] = src[rs[i+1].src]
		dst[rs[i+2].dst] = src[rs[i+2].src]
		dst[rs[i+3].dst] = src[rs[i+3].src]
		i += 4
	}
	for i < n {
		dst[rs[i+0].dst] = src[rs[i+0].src]
		i += 1
	}
	h.pairs = dst
}

func (h *Hash) String() string {
	return fmt.Sprintf("%d elts/%d cap", h.Elts(), h.Cap())
}

type testHash struct {
	uiHash uiHash

	pairs    uiPairVec
	inserted Bitmap

	// Number of iterations to run
	iterations Count

	// Validate/print every so many iterations (zero means never).
	validateEvery Count
	printEvery    Count

	// Seed to make randomness deterministic.  0 means choose seed.
	seed int64

	nKeys Count

	verbose int

	profile string
}

func HashTest() {
	t := testHash{
		iterations: 10,
		nKeys:      10,
		verbose:    1,
	}
	flag.Var(&t.iterations, "iter", "Number of iterations")
	flag.Var(&t.validateEvery, "valid", "Number of iterations per validate")
	flag.Var(&t.printEvery, "print", "Number of iterations per print")
	flag.Int64Var(&t.seed, "seed", 0, "Seed for random number generator")
	flag.Var(&t.nKeys, "keys", "Number of random keys")
	flag.IntVar(&t.verbose, "verbose", 0, "Be verbose")
	flag.StringVar(&t.profile, "profile", "", "Write CPU profile to file")
	flag.Parse()

	err := runHashTest(&t)
	if err != nil {
		panic(err)
	}
}

func (t *testHash) doValidate() (err error) {
	h := &t.uiHash
	for pi := uint(0); pi < t.pairs.Len(); pi++ {
		p := &t.pairs[pi]
		i, ok := h.Get(p.k)
		if got, want := ok, t.inserted.Get(pi); got != want {
			err = fmt.Errorf("get ok %v != inserted %v", got, want)
			j, ok1 := h.Get(p.k)
			_ = j
			_ = ok1
			return
		}
		if ok && !p.Equal(&h.pairs[i]) {
			err = fmt.Errorf("get index got %d != want %d", i, pi)
			return
		}
	}
	return
}

func (t *testHash) validate(h *Hash, iter int) (err error) {
	if t.validateEvery != 0 && iter%int(t.validateEvery) == 0 {
		if err = t.doValidate(); err != nil {
			if t.verbose != 0 {
				fmt.Printf("iter %d: %s\n", iter, err)
			}
			return
		}
	}
	if t.printEvery != 0 && iter%int(t.printEvery) == 0 {
		fmt.Printf("%10g iterations: %s\n", float64(iter), h)
	}
	return
}

func runHashTest(t *testHash) (err error) {
	if t.seed == 0 {
		t.seed = int64(time.Now().Nanosecond())
	}

	rand.Seed(t.seed)
	if t.verbose != 0 {
		fmt.Printf("%#v\n", t)
	}

	h := &t.uiHash
	t.pairs.Resize(uint(t.nKeys))
	log2n := Word(t.nKeys).MaxLog2()
	for i := range t.pairs {
		t.pairs[i].k = uiKey((uint64(rand.Int63()) << log2n) + uint64(i))
		t.pairs[i].v = uiValue(rand.Int63())
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

	h.Hasher = h
	zero := uiPair{}
	var iter int
	for ; iter < int(t.iterations); iter++ {
		pi := uint(rand.Intn(int(t.nKeys)))
		p := &t.pairs[pi]
		var was bool
		if t.inserted, was = t.inserted.Invert2(pi); !was {
			i, exists := h.Set(p.k)
			if exists {
				panic("exists")
			}
			h.pairs[i] = *p
		} else {
			i, ok := h.Unset(p.k)
			if !ok {
				panic("unset")
			}
			h.pairs[i] = zero
		}

		err = t.validate(&h.Hash, iter)
		if err != nil {
			return
		}
	}
	if t.verbose != 0 {
		fmt.Printf("%d iterations: %s\n", iter, h)
	}
	for _ = range h.pairs {
		err = t.validate(&h.Hash, iter)
		if err != nil {
			return
		}
		iter++
	}
	if t.verbose != 0 {
		fmt.Printf("%d iterations: %s\n", iter, h)
		fmt.Printf("No errors: %d iterations\n", t.iterations)
	}
	return
}
