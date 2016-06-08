package loop

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/hw"

	"math"
	"reflect"
	"unsafe"
)

type BufferFlag uint32

const (
	NextValid BufferFlag = 1 << iota
	Cloned
)

type RefHeader struct {
	// 28 bits of offset; 4 bits of flags.
	offsetAndFlags uint32

	dataOffset uint16
	dataLen    uint16
}

const (
	RefBytes       = 16
	RefHeaderBytes = 1*4 + 2*2
	RefOpaqueBytes = RefBytes - RefHeaderBytes
)

type Ref struct {
	RefHeader

	// User opaque area.
	opaque [RefOpaqueBytes]byte
}

func (r *RefHeader) offset() uint32         { return r.offsetAndFlags &^ 0xf }
func (dst *RefHeader) copyOffset(src *Ref)  { dst.offsetAndFlags |= src.offset() }
func (r *RefHeader) Buffer() unsafe.Pointer { return hw.DmaGetOffset(uint(r.offset())) }
func (r *RefHeader) GetBuffer() *Buffer     { return (*Buffer)(r.Buffer()) }
func (r *RefHeader) Data() unsafe.Pointer {
	return hw.DmaGetOffset(uint(r.offset() + uint32(r.dataOffset)))
}

func (r *RefHeader) Flags() BufferFlag { return BufferFlag(r.offsetAndFlags & 0xf) }

func RefFlag1(f BufferFlag, r0 *RefHeader) bool { return r0.offsetAndFlags&uint32(f) != 0 }
func RefFlag2(f BufferFlag, r0, r1 *RefHeader) bool {
	return (r0.offsetAndFlags|r1.offsetAndFlags)&uint32(f) != 0
}
func RefFlag4(f BufferFlag, r0, r1, r2, r3 *RefHeader) bool {
	return (r0.offsetAndFlags|r1.offsetAndFlags|r2.offsetAndFlags|r3.offsetAndFlags)&uint32(f) != 0
}

func (r *RefHeader) DataSlice() (b []byte) {
	var h reflect.SliceHeader
	h.Data = uintptr(r.Data())
	h.Len = int(r.dataLen)
	h.Cap = int(r.dataLen)
	b = *(*[]byte)(unsafe.Pointer(&h))
	return
}

func (r *RefHeader) DataLen() uint { return uint(r.dataLen) }
func (r *RefHeader) SetLen(l uint) { r.dataLen = uint16(l) }
func (r *RefHeader) Advance(i int) (oldDataOffset int) {
	oldDataOffset = int(r.dataOffset)
	r.dataOffset = uint16(oldDataOffset + i)
	r.dataLen = uint16(int(r.dataLen) - i)
	return
}
func (r *RefHeader) Restore(oldDataOffset int) {
	r.dataOffset = uint16(oldDataOffset)
	Δ := int(r.dataOffset) - oldDataOffset
	r.dataLen = uint16(int(r.dataLen) - Δ)
}

//go:generate gentemplate -d Package=loop -id Ref -d VecType=RefVec -d Type=Ref github.com/platinasystems/elib/vec.tmpl

const (
	// Cache aligned/sized space for buffer header.
	BufferHeaderBytes = cpu.CacheLineBytes
	// Rewrite area.
	RewriteBytes  = 128
	overheadBytes = BufferHeaderBytes + RewriteBytes
)

// Buffer header.
type BufferHeader struct {
	// Identical to flags in buffer reference.
	flags BufferFlag

	// Valid only if NextValid flag is set.
	nextRef RefHeader

	// Number of clones of this buffer.
	cloneCount uint32
}

func (r *RefHeader) NextRef() (x *RefHeader) {
	if r.Flags()&NextValid != 0 {
		x = &r.GetBuffer().nextRef
	}
	return
}

type Buffer struct {
	BufferHeader
	Opaque [BufferHeaderBytes - unsafe.Sizeof(BufferHeader{})]byte
}

func (b *Buffer) reset(p *BufferPool) { *b = p.Buffer }

type BufferPool struct {
	BufferTemplate

	// References to buffers in this pool.
	refs RefVec

	// DMA memory chunks used by this pool.
	memChunkIDs []elib.Index
}

// Method to over-ride to initialize refs for this buffer pool.
// This is used for example to set packet lengths, adjust packet fields, etc.
func (p *BufferPool) InitRefs(refs []Ref) {}

func isPrime(i uint) bool {
	max := uint(math.Sqrt(float64(i)))
	for j := uint(2); j <= max; j++ {
		if i%j == 0 {
			return false
		}
	}
	return true
}

