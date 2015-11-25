//+build uio_pci_dma

package hw

import (
	"github.com/platinasystems/elib"

	"fmt"
	"syscall"
	"unsafe"
)

const (
	log2BytesPerCacheLine = 6
	cacheLineBytes        = 1 << log2BytesPerCacheLine
)

// Allocate memory in aligned cache lines.
type cacheLine [cacheLineBytes]byte

type heap struct {
	// Allocation heap of cache lines.
	elib.Heap

	// /dev/uio-dma
	fd int

	// Virtual address lines.
	lines []cacheLine

	// Chunks are 2^log2LinesPerChunk cache lines long.
	// Kernel gives us memory in "Chunks" which are physically contiguous.
	log2LinesPerChunk, log2BytesPerChunk uint8

	pageTable []uintptr
}

func (h *heap) Alloc(nBytes uint) (p unsafe.Pointer, id elib.Index) {
	nLines := (nBytes + cacheLineBytes - 1) &^ (cacheLineBytes - 1)
	id, i := h.Get(nLines)

	p = unsafe.Pointer(&h.lines[i])

	return
}

func (h *heap) Free(id elib.Index) { h.Put(id) }

const (
	uio_dma_cache_default = iota
	uio_dma_cache_disable
	uio_dma_cache_writecombine
)

const (
	uio_dma_bidirectional = iota
	uio_dma_todevice
	uio_dma_fromdevice
)

const (
	uio_dma_alloc = 0x400455c8
	uio_dma_free  = 0x400455c9
	uio_dma_map   = 0x400455ca
	uio_dma_unmap = 0x400455cb
)

type uio_dma_alloc_req struct {
	dma_mask    uint64
	memnode     uint16
	cache       uint16
	flags       uint32
	chunk_count uint32
	chunk_size  uint32
	mmap_offset uint64
}

type uio_dma_free_req struct {
	mmap_offset uint64
}

type uio_dma_map_req struct {
	mmap_offset uint64
	flags       uint32
	devid       uint32
	direction   uint32
	chunk_count uint32
	chunk_size  uint32
	dma_addr    [0]uint64
}

type uio_dma_unmap_req struct {
	mmap_offset uint64
	devid       uint32
	flags       uint32
	direction   uint32
}

func (h *heap) init() (err error) {
	h.fd, err = syscall.Open("/dev/uio-dma", syscall.O_RDWR, 0)
	if err != nil {
		return
	}
	defer func() {
		if err != nil && h.fd != 0 {
			syscall.Close(h.fd)
		}
	}()

	r := uio_dma_alloc_req{}
	r.dma_mask = uint64(^uintptr(0))

	sz := uint32(16 << 20)
	ok := false
	for r.chunk_size = sz; !ok; r.chunk_size /= 2 {
		_, _, e := syscall.RawSyscall(syscall.SYS_IOCTL, uintptr(h.fd), uintptr(uio_dma_alloc), uintptr(unsafe.Pointer(&r)))
		ok = e == 0
		r.chunk_count = sz / r.chunk_size
		if r.chunk_size == 4<<10 {
			return fmt.Errorf("ioctl UIO_DMA_ALLOC fails: %s", e)
		}
	}

	data, err := syscall.Mmap(h.fd, 0, int(sz), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("uio-dma mmap: %s", err)
	}

	h.lines = *(*[]cacheLine)(unsafe.Pointer(&data[0]))
	h.lines = h.lines[:sz>>log2BytesPerCacheLine]

	var buf [4096]byte
	m := (*uio_dma_map_req)(unsafe.Pointer(&buf[0]))
	m.direction = uio_dma_bidirectional
	m.chunk_size = r.chunk_size
	m.chunk_count = r.chunk_count
	_, _, e := syscall.RawSyscall(syscall.SYS_IOCTL, uintptr(h.fd), uintptr(uio_dma_map), uintptr(unsafe.Pointer(&m)))
	if e != 0 {
		return fmt.Errorf("uio-dma-map: %s", e)
	}

	h.log2BytesPerChunk = uint8(elib.MinLog2(elib.Word(r.chunk_size)))
	h.log2LinesPerChunk = h.log2BytesPerChunk - log2BytesPerCacheLine
	h.pageTable = make([]uintptr, r.chunk_count)
	for i := range h.pageTable {
		h.pageTable[i] = uintptr(m.dma_addr[i])
	}

	return err
}

var defaultHeap = &heap{}

func Alloc(nBytes uint) (p unsafe.Pointer, id elib.Index) { return defaultHeap.Alloc(nBytes) }
func Free(id elib.Index)                                  { defaultHeap.Free(id) }
func AllocInit() (err error)                              { return defaultHeap.init() }
