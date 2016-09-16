// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=srpc -id event -d Type=event github.com/platinasystems/elib/elog/event.tmpl]

package srpc

import (
	"github.com/platinasystems/elib/elog"
)

var eventType = &elog.EventType{
	Name: "srpc.event",
}

func init() {
	t := eventType
	t.Stringer = stringer_event
	t.Encode = encode_event
	t.Decode = decode_event
	elog.RegisterType(eventType)
}

func stringer_event(e *elog.Event) string {
	var x event
	x.Decode(e.Data[:])
	return x.String()
}

func encode_event(b []byte, e *elog.Event) int {
	var x event
	x.Decode(e.Data[:])
	return x.Encode(b)
}

func decode_event(b []byte, e *elog.Event) int {
	var x event
	x.Decode(b)
	return x.Encode(e.Data[:])
}

func (x event) Log() { x.Logb(elog.DefaultBuffer) }

func (x event) Logb(b *elog.Buffer) {
	e := b.Add(eventType)
	x.Encode(e.Data[:])
}
