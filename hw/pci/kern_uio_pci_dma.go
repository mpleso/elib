//+build uio_pci_dma

package pci

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/elib/iomux"

	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

type uioPciDmaMain struct {
	// Allocation heap of cache lines.
	mu   sync.Mutex
	heap elib.Heap

	// /dev/uio-dma
	uio_dma_fd int

	// Virtual address lines.
	data []byte

	// Chunks are 2^log2LinesPerChunk cache lines long.
	// Kernel gives us memory in "Chunks" which are physically contiguous.
	log2LinesPerChunk, log2BytesPerChunk uint8

	pageTable []uintptr

	// So that heapInit is called exactly once when first device is initialized.
	heapInitOnce sync.Once
}

// Checks whether objects with offset and size spans a physical chunk boundary.
// Objects that span boundaries cannot be used for DMA.
func (h *uioPciDmaMain) chunkIndexForOffset(byteOffset, nBytes uintptr) (chunkIndex uintptr, ok bool) {
	chunkIndex = byteOffset >> h.log2BytesPerChunk
	hi := (byteOffset + nBytes - 1) >> h.log2BytesPerChunk
	ok = chunkIndex == hi
	return
}

func (h *uioPciDmaMain) alloc(ask uint) (b []byte, id elib.Index, offset uint, cap uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	nBytes := uintptr(elib.Word(ask).RoundCacheLine())
	nLines := uint(nBytes >> cpu.Log2CacheLineBytes)

	idsToFree := []elib.Index{}
	defer func() {
		if idsToFree != nil {
			for _, fid := range idsToFree {
				h.heap.Put(fid)
			}
		}
	}()

	for {
		var lineIndex uint
		id, lineIndex = h.heap.Get(nLines)

		lo := uintptr(lineIndex) << cpu.Log2CacheLineBytes

		if _, ok := h.chunkIndexForOffset(lo, nBytes); ok {
			b = h.data[lo : lo+nBytes]
			cap = uint(nBytes)
			offset = uint(lo)
			return
		}
		idsToFree = append(idsToFree, id)
	}

	return
}

func (h *uioPciDmaMain) free(id elib.Index) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.heap.Put(id)
}

func (h *uioPciDmaMain) get(id elib.Index) (b []byte) {
	offset, len := h.heap.GetID(id)
	return h.data[offset : offset+len]
}

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
	dma_addr    [256]uint64
}

type uio_dma_unmap_req struct {
	mmap_offset uint64
	devid       uint32
	flags       uint32
	direction   uint32
}

