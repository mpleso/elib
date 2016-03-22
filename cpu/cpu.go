// +build !amd64

package cpu

import (
	"time"
)

// Cache lines on generic.
const Log2CacheLineBytes = 6

func TimeNow() uint64 {
	return uint64(time.Now().UnixNano())
}
