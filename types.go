package elib

// nextCap gives next larger resizeable array capacity.
// FIXME make this better
func NextResizeCap(x Index) Index {
	return Index(MaxPow2(Word(x)))
}

//go:generate gentemplate -d Package=elib -id Byte  -d Type=byte vec.tmpl

//go:generate gentemplate -d Package=elib -id String -d Type=string vec.tmpl
//go:generate gentemplate -d Package=elib -id String -d Type=StringVec -d Data=Strings pool.tmpl

//go:generate gentemplate -d Package=elib -id Int64 -d Type=int64 vec.tmpl
//go:generate gentemplate -d Package=elib -id Int32 -d Type=int32 vec.tmpl
//go:generate gentemplate -d Package=elib -id Int16 -d Type=int16 vec.tmpl
//go:generate gentemplate -d Package=elib -id Int8  -d Type=int8  vec.tmpl

//go:generate gentemplate -d Package=elib -id Uint64 -d Type=uint64 vec.tmpl
//go:generate gentemplate -d Package=elib -id Uint32 -d Type=uint32 vec.tmpl
//go:generate gentemplate -d Package=elib -id Uint16 -d Type=uint16 vec.tmpl
//go:generate gentemplate -d Package=elib -id Uint8  -d Type=uint8  vec.tmpl
