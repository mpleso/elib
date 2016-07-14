//+build !uio_pci_dma

package hw

import (
	"github.com/platinasystems/elib"

	"unsafe"
)

var memHeap = &elib.MemHeap{}

func DmaInit(n uint) {
	if err := memHeap.Init(n); err != nil {
		panic(err)
	}
}

func DmaAlloc(n uint) (b []byte, id elib.Index, offset, cap uint) { return memHeap.Get(n) }
func DmaFree(id elib.Index)                                       { memHeap.Put(id) }
func DmaGet(id elib.Index) (b []byte)                             { return memHeap.GetId(id) }
func DmaPhysAddress(a uintptr) uintptr                            { return a }
func DmaOffset(b []byte) uint                                     { return memHeap.Offset(b) }
func DmaGetOffset(o uint) unsafe.Pointer                          { return memHeap.Data(o) }