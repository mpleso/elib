package elib

import (
	"fmt"
	"reflect"
	"unsafe"
)

type TypedPool struct {
	// Allocation free list and free bitmap.
	pool Pool

	// Object type and data vectors.
	object_types ByteVec
	object_data  ByteVec

	// Size of largest type in bytes.
	object_size uint32

	// Unused data is poisoned; new data is zeroed when allocated.
	poison, zero    []byte
	poison_zero_buf [128]byte
}

func (p *TypedPool) Init(args ...interface{}) {
	for i := range args {
		if x := uint32(reflect.TypeOf(args[i]).Size()); x > p.object_size {
			p.object_size = x
		}
	}

	l := int(p.object_size)
	b := p.poison_zero_buf[:]
	if 2*l > len(b) {
		b = make([]byte, 2*l)
	}
	p.zero = b[0:l]
	p.poison = b[l : 2*l]
	dead := [...]byte{0xde, 0xad}
	for i := range p.poison {
		p.poison[i] = dead[i%len(dead)]
	}
}

func (p *TypedPool) GetIndex(typ uint) (i uint) {
	i = p.pool.GetIndex(uint(len(p.object_types)))
	p.object_types.Validate(i)
	p.object_types[i] = byte(typ)
	j := uint32(i) * p.object_size
	p.object_data.Validate(uint(j + p.object_size - 1))
	copy(p.object_data[j:j+p.object_size], p.zero)
	return
}

func (p *TypedPool) PutIndex(t, i uint) (ok bool) {
	ok = p.object_types[i] == byte(t)
	if !ok {
		return
	}
	ok = p.pool.PutIndex(i)
	if !ok {
		return
	}
	p.object_types[i] = 0
	j := uint32(i) * p.object_size
	copy(p.object_data[j:j+p.object_size], p.poison)
	return
}

func (p *TypedPool) GetData(t, i uint) unsafe.Pointer {
	if want := uint(p.object_types[i]); want != t {
		panic(fmt.Errorf("wrong type want %d != got %d", want, t))
	}
	return unsafe.Pointer(&p.object_data[uint32(i)*p.object_size])
}

func (p *TypedPool) Data(i uint) (t uint, x unsafe.Pointer) {
	t = uint(p.object_types[i])
	x = unsafe.Pointer(&p.object_data[uint32(i)*p.object_size])
	return
}