func (h *uioPciDmaMain) heapInit(uioMinorDevice uint32, maxSize uint) (err error) {
	h.uio_dma_fd, err = syscall.Open("/dev/uio-dma", syscall.O_RDWR, 0)
	if err != nil {
		return
	}
	defer func() {
		if err != nil && h.uio_dma_fd != 0 {
			syscall.Close(h.uio_dma_fd)
		}
	}()

	r := uio_dma_alloc_req{}
	r.dma_mask = 0xffffffff

	r.chunk_size = uint32(maxSize)
	for {
		r.chunk_count = uint32(maxSize) / r.chunk_size
		_, _, e := syscall.RawSyscall(syscall.SYS_IOCTL, uintptr(h.uio_dma_fd), uintptr(uio_dma_alloc), uintptr(unsafe.Pointer(&r)))
		if e == 0 {
			break
		}
		if r.chunk_size == 4<<10 {
			return fmt.Errorf("ioctl UIO_DMA_ALLOC fails: %s", e)
		}
		r.chunk_size /= 2
	}

	h.data, err = syscall.Mmap(h.uio_dma_fd, int64(r.mmap_offset), int(maxSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("uio-dma mmap: %s", err)
	}

	m := uio_dma_map_req{}
	m.direction = uio_dma_bidirectional
	m.chunk_size = r.chunk_size
	m.chunk_count = r.chunk_count
	m.mmap_offset = r.mmap_offset
	m.devid = uint32(uioMinorDevice)
	_, _, e := syscall.RawSyscall(syscall.SYS_IOCTL, uintptr(h.uio_dma_fd), uintptr(uio_dma_map), uintptr(unsafe.Pointer(&m)))
	if e != 0 {
		return fmt.Errorf("uio-dma-map: %s", e)
	}

	h.log2BytesPerChunk = uint8(elib.MinLog2(elib.Word(r.chunk_size)))
	h.log2LinesPerChunk = h.log2BytesPerChunk - cpu.Log2CacheLineBytes
	h.pageTable = make([]uintptr, r.chunk_count)
	for i := range h.pageTable {
		h.pageTable[i] = uintptr(m.dma_addr[i])
	}

	h.heap.SetMaxLen(maxSize >> cpu.Log2CacheLineBytes)

	return err
}

var uioPciDma = &uioPciDmaMain{}

func DmaAlloc(nBytes uint) (b []byte, id elib.Index, offset, cap uint) { return uioPciDma.alloc(nBytes) }
func DmaFree(id elib.Index)                                            { uioPciDma.free(id) }
func DmaGet(id elib.Index) (b []byte)                                  { return uioPciDma.get(id) }
func DmaOffset(b []byte) uint {
	return uint(uintptr(unsafe.Pointer(&b[0])) - uintptr(unsafe.Pointer(&uioPciDma.data[0])))
}
func DmaGetOffset(o uint) unsafe.Pointer { return unsafe.Pointer(&uioPciDma.data[o]) }

// Returns caller physical address of given virtual address.
// Physical address will be suitable for hardware DMA.
func DmaPhysAddress(a uintptr) uintptr {
	m := uioPciDma
	offset := a - uintptr(unsafe.Pointer(&m.data[0]))
	return m.pageTable[offset>>m.log2BytesPerChunk] + offset&(1<<m.log2BytesPerChunk-1)
}

type uioPciDevice struct {
	Device

	// /dev/uioN
	iomux.File

	index uint32

	uioMinorDevice uint32
}

func sysfsWrite(path, format string, args ...interface{}) error {
	fn := "/sys/bus/pci/drivers/uio_pci_dma/" + path
	f, err := os.OpenFile(fn, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, format, args...)
	return err
}

func (d *uioPciDevice) bind() (err error) {
	err = sysfsWrite("new_id", "%04x %04x", int(d.VendorID()), int(d.DeviceID()))
	if err != nil {
		return
	}

	err = sysfsWrite("bind", "%s", &d.Addr)
	if err != nil {
		return
	}

	var fis []os.FileInfo
	fis, err = ioutil.ReadDir(d.SysfsPath("uio"))
	if err != nil {
		return
	}

	ok := false
	for _, fi := range fis {
		if _, err = fmt.Sscanf(fi.Name(), "uio%d", &d.uioMinorDevice); err == nil {
			ok = true
			break
		}
	}
	if !ok {
		err = fmt.Errorf("failed to get minor number for uio device")
		return
	}

	return
}

func NewDevice() Devicer {
	d := &uioPciDevice{}
	d.Device.Devicer = d
	return d
}

func (d *uioPciDevice) GetDevice() *Device { return &d.Device }

func (d *uioPciDevice) Open() (err error) {
	err = d.bind()
	if err != nil {
		return
	}

	uioPath := fmt.Sprintf("/dev/uio%d", d.uioMinorDevice)
	d.File.Fd, err = syscall.Open(uioPath, syscall.O_RDONLY, 0)
	if err != nil {
		panic(fmt.Errorf("open %s: %s", uioPath, err))
	}

	// Initialize DMA heap once device is open.
	m := uioPciDma
	m.heapInitOnce.Do(func() {
		err = m.heapInit(d.uioMinorDevice, 16<<20)
	})
	if err != nil {
		panic(err)
	}

	// Listen for interrupts.
	iomux.Add(d)

	return
}

var errShouldNeverHappen = errors.New("should never happen")

func (d *uioPciDevice) ErrorReady() error    { return errShouldNeverHappen }
func (d *uioPciDevice) WriteReady() error    { return errShouldNeverHappen }
func (d *uioPciDevice) WriteAvailable() bool { return false }
func (d *uioPciDevice) String() string       { return "pci " + d.Device.String() }

// UIO file is ready when interrupt occurs.
func (d *uioPciDevice) ReadReady() (err error) {
	d.DriverDevice.Interrupt()
	return
}
