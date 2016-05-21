package elib

import (
	"fmt"
	"math/rand"
	"reflect"
	"unsafe"
)

type hash64 uint64

func (h hash64) rotate(n hash64) hash64 { return (h << n) | (h >> (64 - n)) }

type HashState [2]hash64

func (s *HashState) mixStep(a, b, c, n hash64) (hash64, hash64, hash64) {
	a = a.rotate(n) + b
	c ^= a
	return a, b, c
}

func (s *HashState) mix(h0, h1, h2, h3 hash64) (hash64, hash64, hash64, hash64) {
	h2, h3, h0 = s.mixStep(h2, h3, h0, 50)
	h3, h0, h1 = s.mixStep(h3, h0, h1, 52)
	h0, h1, h2 = s.mixStep(h0, h1, h2, 30)
	h1, h2, h3 = s.mixStep(h1, h2, h3, 41)

	h2, h3, h0 = s.mixStep(h2, h3, h0, 54)
	h3, h0, h1 = s.mixStep(h3, h0, h1, 48)
	h0, h1, h2 = s.mixStep(h0, h1, h2, 38)
	h1, h2, h3 = s.mixStep(h1, h2, h3, 37)

	h2, h3, h0 = s.mixStep(h2, h3, h0, 62)
	h3, h0, h1 = s.mixStep(h3, h0, h1, 34)
	h0, h1, h2 = s.mixStep(h0, h1, h2, 5)
	h1, h2, h3 = s.mixStep(h1, h2, h3, 36)

	return h0, h1, h2, h3
}

func (*HashState) finStep(a, b, n hash64) (hash64, hash64) {
	a ^= b
	b = b.rotate(n)
	a += b
	return a, b
}

func (s *HashState) finalize(h0, h1, h2, h3 hash64) (hash64, hash64, hash64, hash64) {
	h3, h2 = s.finStep(h3, h2, 15)
	h0, h3 = s.finStep(h0, h3, 52)
	h1, h0 = s.finStep(h1, h0, 26)
	h2, h1 = s.finStep(h2, h1, 51)

	h3, h2 = s.finStep(h3, h2, 28)
	h0, h3 = s.finStep(h0, h3, 9)
	h1, h0 = s.finStep(h1, h0, 47)
	h2, h1 = s.finStep(h2, h1, 54)

	h3, h2 = s.finStep(h3, h2, 32)
	h0, h3 = s.finStep(h0, h3, 25)
	h1, h0 = s.finStep(h1, h0, 63)

	return h0, h1, h2, h3
}

func (s *HashState) get64(b []byte, i int) hash64 {
	return hash64(unalignedUint64(unsafe.Pointer(&b[i])))
}
func (s *HashState) get32(b []byte, i int) hash64 {
	return hash64(unalignedUint32(unsafe.Pointer(&b[i])))
}
func (s *HashState) get16(b []byte, i int) hash64 {
	return hash64(unalignedUint16(unsafe.Pointer(&b[i])))
}

func (s *HashState) mixSlice(h0, h1, h2, h3 hash64, b []byte) (hash64, hash64, hash64, hash64) {
	n := len(b)
	i := 0

	n8 := n &^ 7

	for i+4*8 <= n8 {
		h2 += s.get64(b, i+0*8)
		h3 += s.get64(b, i+1*8)
		h0, h1, h2, h3 = s.mix(h0, h1, h2, h3)
		h0 += s.get64(b, i+2*8)
		h1 += s.get64(b, i+3*8)
		i += 4 * 8
	}

	if i+2*8 <= n8 {
		h2 += s.get64(b, i+0*8)
		h3 += s.get64(b, i+1*8)
		h0, h1, h2, h3 = s.mix(h0, h1, h2, h3)
		i += 2 * 8
	}

	if i+1*8 <= n8 {
		h2 += s.get64(b, i)
		i += 1 * 8
	}

	n4 := (n - i) &^ 3
	if i+1*4 <= n4 {
		h3 += s.get32(b, i)
		i += 1 * 4
	}

	n2 := (n - i) &^ 1
	if i+1*2 <= n2 {
		h3 += s.get16(b, i) << 32
		i += 1 * 2
	}

	if i < n {
		h3 += hash64(b[i]) << (32 + 16)
	}

	return h0, h1, h2, h3
}

func (s *HashState) hashSlice(b []byte) {
	// A constant which:
	//  * is not zero
	//  * is odd
	//  * is a not-very-regular mix of 1's and 0's
	//  * does not need any other special mathematical properties.
	const seedConst hash64 = 0xdeadbeefdeadbeef

	h0, h1, h2, h3 := s[0], s[1], seedConst, seedConst

	// Mix in data length.
	h0 += hash64(len(b))

	h0, h1, h2, h3 = s.mixSlice(h0, h1, h2, h3, b)

	h0, h1, h2, h3 = s.finalize(h0, h1, h2, h3)

	s[0], s[1] = h0, h1
}

func (s *HashState) seed(h0, h1 uint64) { s[0], s[1] = hash64(h0), hash64(h1) }

