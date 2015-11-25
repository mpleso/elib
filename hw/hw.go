// Memory mapped register read/write
package hw

// Memory-mapped read/write
func LoadUint32(addr *uint32) (data uint32)
func StoreUint32(addr *uint32, data uint32)

func MemoryBarrier()
