package hw

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"

	"fmt"
	"math"
	"reflect"
	"sync"
	"unsafe"
)

type BufferFlag uint32

const (
	NextValid, Log2NextValid BufferFlag = 1 << iota, iota
	Cloned, Log2Cloned
)

var bufferFlagStrings = [...]string{
	Log2NextValid: "next-valid",
	Log2Cloned:    "cloned",
}

func (f BufferFlag) String() string { return elib.FlagStringer(bufferFlagStrings[:], elib.Word(f)) }

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
func (r *RefHeader) NextIsValid() bool         { return r.NextValidFlag() != 0 }
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

func (r *RefHeader) DataLen() uint     { return uint(r.dataLen) }
func (r *RefHeader) SetDataLen(l uint) { r.dataLen = uint16(l) }
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
	mu sync.Mutex

	BufferTemplate

	// References to buffers in this pool.
	refs RefVec

	// DMA memory chunks used by this pool.
	memChunkIDs []elib.Index

	// Number of bytes of dma memory allocated by this pool.
	DmaMemAllocBytes uint64

	freeNext freeNext

	stateByOffset map[uint32]allocState
}

type allocState uint8

const (
	allocStateUnknown = iota
	allocStateKnownAllocated
	allocStateKnownFree
)

var allocStateStrings = [...]string{
	allocStateUnknown:        "unkown",
	allocStateKnownAllocated: "known-allocated",
	allocStateKnownFree:      "known-free",
}

func (s allocState) String() string { return elib.Stringer(allocStateStrings[:], int(s)) }

func (p *BufferPool) setState(r *Ref, new allocState) (old allocState) {
	if p.stateByOffset == nil {
		p.stateByOffset = make(map[uint32]allocState)
	}
	o := r.offset()
	old = p.stateByOffset[o]
	p.stateByOffset[o] = new
	return
}

func (p *BufferPool) getState(r *Ref) allocState { return p.stateByOffset[r.offset()] }

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

func (t *BufferTemplate) SizeIncludingOverhead() uint { return t.sizeIncludingOverhead }

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
	p.DmaMemAllocBytes = 0
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

func (p *BufferPool) FreeLen() uint { return uint(len(p.refs)) }

func (p *BufferPool) AllocRefs(r *RefHeader, n uint) { p.AllocRefsStride(r, n, 1) }
func (p *BufferPool) AllocRefsStride(r *RefHeader, want, stride uint) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var got uint
	for {
		if got = p.FreeLen(); got >= want {
			break
		}
		b := p.sizeIncludingOverhead
		n_alloc := uint(elib.RoundPow2(elib.Word(want-got), 256))
		nb := n_alloc * b
		for nb > 1<<20 {
			n_alloc /= 2
			nb /= 2
		}
		_, id, offset, _ := DmaAlloc(nb)
		ri := got
		p.refs.Resize(n_alloc)
		p.memChunkIDs = append(p.memChunkIDs, id)
		p.DmaMemAllocBytes += uint64(nb)
		// Refs are allocated from end of refs so we put smallest offsets there.
		o := offset + (n_alloc-1)*b
		for i := uint(0); i < n_alloc; i++ {
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
		got += n_alloc
		// Possibly initialize/adjust newly made buffers.
		p.InitRefs(p.refs[got-n_alloc : got])
	}

	pr := p.refs[got-want : got]

	refs := r.slice(want * stride)
	if stride == 1 {
		copy(refs, pr)
	} else {
		i, ri := uint(0), uint(0)
		for i+4 < want {
			refs[ri+0*stride] = pr[i+0]
			refs[ri+1*stride] = pr[i+1]
			refs[ri+2*stride] = pr[i+2]
			refs[ri+3*stride] = pr[i+3]
			i += 4
			ri += 4 * stride
		}
		for i < want {
			refs[ri+0*stride] = pr[i+0]
			i += 1
			ri += 1 * stride
		}
	}

	p.refs = p.refs[:got-want]

	if elib.Debug {
		for i := range refs {
			s := p.setState(&refs[i], allocStateKnownAllocated)
			if s == allocStateKnownAllocated {
				panic("duplicate alloc")
			}
		}
	}
}

type freeNext struct {
	count uint
	refs  RefVec
}

func (f *freeNext) add(p *BufferPool, r *Ref, nextRef RefHeader) {
	h := &r.RefHeader
	if h.NextIsValid() {
		for {
			f.refs.Validate(f.count)
			f.refs[f.count].RefHeader = nextRef
			f.count++
			if !nextRef.NextIsValid() {
				break
			}
			b := nextRef.GetBuffer()
			h = &b.nextRef
			nextRef = *h
		}
	}
}