func (s *HashState) HashPointer(p unsafe.Pointer, size uintptr) {
	var h reflect.SliceHeader
	h.Data = uintptr(p)
	h.Len = int(size)
	h.Cap = int(size)
	b := *(*[]byte)(unsafe.Pointer(&h))
	s.hashSlice(b)
}

type stats struct {
	calls    uint64
	searches uint64
	compares uint64
}

func (s *stats) compare(x uint) { s.compares += uint64(x) }
func (s *stats) search(x uint)  { s.searches += uint64(x); s.calls += 1 }

type Hash struct {
	Hasher            Hasher
	seed              HashState
	cap               Cap
	log2Cap           [2]uint8
	log2EltsPerBucket uint8
	eltsPerBucket     uint32
	limit0            uint32
	bitDiffs          []bitDiff
	maxBucketBitDiffs []bitDiff
	nElts             uint
	ResizeRemaps      []HashRemap
	stats             struct {
		grows           uint64
		copies          uint64
		get, set, unset stats
	}
}

// Bit difference plus 1.
type bitDiff uint8

func (d bitDiff) isValid() bool        { return d != 0 }
func (d *bitDiff) invalidate()         { *d = 0 }
func (d bitDiff) match(diff uint) bool { return diff+1 == uint(d) }
func (d *bitDiff) set(h *Hash, baseIndex, diff uint) {
	bd := bitDiff(1 + diff)
	bi := baseIndex >> h.log2EltsPerBucket
	if bd > h.maxBucketBitDiffs[bi] {
		h.maxBucketBitDiffs[bi] = bd
	}
	*d = bd
}

type HashRemap struct{ src, dst uint }

type Hasher interface {
	HashIndex(s *HashState, i uint)
	HashResize()
}

type HasherKey interface {
	// k0.Equal(keys[i]) returns k0 == k1.
	HashKeyEqual(h Hasher, i uint) bool
	// Compute hash for key.
	HashKey(s *HashState)
}

func (h *Hash) capMask(i uint) uint { return uint(1)<<h.log2Cap[i] - 1 }

func (h *HashState) limit() uint32  { return uint32(h[0] >> 32) }
func (h *HashState) offset() hash64 { return h[1] }

func (h *Hash) baseIndex(s *HashState) uint {
	is_table_1 := uint(0)
	if uint32(s[0]) > h.limit0 {
		is_table_1 = 1
	}
	return uint(s.offset())&h.capMask(is_table_1) + (is_table_1 << h.log2Cap[0])
}

func (h *Hash) baseIndexForKey(s *HashState, k HasherKey) uint {
	*s = h.seed
	k.HashKey(s)
	return h.baseIndex(s)
}

func (h *Hash) baseIndexForIndex(s *HashState, i uint) uint {
	*s = h.seed
	h.Hasher.HashIndex(s, i)
	return h.baseIndex(s)
}

func (h *Hash) empty() bool { return len(h.bitDiffs) == 0 }

func (h *Hash) searchBase(baseIndex uint, st *stats, k HasherKey) (matchDiff uint, ok bool) {
	n := uint(1) << h.log2EltsPerBucket
	matchDiff = n
	bucketIndex := baseIndex >> h.log2EltsPerBucket
	maxValidDiff := h.maxBucketBitDiffs[bucketIndex]
	diff := uint(0)
	for ; diff < n; diff++ {
		i := baseIndex ^ diff
		if bd := h.bitDiffs[i]; bd.match(diff) {
			st.compare(1)
			if k.HashKeyEqual(h.Hasher, i) {
				matchDiff = diff
				ok = true
				break
			}
		} else if diff+1 >= uint(maxValidDiff) {
			break
		}
	}
	st.search(diff)
	return
}

func (h *Hash) searchKey(s *HashState, st *stats, k HasherKey) (baseIndex, matchDiff uint, ok bool) {
	baseIndex = h.baseIndexForKey(s, k)
	matchDiff, ok = h.searchBase(baseIndex, st, k)
	return
}

// Search for an empty slot for key at index i.
func (h *Hash) searchFreeIndex(baseIndex uint) (i, diff uint, ok bool) {
	n := uint(1) << h.log2EltsPerBucket
	for ; diff < n; diff++ {
		i = baseIndex ^ diff
		if bd := h.bitDiffs[i]; !bd.isValid() {
			h.bitDiffs[i].set(h, baseIndex, diff)
			ok = true
			return
		}
	}
	return
}

// Search for an empty slot for key at index i.
func (h *Hash) searchIndex(s *HashState, ki uint) (i uint, ok bool) {
	baseIndex := h.baseIndexForIndex(s, ki)
	i, _, ok = h.searchFreeIndex(baseIndex)
	return
}

func (h *Hash) ForeachIndex(f func(i uint)) {
	for i := range h.bitDiffs {
		if h.bitDiffs[i].isValid() {
			f(uint(i))
		}
	}
}

func (h *Hash) Elts() uint { return uint(h.nElts) }
func (h *Hash) Cap() uint  { return uint(h.cap) }

