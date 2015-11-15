package elog

import (
	"github.com/platinasystems/elib"

	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"math"
)

func Uvarint(b []byte) (c []byte, i int) {
	x, n := binary.Uvarint(b)
	i = int(x)
	c = b[n:]
	return
}

func PutUvarint(b []byte, i int) (c []byte) {
	n := binary.PutUvarint(b, uint64(i))
	c = b[n:]
	return
}

func (l *Log) Save(w io.Writer) (err error) {
	enc := gob.NewEncoder(w)
	err = enc.Encode(l)
	return
}

func (l *Log) Restore(r io.Reader) (err error) {
	dec := gob.NewDecoder(r)
	err = dec.Decode(l)
	return
}

type EventDataDecoder interface {
	Decode(b []byte) int
}

type EventDataEncoder interface {
	Encode(b []byte) int
}

func (e *Event) encode(b0 elib.ByteVec, eType uint16, t0 Time, i0 int) (b elib.ByteVec, t Time, i int) {
	b, i = b0, i0
	b.Validate(uint(i + 1<<log2EventBytes))
	// Encode time differences for shorter encodings.
	t = e.timestamp
	i += binary.PutUvarint(b[i:], uint64(t-t0))
	i += binary.PutUvarint(b[i:], uint64(eType))
	i += binary.PutUvarint(b[i:], uint64(e.Track))
	tp := e.getType()
	i += tp.Encoder(b[i:], e)
	return
}

var (
	errUnderflow = errors.New("decode buffer underflow")
)

func (e *Event) decode(b elib.ByteVec, typeMap elib.Uint16Vec, t0 Time, i0 int) (t Time, i int, err error) {
	i, t = i0, t0
	var (
		x  uint64
		n  int
		tp *EventType
	)

	if x, n = binary.Uvarint(b[i:]); n <= 0 {
		goto short
	}
	t += Time(x)
	e.timestamp = t
	i += n

	if x, n = binary.Uvarint(b[i:]); n <= 0 {
		goto short
	}
	if int(x) >= len(typeMap) {
		return 0, 0, fmt.Errorf("type index out of range %d >= %d", x, len(typeMap))
	}
	e.Type = typeMap[x]
	i += n

	if x, n = binary.Uvarint(b[i:]); n <= 0 {
		goto short
	}
	e.Track = uint16(x)
	i += n

	tp = getTypeByIndex(int(e.Type))
	i += tp.Decoder(b[i:], e)
	return

short:
	return 0, 0, errUnderflow
}

func (l *Log) MarshalBinary() ([]byte, error) {
	var b elib.ByteVec

	i := 0
	bo := binary.BigEndian

	b.Validate(uint(i + 8))
	bo.PutUint64(b[i:], math.Float64bits(l.timeUnitNsecs()))
	i += 8

	b.Validate(uint(i + binary.MaxVarintLen64))
	i += binary.PutUvarint(b[i:], uint64(l.cpuStartTime))

	d, err := l.StartTime.MarshalBinary()
	if err != nil {
		return nil, err
	}
	b.Validate(uint(i + len(b) + binary.MaxVarintLen64))
	i += binary.PutUvarint(b[i:], uint64(len(d)))
	i += copy(b[i:], d)

	b.Validate(uint(i + binary.MaxVarintLen64))
	i += binary.PutUvarint(b[i:], uint64(l.Len()))

	// Map global event types to log local ones.
	var localTypes elib.Uint16Vec
	var globalTypes elib.Uint32Vec

	typesUsed := elib.Bitmap(0)
	l.ForeachEvent(func(e *Event) {
		ti := uint(e.Type)
		if !typesUsed.Get(ti) {
			typesUsed = typesUsed.Orx(ti)
			globalTypes.Validate(ti)
			globalTypes[ti] = uint32(len(localTypes))
			localTypes = append(localTypes, e.Type)
		}
	})

	// Encode number of unique types followed by type names.
	b.Validate(uint(i + binary.MaxVarintLen64))
	i += binary.PutUvarint(b[i:], uint64(len(localTypes)))
	for x := range localTypes {
		t := getTypeByIndex(int(localTypes[x]))
		b.Validate(uint(i + binary.MaxVarintLen64 + len(t.Name)))
		i += binary.PutUvarint(b[i:], uint64(len(t.Name)))
		i += copy(b[i:], t.Name)
	}

	t := l.cpuStartTime
	l.ForeachEvent(func(e *Event) {
		b, t, i = e.encode(b, uint16(globalTypes[e.Type]), t, i)
	})

	return b[:i], nil
}

func (l *Log) UnmarshalBinary(b []byte) (err error) {
	i := 0
	bo := binary.BigEndian

	l.timeUnitNsec = math.Float64frombits(bo.Uint64(b[i:]))
	i += 8

	if x, n := binary.Uvarint(b[i:]); n > 0 {
		l.cpuStartTime = Time(x)
		i += n
	} else {
		return errUnderflow
	}

	if x, n := binary.Uvarint(b[i:]); n > 0 {
		i += n
		timeLen := int(x)
		if i+timeLen > len(b) {
			return errUnderflow
		}
		err = l.StartTime.UnmarshalBinary(b[i : i+timeLen])
		if err != nil {
			return err
		}
		i += timeLen
	} else {
		return errUnderflow
	}

	if x, n := binary.Uvarint(b[i:]); n > 0 {
		l.index = x
		i += n
	} else {
		return errUnderflow
	}

	var typeMap elib.Uint16Vec

	if x, n := binary.Uvarint(b[i:]); n > 0 {
		typeMap.Resize(uint(x))
		i += n
	} else {
		return errUnderflow
	}

	for li := range typeMap {
		if x, n := binary.Uvarint(b[i:]); n > 0 {
			i += n
			nameLen := int(x)
			if i+nameLen > len(b) {
				return errUnderflow
			}
			name := string(b[i : i+nameLen])
			i += nameLen
			if tp, ok := getTypeByName(name); !ok {
				return fmt.Errorf("unknown type named `%s'", name)
			} else {
				typeMap[li] = uint16(tp.index)
			}
		} else {
			return errUnderflow
		}
	}

	t := l.cpuStartTime
	for ei := 0; ei < int(l.index); ei++ {
		e := &l.events[ei]
		t, i, err = e.decode(b, typeMap, t, i)
		if err != nil {
			return
		}
	}

	b = b[:i]

	return
}

func (t *EventType) MarshalBinary() ([]byte, error) {
	return []byte(t.Name), nil
}

func (t *EventType) UnmarshalBinary(data []byte) (err error) {
	n := string(data)
	if rt, ok := getTypeByName(n); ok {
		*t = *rt
	} else {
		err = errors.New("unknown type: " + n)
	}
	return
}
