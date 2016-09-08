// +build uio_pci_dma

package hw

import (
	"github.com/platinasystems/elib"

	"unsafe"
)

type pageTable struct {
	Data []byte

	Pages []uintptr

	Log2BytesPerPage uint
}

var PageTable pageTable

func DmaPhysAddress(a uintptr) uintptr {
	t := &PageTable
	l := t.Log2BytesPerPage
	o := a - uintptr(unsafe.Pointer(&t.Data[0]))
	return t.Pages[o>>l] + o&(1<<(l-1))
}

func (t *pageTable) spansPage(o, n uint) bool {
	l := t.Log2BytesPerPage
	return o>>l == (o+n-1)>>l
}

func DmaAllocAligned(n, log2Align uint) (b []byte, id elib.Index, offset, cap uint) {
	t := &PageTable
	f := []elib.Index{}
	for {
		b, id, offset, cap = heap.GetAligned(n, log2Align)
		if !t.spansPage(offset, n) {
			break
		}
		f = append(f, id)
	}
	for _, x := range f {
		heap.Put(x)
	}
	return
}
