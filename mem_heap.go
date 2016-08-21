package elib

import (
	"github.com/platinasystems/elib/cpu"

	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

func (x Word) RoundCacheLine() Word { return x.RoundPow2(cpu.CacheLineBytes) }
func RoundCacheLine(x Word) Word    { return x.RoundCacheLine() }

// Allocation heap of cache lines.
type MemHeap struct {
	heap Heap

	once sync.Once

	// Virtual address lines returned via mmap of anonymous memory.
	data []byte
}

// Init initializes heap with n bytes of mmap'ed anonymous memory.
func (h *MemHeap) init(n uint) (err error) {
	n = uint(Word(n).RoundCacheLine())
	h.data, err = syscall.Mmap(0, 0, int(n), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON|syscall.MAP_NORESERVE)
	if err != nil {
		return fmt.Errorf("mmap: %s", err)
	}
	h.heap.SetMaxLen(n >> cpu.Log2CacheLineBytes)
	return err
}

func (h *MemHeap) Init(n uint) (err error) {
	h.once.Do(func() { err = h.init(n) })
	return
}

func (h *MemHeap) Get(n uint) (b []byte, id Index, offset, cap uint) {
	// Allocate memory in case caller has not called Init to select a size.
	if err := h.Init(64 << 20); err != nil {
		panic(err)
	}

	cap = uint(Word(n).RoundCacheLine())
	id, i := h.heap.Get(cap >> cpu.Log2CacheLineBytes)
	offset = uint(i) << cpu.Log2CacheLineBytes
	b = h.data[offset : offset+cap]
	return
}

func (h *MemHeap) Put(id Index) { h.heap.Put(id) }

func (h *MemHeap) GetId(id Index) (b []byte) {
	offset, len := h.heap.GetID(id)
	return h.data[offset : offset+len]
}

func (h *MemHeap) Offset(b []byte) uint {
	return uint(uintptr(unsafe.Pointer(&b[0])) - uintptr(unsafe.Pointer(&h.data[0])))
}

func (h *MemHeap) Data(o uint) unsafe.Pointer { return unsafe.Pointer(&h.data[o]) }
func (h *MemHeap) OffsetValid(o uint) bool    { return o < uint(len(h.data)) }

func (h *MemHeap) String() string {
	max := h.heap.GetMaxLen()
	if max == 0 {
		return "empty"
	}
	u := h.heap.GetUsage()
	return fmt.Sprintf("used %s, free %s, capacity %s",
		MemorySize(u.Used<<cpu.Log2CacheLineBytes),
		MemorySize(u.Free<<cpu.Log2CacheLineBytes),
		MemorySize(max<<cpu.Log2CacheLineBytes))
}