// Return all buffers to pool and reset for next usage.
func (p *BufferPool) FreeRefs(rh *RefHeader, n uint) {
	p.mu.Lock()
	defer p.mu.Unlock()
	toFree := rh.slice(n)

	if elib.Debug {
		for i := range toFree {
			s := p.setState(&toFree[i], allocStateKnownFree)
			if s != allocStateKnownAllocated {
				panic("duplicate free")
			}
		}
	}

	initialLen := p.FreeLen()
	p.refs.Resize(n)
	r := p.refs[initialLen:]

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
		n0, n1, n2, n3 := b0.nextRef, b1.nextRef, b2.nextRef, b3.nextRef
		b0.reset(p)
		b1.reset(p)
		b2.reset(p)
		b3.reset(p)
		i += 4
		n -= 4
		if RefFlag4(NextValid, r0.h(), r1.h(), r2.h(), r3.h()) {
			p.freeNext.add(p, r0, n0)
			p.freeNext.add(p, r1, n1)
			p.freeNext.add(p, r2, n2)
			p.freeNext.add(p, r3, n3)
		}
	}

	for n > 0 {
		r0 := &toFree[i+0]
		b0 := r0.GetBuffer()
		r0.Validate()
		r[i+0] = t
		r[i+0].copyOffset(r0)
		n0 := b0.nextRef
		b0.reset(p)
		i += 1
		n -= 1
		if RefFlag1(NextValid, r0.h()) {
			p.freeNext.add(p, r0, n0)
		}
	}

	if f := &p.freeNext; f.count > 0 {
		n = f.count
		f.count = 0
		l := uint(len(p.refs))
		p.refs.Resize(n)
		r := p.refs[l:]

		i = 0
		for n >= 4 {
			r0, r1, r2, r3 := &f.refs[i+0], &f.refs[i+1], &f.refs[i+2], &f.refs[i+3]
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
			r0 := &f.refs[i+0]
			b0 := r0.GetBuffer()
			r[i+0] = t
			r[i+0].copyOffset(r0)
			b0.reset(p)
			i += 1
			n -= 1
		}
	}

	p.InitRefs(p.refs[initialLen:])
}

func (r *RefHeader) String() (s string) {
	s = ""
	for {
		if s != "" {
			s += ", "
		}
		s += "{"
		s += fmt.Sprintf("0x%x+%d, %d bytes", r.offset(), r.dataOffset, r.dataLen)
		if f := r.Flags(); f != 0 {
			s += ", " + f.String()
		}
		var ok bool
		if _, ok = DmaIsValidOffset(uint(r.offset() + uint32(r.dataOffset))); !ok {
			s += ", bad-offset"
		}
		s += "}"
		if !ok {
			break
		}
		if r = r.NextRef(); r == nil {
			break
		}
	}
	return
}

func (h *RefHeader) Validate() {
	if !elib.Debug {
		return
	}
	var err error
	defer func() {
		if err != nil {
			panic(fmt.Errorf("%s %s", h, err))
		}
	}()
	r := h
	for {
		if offset, ok := DmaIsValidOffset(uint(r.offset() + uint32(r.dataOffset))); !ok {
			err = fmt.Errorf("bad dma offset: %x", offset)
			return
		}
		if r = r.NextRef(); r == nil {
			break
		}
	}
}

// Chains of buffer references.
type RefChain struct {
	// Number of bytes in chain.
	len   uint32
	count uint32
	// Head and tail buffer reference.
	head            Ref
	tail, prev_tail *RefHeader
}

func (c *RefChain) Head() *Ref          { return &c.head }
func (c *RefChain) Len() uint           { return uint(c.len) }
func (c *RefChain) addLen(r *RefHeader) { c.len += uint32(r.dataLen) }

func (c *RefChain) Append(r *RefHeader) {
	c.addLen(r)
	if c.tail == nil {
		c.tail = &c.head.RefHeader
	}
	*c.tail = *r
	if c.prev_tail != nil {
		c.prev_tail.setNextValid()
	}
	tail := r
	for {
		// End of chain for reference to be added?
		if x := tail.NextRef(); x == nil {
			c.prev_tail, c.tail = c.tail, &tail.GetBuffer().nextRef
			break
		} else {
			c.addLen(x)
			tail = x
		}
	}
}

// Length in buffer chain.
func (r *RefHeader) TotalLen() (l uint) {
	for {
		l += r.DataLen()
		if r = r.NextRef(); r == nil {
			break
		}
	}
	return
}

func (r *RefHeader) validateTotalLen(want uint) (l uint, ok bool) {
	for {
		l += r.DataLen()
		if r = r.NextRef(); r == nil {
			ok = true
			return
		}
		if l > want {
			ok = false
			return
		}
	}
}

func (c *RefChain) Validate() {
	if !elib.Debug {
		return
	}
	want := c.Len()
	got, ok := c.head.validateTotalLen(want)
	if !ok || got != want {
		panic(fmt.Errorf("length mismatch; got %d != want %d", got, want))
	}
	c.head.Validate()
}
