// Memory mapped register read/write
package hw

// Memory-mapped read/write
func LoadUint32(addr *uint32) (data uint32)
func StoreUint32(addr *uint32, data uint32)

func MemoryBarrier()

// Generic 32 bit register
type Reg32 uint32

func (r *Reg32) Get() uint32  { return LoadUint32((*uint32)(r)) }
func (r *Reg32) Set(x uint32) { StoreUint32((*uint32)(r), x) }
