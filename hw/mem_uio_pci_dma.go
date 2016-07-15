//+build uio_pci_dma

package hw

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/hw/pci"

	"unsafe"
)

func DmaInit(n uint)                                              {}
func DmaAlloc(n uint) (b []byte, id elib.Index, offset, cap uint) { return pci.DmaAlloc(n) }
func DmaFree(id elib.Index)                                       { pci.DmaFree(id) }
func DmaGet(id elib.Index) (b []byte)                             { return pci.DmaGet(id) }
func DmaPhysAddress(a uintptr) uintptr                            { return pci.DmaPhysAddress(a) }
func DmaOffset(b []byte) uint                                     { return pci.DmaOffset(b) }
func DmaGetOffset(o uint) unsafe.Pointer                          { return pci.DmaGetOffset(o) }
func DmaIsValidOffset(o uint) (uint, bool)                        { return o, pci.DmaIsValidOffset(o) }
