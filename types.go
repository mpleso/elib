package elib

// Vector capacities of the form 2^i + 2^j
type Cap uint32

// True if removing first set bit gives a power of 2.
func (c Cap) IsValid() bool {
	f := c & -c
	c ^= f
	return 0 == c&(c-1)
}

func (n Cap) Round(log2Unit Cap) Cap {
	// Power of 2?
	if n&(n-1) != 0 {
		u := Word(1<<log2Unit - 1)
		w := (Word(n) + u) &^ u
		l0 := MinLog2(w)
		m0 := Word(1) << l0
		l1 := MaxLog2(w ^ m0)
		n = Cap(m0 + 1<<l1)
	}
	return n
}

func (c Cap) Pow2() (i, j Cap) {
	j = c & -c
	i = c ^ j
	if i == 0 {
		i = j
		j = ^Cap(0)
	}
	return
}

func (c Cap) Log2() (i, j Cap) {
	i, j = c.Pow2()
	i = Cap(Word(i).MinLog2())
	if j != ^Cap(0) {
		j = Cap(Word(j).MinLog2())
	}
	return
}

func (c Cap) NextUnit(log2Min, log2Unit Cap) (n Cap) {
	n = c

	// Double every 2/8/16 expands depending on table size.
	min := Cap(1)<<log2Min - 1
	switch {
	case n < min:
		n = min
	case n < 256:
		n = Cap(float64(n) * 1.41421356237309504878) /* exp (log2 / 2) */
	case n < 1024:
		n = Cap(float64(n) * 1.09050773266525765919) /* exp (log2 / 8) */
	default:
		n = Cap(float64(n) * 1.04427378242741384031) /* exp (log2 / 16) */
	}

	n = n.Round(log2Unit)
	return
}
func (c Cap) Next() Cap { return c.NextUnit(3, 2) }

// NextResizeCap gives next larger resizeable array capacity.
func NextResizeCap(x Index) Index { return Index(Cap(x).Next()) }

//go:generate gentemplate -d Package=elib -id Byte  -d VecType=ByteVec -d Type=byte vec.tmpl

//go:generate gentemplate -d Package=elib -id String -d VecType=StringVec -d Type=string vec.tmpl
//go:generate gentemplate -d Package=elib -id String -d PoolType=StringPool -d Type=string -d Data=Strings pool.tmpl

//go:generate gentemplate -d Package=elib -id Int64 -d VecType=Int64Vec -d Type=int64 vec.tmpl
//go:generate gentemplate -d Package=elib -id Int32 -d VecType=Int32Vec -d Type=int32 vec.tmpl
//go:generate gentemplate -d Package=elib -id Int16 -d VecType=Int16Vec -d Type=int16 vec.tmpl
//go:generate gentemplate -d Package=elib -id Int8  -d VecType=Int8Vec -d Type=int8  vec.tmpl

//go:generate gentemplate -d Package=elib -id Uint64 -d VecType=Uint64Vec -d Type=uint64 vec.tmpl
//go:generate gentemplate -d Package=elib -id Uint32 -d VecType=Uint32Vec -d Type=uint32 vec.tmpl
//go:generate gentemplate -d Package=elib -id Uint16 -d VecType=Uint16Vec -d Type=uint16 vec.tmpl
//go:generate gentemplate -d Package=elib -id Uint8  -d VecType=Uint8Vec -d Type=uint8  vec.tmpl
