package main

import (
	"github.com/platinasystems/elib/elog"

	"bytes"
	"fmt"
	"os"
)

// Event logging.
type event struct {
	i uint32
}

func (e *event) String() string          { return fmt.Sprintf("event #%d", e.i) }
func (e *event) Encode(b []byte) int     { return elog.EncodeUint32(b, e.i) }
func (e *event) Decode(b []byte) (i int) { e.i, i = elog.DecodeUint32(b, i); return }

//go:generate gentemplate -d Package=main -id event -d Type=event github.com/platinasystems/elib/elog/event.tmpl

func main() {
	elog.Enable(true)
	for i := uint32(0); i < 10; i++ {
		e := event{i: i}
		e.Log()
	}
	var b bytes.Buffer

	elog.Print(os.Stdout)

	if false {
		tb := elog.DefaultLog.TimeBounds()
		fmt.Printf("%+v\n", tb)
		os.Exit(0)
	}

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
