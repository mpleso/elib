package elib

import (
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
	searches uint64
	compares uint64
}

func (s *stats) compare(x uint) { s.compares += uint64(x) }
func (s *stats) search(x uint)  { s.searches += uint64(x) }

type Hash struct {
	Hasher            Hasher
	seed              HashState
	cap               Cap
	log2Cap           [2]uint8
	log2EltsPerBucket uint8
	eltsPerBucket     uint32
	limit0            uint32
	shortHash         []shortHash
	nElts             uint
	stats
	ResizeRemaps []HashRemap
}

type shortHash uint8

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

func (h *HashState) limit() uint32 { return uint32(h[0] >> 32) }

const shortHashNil = 0

func (h *HashState) shortHash() (sh shortHash) {
	x := h[0]
	for {
		sh = shortHash(x)
		if sh != shortHashNil {
			break
		}
		x = x >> 8
		if x == 0 {
			// fallback non-nil value when
			sh = 1 + shortHashNil
			break
		}
	}
	return
}
func (s shortHash) isValid() bool   { return s != shortHashNil }
func (h *HashState) offset() hash64 { return h[1] }

func (h *Hash) baseIndex(s *HashState) uint {
	is_table_1 := uint(0)
	if uint32(s[0]) > h.limit0 {
		is_table_1 = 1
	}
	return uint(s.offset())&h.capMask(is_table_1) + (is_table_1 << h.log2Cap[0])
}

func (h *Hash) baseIndexForKey(k HasherKey) (uint, shortHash) {
	var s HashState = h.seed
	k.HashKey(&s)
	return h.baseIndex(&s), s.shortHash()
}

func (h *Hash) baseIndexForIndex(i uint) (uint, shortHash) {
	var s HashState = h.seed
	h.Hasher.HashIndex(&s, i)
	return h.baseIndex(&s), s.shortHash()
}

func (h *Hash) searchKey(k HasherKey) (baseIndex, matchDiff, freeDiff uint, kh shortHash, ok bool) {
	n := uint(1) << h.log2EltsPerBucket
	freeDiff = n
	matchDiff = n
	if len(h.shortHash) == 0 {
		return
	}

	baseIndex, kh = h.baseIndexForKey(k)
	diff := uint(0)
	for ; diff < n; diff++ {
		i := baseIndex ^ diff
		if sh := h.shortHash[i]; sh == kh {
			h.stats.compare(1)
			if k.HashKeyEqual(h.Hasher, i) {
				matchDiff = diff
				ok = true
				break
			}
		} else if !sh.isValid() && freeDiff >= n {
			freeDiff = diff
		}
	}
	h.stats.search(diff)
	return
}

// Search for an empty slot for key at index i.
func (h *Hash) searchIndex(ki uint) (uint, bool) {
	baseIndex, kh := h.baseIndexForIndex(ki)
	diff := uint(0)
	n := uint(1) << h.log2EltsPerBucket
	for ; diff < n; diff++ {
		i := baseIndex ^ diff
		sh := h.shortHash[i]
		if !sh.isValid() {
			h.shortHash[i] = kh
			return i, true
		}
	}
	return diff, false
}

func (h *Hash) ForeachIndex(f func(i uint)) {
	for i := range h.shortHash {
		if h.shortHash[i].isValid() {
			f(uint(i))
		}
	}
}

func (h *Hash) Elts() uint { return uint(h.nElts) }
func (h *Hash) Cap() uint  { return uint(h.cap) }

func (h *Hash) Get(k HasherKey) (i uint, ok bool) {
	var bi, mi uint
	if bi, mi, _, _, ok = h.searchKey(k); ok {
		i = bi ^ mi
	}
	return i, ok
}

func (h *Hash) diffValid(d uint) bool { return d>>h.log2EltsPerBucket == 0 }

func (h *Hash) Set(k HasherKey) (i uint, exists bool) {
	var (
		bi, mi, fi uint
		kh         shortHash
	)
	bi, mi, fi, kh, exists = h.searchKey(k)
	if exists {
		// Key already exists.
		i = bi ^ mi
	} else if h.diffValid(fi) {
		// Use up free slot in bucket.
		i = bi ^ fi
		h.shortHash[i] = kh
		h.nElts++
	} else {
		// Bucket full.
		save := h.shortHash
		for {
			h.grow()
			if h.copy(save) {
				break
			}
		}
		return h.Set(k)
	}
	return
}

func (h *Hash) Unset(k HasherKey) (i uint, ok bool) {
	var bi, mi uint
	bi, mi, _, _, ok = h.searchKey(k)
	if ok {
		i = bi ^ mi
		h.shortHash[i] = shortHashNil
		h.nElts--
	}
	return
}

func (h *Hash) grow() {
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

	h.shortHash = make([]shortHash, h.cap, h.cap)
	h.nElts = 0
	return
}

func (h *Hash) copy(sh []shortHash) (ok bool) {
	if cap(h.ResizeRemaps) < len(sh) {
		h.ResizeRemaps = make([]HashRemap, len(sh))
	}
	var src, dst, n, l uint
	l = uint(len(sh))
	for src = 0; src < l; src++ {
		if sh[src].isValid() {
			if dst, ok = h.searchIndex(src); ok {
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