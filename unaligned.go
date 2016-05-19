package elib

import (
	"runtime"
	"unsafe"
)

const (
	supportsUnaligned = runtime.GOARCH == "386" || runtime.GOARCH == "amd64" || runtime.GOARCH == "ppc64" || runtime.GOARCH == "ppc64le" || runtime.GOARCH == "s390x"
	isBig             = runtime.GOARCH == "ppc64" || runtime.GOARCH == "mips64"
)

func unalignedUint16(p unsafe.Pointer) uint16 {
	if supportsUnaligned {
		return *(*uint16)(p)
	}
	f := func(i, j uint) uint16 {
		q := (*[2]byte)(p)
		return uint16(q[i]) << (8 * j)
	}
	if isBig {
		return f(0, 1) + f(1, 0)
	} else {
		return f(0, 0) + f(1, 1)
	}
}

func unalignedUint32(p unsafe.Pointer) uint32 {
	if supportsUnaligned {
		return *(*uint32)(p)
	}
	f := func(i, j uint) uint32 {
		q := (*[4]byte)(p)
		return uint32(q[i]) << (8 * j)
	}
	if isBig {
		return f(0, 3) + f(1, 2) + f(2, 1) + f(3, 0)
	} else {
		return f(0, 0) + f(1, 1) + f(2, 2) + f(3, 3)
	}
}

func unalignedUint64(p unsafe.Pointer) uint64 {
	if supportsUnaligned {
		return *(*uint64)(p)
	}
	f := func(i, j uint) uint64 {
		q := (*[8]byte)(p)
		return uint64(q[i]) << (8 * j)
	}
	if isBig {
		return f(0, 7) + f(1, 6) + f(2, 5) + f(3, 4) + f(4, 3) + f(5, 2) + f(6, 1) + f(7, 0)
	} else {
		return f(0, 0) + f(1, 1) + f(2, 2) + f(3, 3) + f(4, 4) + f(5, 5) + f(6, 6) + f(7, 7)
	}
}
