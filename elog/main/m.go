package main

import (
	"github.com/platinasystems/elib/elog"

	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

// Event logging.
type event struct {
	i uint32
}

func (e *event) String() string {
	return fmt.Sprintf("event #%d", e.i)
}

//go:generate gentemplate -d Package=main -id event -d Type=event github.com/platinasystems/elib/elog/event.tmpl

func (e *event) Encode(b []byte) int {
	return binary.PutUvarint(b, uint64(e.i))
}

func (e *event) Decode(b []byte) int {
	x, n := binary.Uvarint(b)
	e.i = uint32(x)
	return n
}

func main() {
	elog.Enable(true)
	for i := uint32(0); i < 10; i++ {
		e := event{i: i}
		e.Log()
	}
	var b bytes.Buffer

	elog.Print(os.Stdout)

	err := elog.DefaultLog.Save(&b)
	if err != nil {
		panic(err)
	}

	if nb, ne := b.Len(), elog.DefaultLog.Len(); ne > 0 {
		fmt.Printf("%d events, %d bytes, %.4f bytes/event\n", ne, nb, float64(nb)/float64(ne))
	}

	l := &elog.Log{}
	err = l.Restore(&b)
	if err != nil {
		panic(err)
	}

	l.Print(os.Stdout)
}