// Size of a data buffer in given free list.
// Choose to be a prime number of cache lines to randomize addresses for better cache usage.
func (p *BufferPool) bufferSize() uint {
	nBytes := overheadBytes + p.Size
	nLines := nBytes / cpu.CacheLineBytes
	for !isPrime(nLines) {
		nLines++
	}
	return nLines * cpu.CacheLineBytes
}

var defaultRef = Ref{RefHeader: RefHeader{dataOffset: RewriteBytes}}
var defaultBuf = Buffer{}

type BufferTemplate struct {
	// Data size of buffers.
	Size uint

	sizeIncludingOverhead uint

	Ref
	Buffer

	// If non-nil buffers will be initialized with this data.
	Data []byte
}

var DefaultBufferTemplate = &BufferTemplate{
	Size: 512,
	Ref:  Ref{RefHeader: RefHeader{dataOffset: RewriteBytes}},
}
var DefaultBufferPool = NewBufferPool(DefaultBufferTemplate)

func (p *BufferPool) Init() {
	t := &p.BufferTemplate
	if len(t.Data) > 0 {
		t.Ref.dataLen = uint16(len(t.Data))
	}
	p.Size = uint(elib.Word(p.Size).RoundCacheLine())
	p.sizeIncludingOverhead = p.bufferSize()
	p.Size = p.sizeIncludingOverhead - overheadBytes
}

func NewBufferPool(t *BufferTemplate) (p *BufferPool) {
	p = &BufferPool{}
	p.BufferTemplate = *t
	p.Init()
	return
}

func (p *BufferPool) Del() {
	for i := range p.memChunkIDs {
		hw.DmaFree(p.memChunkIDs[i])
	}
	// Unlink garbage.
	p.memChunkIDs = nil
	p.refs = nil
	p.Data = nil
}

func (r *RefHeader) slice(n uint) (l []Ref) {
	var h reflect.SliceHeader
	h.Data = uintptr(unsafe.Pointer(r))
	h.Len = int(n)
	h.Cap = int(n)
	l = *(*[]Ref)(unsafe.Pointer(&h))
	return
}

func (p *BufferPool) AllocRefs(r *RefHeader, n uint) {
	refs := r.slice(n)
	var got, want uint
	if got, want = uint(len(p.refs)), uint(len(refs)); got < want {
		n := uint(elib.RoundPow2(elib.Word(want-got), 2*V))
		b := p.sizeIncludingOverhead
		_, id, offset, _ := hw.DmaAlloc(n * b)
		ri := got
		p.refs.Resize(n)
		p.memChunkIDs = append(p.memChunkIDs, id)
		// Refs are allocated from end of refs so we put smallest offsets there.
		o := offset + (n-1)*b
		for i := uint(0); i < n; i++ {
			r := p.Ref
			r.offsetAndFlags += uint32(o)
			p.refs[ri] = r
			ri++
			o -= b
			if p.Data != nil {
				d := r.DataSlice()
				copy(d, p.Data)
			}
		}
		got += n
		// Possibly initialize/adjust newly made buffers.
		p.InitRefs(p.refs[got-n : got])
	}

	copy(refs, p.refs[got-want:got])
	p.refs = p.refs[:got-want]
}

// Return all buffers to pool and reset for next usage.
func (p *BufferPool) FreeRefs(rh *RefHeader, n uint) {
	toFree := rh.slice(n)
	l := uint(len(p.refs))
	p.refs.Resize(uint(n))
	r := p.refs[l:]

	t := p.Ref
	i := 0
	for n >= 4 {
		r0, r1, r2, r3 := &toFree[i+0], &toFree[i+1], &toFree[i+2], &toFree[i+3]
		b0, b1, b2, b3 := r0.GetBuffer(), r1.GetBuffer(), r2.GetBuffer(), r3.GetBuffer()
		r[i+0], r[i+1], r[i+2], r[i+3] = t, t, t, t
		r[i+0].copyOffset(r0)
		r[i+1].copyOffset(r1)
		r[i+2].copyOffset(r2)
		r[i+3].copyOffset(r3)
		b0.reset(p)
		b1.reset(p)
		b2.reset(p)
		b3.reset(p)
		i += 4
		n -= 4
	}

	for n > 0 {
		r0 := &toFree[i+0]
		b0 := r0.GetBuffer()
		r[i+0] = t
		r[i+0].copyOffset(r0)
		b0.reset(p)
		i += 1
		n -= 1
	}

	p.InitRefs(p.refs[l:])
}
