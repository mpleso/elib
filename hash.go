package elib

import (
	"reflect"
	"unsafe"
)

type hash64 uint64

func (h hash64) rotate(n hash64) hash64 { return (h << n) | (h >> (64 - n)) }

type HashState struct {
	state [2]hash64
}

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

	n8 := n / 8
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

	n4 := (n - i) / 4
	if i+1*4 <= n4 {
		h3 += s.get32(b, i)
		i += 1 * 4
	}

	n2 := (n - i) / 2
	if i+1*2 <= n2 {
		h3 += s.get16(b, i) << 32
		i += 1 * 2
	}

	if i > 0 {
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

	h0, h1, h2, h3 := s.state[0], s.state[1], seedConst, seedConst

	// Mix in data length.
	h0 += hash64(len(b))

	h0, h1, h2, h3 = s.mixSlice(h0, h1, h2, h3, b)

	h0, h1, h2, h3 = s.finalize(h0, h1, h2, h3)

	s.state[0], s.state[1] = h0, h1
}

func (s *HashState) seed(h0, h1 uint64) { s.state[0], s.state[1] = hash64(h0), hash64(h1) }

func (s *HashState) HashPointer(p unsafe.Pointer, size uintptr) {
	var h reflect.SliceHeader
	h.Data = uintptr(p)
	h.Len = int(size)
	h.Cap = int(size)
	b := *(*[]byte)(unsafe.Pointer(&h))
	s.hashSlice(b)
}

type Hash struct {
	seed              HashState
	cap               Cap
	log2Cap           [2]uint8
	log2EltsPerBucket uint8
	eltsPerBucket     uint32
	limit0            uint32
	shortHash         []shortHash
}

type shortHash uint8

type Hasher interface {
	// k0.Equal(keys[i]) returns k0 == k1.
	KeyEqual(i uint) bool
	// Compute hash for key.
	KeyHash(s *HashState)
}

func (h *Hash) capMask(i uint) uint { return uint(1)<<h.log2Cap[i] - 1 }

func (h *HashState) limit() uint32 { return uint32(h.state[0] >> 32) }

const shortHashValid = 1

func (h *HashState) shortHash() shortHash { return shortHash(h.state[0]) | shortHashValid }
func (s shortHash) isValid() bool         { return s&shortHashValid != 0 }
func (h *HashState) offset() hash64       { return h.state[1] }

func (h *Hash) baseIndex(s *HashState) uint {
	is_table_1 := uint(0)
	if uint32(s.state[0]) > h.limit0 {
		is_table_1 = 1
	}
	return uint(s.offset())&h.capMask(is_table_1) + (is_table_1 << h.log2Cap[0])
}

func (h *Hash) baseIndexForKey(k Hasher) (uint, shortHash) {
	var s HashState = h.seed
	k.KeyHash(&s)
	return h.baseIndex(&s), s.shortHash()
}

func (h *Hash) search(k Hasher) (baseIndex, matchDiff, freeDiff uint, kh shortHash, ok bool) {
	baseIndex, kh = h.baseIndexForKey(k)
	n := uint(1) << h.log2EltsPerBucket
	freeDiff = n
	for diff := uint(0); diff < n; diff++ {
		i := baseIndex ^ diff
		sh := h.shortHash[i]
		if sh == kh && k.KeyEqual(i) {
			matchDiff = diff
			ok = true
			break
		}
		if !sh.isValid() && freeDiff >= n {
			freeDiff = diff
		}
	}
	return
}

func (h *Hash) ForeachIndex(f func(i uint)) {
	for i := range h.shortHash {
		if h.shortHash[i].isValid() {
			f(uint(i))
		}
	}
}

func (h *Hash) Get(k Hasher) (i uint, ok bool) {
	var bi, mi uint
	if bi, mi, _, _, ok = h.search(k); ok {
		i = bi ^ mi
	}
	return i, ok
}

func (h *Hash) diffValid(d uint) bool { return d>>h.log2EltsPerBucket == 0 }

func (h *Hash) Set(k Hasher) (i uint, exists bool) {
	var (
		bi, mi, fi uint
		kh         shortHash
	)
	bi, mi, fi, kh, exists = h.search(k)
	if exists {
		// Key already exists.
		i = bi ^ mi
	} else if h.diffValid(fi) {
		// Use up free slot in bucket.
		i = fi ^ mi
		h.shortHash[i] = kh
	} else {
		// Bucket full.
	}
	return
}