func (h *Hash) Get(k HasherKey) (i uint, ok bool) {
	if h.empty() {
		return
	}
	var (
		s      HashState
		bi, mi uint
	)
	if bi, mi, ok = h.searchKey(&s, &h.stats.get, k); ok {
		i = bi ^ mi
	}
	return i, ok
}

func (h *Hash) diffValid(d uint) bool { return d>>h.log2EltsPerBucket == 0 }

func (h *Hash) Set(k HasherKey) (i uint, exists bool) {
	var (
		bi, fi, mi uint
		s          HashState
	)
	nonEmpty := !h.empty()
	for {
		if nonEmpty {
			bi, mi, exists = h.searchKey(&s, &h.stats.set, k)
		}
		if exists {
			// Key already exists.
			i = bi ^ mi
			return
		}

		foundFree := false
		if nonEmpty {
			_, fi, foundFree = h.searchFreeIndex(bi)
		}
		if foundFree {
			// Use up free slot in bucket.
			i = bi ^ fi
			h.bitDiffs[i].set(h, bi, fi)
			h.nElts++
			return
		}

		// Bucket full: grow hash and copy elements.
		save := h.bitDiffs
		for {
			h.grow()
			if h.copy(&s, save) {
				break
			}
		}
		nonEmpty = true
	}
	return
}

func (h *Hash) Unset(k HasherKey) (i uint, ok bool) {
	var (
		bi, mi uint
		s      HashState
	)
	bi, mi, ok = h.searchKey(&s, &h.stats.unset, k)
	if ok {
		i = bi ^ mi
		h.bitDiffs[i].invalidate()
		h.nElts--
	}
	return
}

func (h *Hash) grow() {
	h.stats.grows++
	h.cap = h.cap.Next()

	log2c0, log2c1 := h.cap.Log2()
	h.log2Cap[0] = uint8(log2c0)
	h.log2Cap[1] = 0
	if log2c1 != CapNil {
		h.log2Cap[1] = uint8(log2c1)
	}

	// For approx occupancy of .5 = 2^-1, probability of a full bucket of size M is 2^-(1 + M).
	// So with N_BUCKETS = (2^l0 + 2^l1) / 2^M we have the probability that at least one bucket
	// is full is (2^l0 + 2^l1) / 2^(2M + 1) ~ 1.  So, we set l0 = 2M + 1.
	h.log2EltsPerBucket = uint8((log2c0 - 1) / 2)

	// Since bit diff is only 8 bits, cap bucket size.
	if h.log2EltsPerBucket > 7 {
		h.log2EltsPerBucket = 7
	}

	if log2c1 == CapNil {
		// No limit for table 0.
		h.limit0 = ^uint32(0)
	} else {
		// Capacity must be even number of buckets.
		if h.log2EltsPerBucket > h.log2Cap[1] {
			log2c1 = Cap(h.log2EltsPerBucket)
			h.cap = (1 << log2c0) | (1 << log2c1)
			h.log2Cap[1] = uint8(log2c1)
		}

		// 2^32 2^i_0 / (2^i_0 + 2^i_1).
		h.limit0 = uint32(uint64(1) << (32 + log2c0) / uint64(h.cap))
	}

	for i := range h.seed {
		h.seed[i] = hash64(rand.Int63())
	}

	h.bitDiffs = make([]bitDiff, h.cap)
	nBuckets := h.cap >> h.log2EltsPerBucket
	h.maxBucketBitDiffs = make([]bitDiff, nBuckets)
	h.nElts = 0
	return
}

func (h *Hash) copy(s *HashState, bds []bitDiff) (ok bool) {
	h.stats.copies++
	if cap(h.ResizeRemaps) < len(bds) {
		h.ResizeRemaps = make([]HashRemap, len(bds))
	}
	var src, dst, n, l uint
	l = uint(len(bds))
	for src = 0; src < l; src++ {
		if bds[src].isValid() {
			if dst, ok = h.searchIndex(s, src); ok {
				h.ResizeRemaps[n].src = src
				h.ResizeRemaps[n].dst = dst
				n++
			} else {
				break
			}
		}
	}
	if ok = src >= l; ok {
		if h.ResizeRemaps != nil {
			h.ResizeRemaps = h.ResizeRemaps[:n]
		}
		h.Hasher.HashResize()
		h.nElts = n
	}
	return
}

func (s *stats) String() (v string) {
	if s.calls != 0 {
		v = fmt.Sprintf("search/call %.2f, cmp/call %.2f", float64(s.searches)/float64(s.calls), float64(s.compares)/float64(s.calls))
	}
	return
}

func (h *Hash) String() string {
	return fmt.Sprintf("elts %d, cap %d, bucket: 2^%d, grows %d, copies %d\n    get: %s\n    set: %s\n  unset: %s",
		h.Elts(), h.Cap(), h.log2EltsPerBucket, h.stats.grows, h.stats.copies, &h.stats.get, &h.stats.set, &h.stats.unset)
}
