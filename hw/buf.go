package hw

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"

	"math"
	"reflect"
	"unsafe"
)

type BufferFlag uint32

const (
	NextValid, Log2NextValid BufferFlag = 1 << iota, iota
	Cloned, Log2Cloned
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

func (r *RefHeader) h() *RefHeader          { return r }
func (r *RefHeader) offset() uint32         { return r.offsetAndFlags &^ 0xf }
func (dst *RefHeader) copyOffset(src *Ref)  { dst.offsetAndFlags |= src.offset() }
func (r *RefHeader) Buffer() unsafe.Pointer { return DmaGetOffset(uint(r.offset())) }
func (r *RefHeader) GetBuffer() *Buffer     { return (*Buffer)(r.Buffer()) }
func (r *RefHeader) Data() unsafe.Pointer {
	return DmaGetOffset(uint(r.offset() + uint32(r.dataOffset)))
}
func (r *RefHeader) DataPhys() uintptr { return DmaPhysAddress(uintptr(r.Data())) }

func (r *RefHeader) Flags() BufferFlag         { return BufferFlag(r.offsetAndFlags & 0xf) }
func (r *RefHeader) NextValidFlag() BufferFlag { return BufferFlag(r.offsetAndFlags) & NextValid }
func (r *RefHeader) Nextvalid() bool           { return r.NextValidFlag() != 0 }
func (r *RefHeader) setNextValid()             { r.offsetAndFlags |= uint32(NextValid) }
func (r *RefHeader) nextValidUint() uint       { return uint(1 & (r.offsetAndFlags >> Log2NextValid)) }

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

//go:generate gentemplate -d Package=hw -id Ref -d VecType=RefVec -d Type=Ref github.com/platinasystems/elib/vec.tmpl

const (
	// Cache aligned/sized space for buffer header.
	BufferHeaderBytes = cpu.CacheLineBytes
	// Rewrite (prepend) area.
	BufferRewriteBytes = 128
	overheadBytes      = BufferHeaderBytes + BufferRewriteBytes
)

// Buffer header.
type BufferHeader struct {
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

var defaultRef = Ref{RefHeader: RefHeader{dataOffset: BufferRewriteBytes}}
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
	Ref:  defaultRef,
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
		DmaFree(p.memChunkIDs[i])
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

func (p *BufferPool) AllocRefs(r *RefHeader, n uint) { p.AllocRefsStride(r, n, 1) }

func (p *BufferPool) AllocRefsStride(r *RefHeader, n, stride uint) {
	var got, want uint
	if got, want = uint(len(p.refs)), n; got < want {
		n := uint(elib.RoundPow2(elib.Word(want-got), 512))
		b := p.sizeIncludingOverhead
		_, id, offset, _ := DmaAlloc(n * b)
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

	pr := p.refs[got-want : got]

	if stride == 1 {
		refs := r.slice(n)
		copy(refs, pr)
	} else {
		l := n * stride
		refs := r.slice(l)
		i, ri := uint(0), uint(0)
		for i+4 < n {
			refs[ri+0*stride] = pr[i+0]
			refs[ri+1*stride] = pr[i+1]
			refs[ri+2*stride] = pr[i+2]
			refs[ri+3*stride] = pr[i+3]
			i += 4
			ri += 4 * stride
		}
		for i < n {
			refs[ri+0*stride] = pr[i+0]
			i += 1
			ri += 1 * stride
		}
	}

	p.refs = p.refs[:got-want]
}

type freeNextRefs struct {
	count uint
	refs  [1024]Ref
}

func (f *freeNextRefs) flush(p *BufferPool) {
	if f.count > 0 {
		p.FreeRefs(&f.refs[0].RefHeader, f.count)
		f.count = 0
	}
}

func (f *freeNextRefs) add(p *BufferPool, r *Ref, nextRef RefHeader) {
	f.refs[f.count].RefHeader = nextRef
	f.count += r.nextValidUint()
	if f.count >= uint(len(f.refs)) {
		f.flush(p)
	}
}

// Return all buffers to pool and reset for next usage.
func (p *BufferPool) FreeRefs(rh *RefHeader, n uint) {
	toFree := rh.slice(n)
	l := uint(len(p.refs))
	p.refs.Resize(n)
	r := p.refs[l:]

	t := p.Ref
	i := 0
	var f freeNextRefs
	for n >= 4 {
		r0, r1, r2, r3 := &toFree[i+0], &toFree[i+1], &toFree[i+2], &toFree[i+3]
		b0, b1, b2, b3 := r0.GetBuffer(), r1.GetBuffer(), r2.GetBuffer(), r3.GetBuffer()
		r[i+0], r[i+1], r[i+2], r[i+3] = t, t, t, t
		r[i+0].copyOffset(r0)
		r[i+1].copyOffset(r1)
		r[i+2].copyOffset(r2)
		r[i+3].copyOffset(r3)
		n0, n1, n2, n3 := b0.nextRef, b1.nextRef, b2.nextRef, b3.nextRef
		b0.reset(p)
		b1.reset(p)
		b2.reset(p)
		b3.reset(p)
		i += 4
		n -= 4
		if RefFlag4(NextValid, r0.h(), r1.h(), r2.h(), r3.h()) {
			f.add(p, r0, n0)
			f.add(p, r1, n1)
			f.add(p, r2, n2)
			f.add(p, r3, n3)
		}
	}

	for n > 0 {
		r0 := &toFree[i+0]
		b0 := r0.GetBuffer()
		r[i+0] = t
		r[i+0].copyOffset(r0)
		n0 := b0.nextRef
		b0.reset(p)
		i += 1
		n -= 1
		if RefFlag1(NextValid, r0.h()) {
			f.add(p, r0, n0)
		}
	}

	f.flush(p)

	p.InitRefs(r)
}

// Chains of buffer references.
type RefChain struct {
	// Number of bytes in chain.
	len uint64
	// Head and tail buffer reference.
	head Ref
	tail RefHeader
}

func (c *RefChain) Len() uint64 { return c.len }
func (c *RefChain) Head() *Ref  { return &c.head }

// Return buffer head and reset for later re-use.
func (c *RefChain) Done() (h Ref) {
	h = c.head
	c.head = Ref{}
	c.tail = RefHeader{}
	return
}

func (c *RefChain) Append(r *RefHeader) {
	tail := r
	if c.len == 0 {
		c.len = uint64(r.dataLen)
		c.head.RefHeader = *r
		tail = &c.head.RefHeader
	} else {
		// Point current tail to given reference.
		b := c.tail.GetBuffer()
		b.nextRef = *r
		c.head.setNextValid()
		c.len += uint64(r.dataLen)
	}
	for {
		// End of chain for reference to be added?
		if x := tail.NextRef(); x == nil {
			c.tail = *tail
			break
		} else {
			c.len += uint64(x.dataLen)
			tail = x
		}
	}
}
