// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=elog -id genEvent -d Type=genEvent github.com/platinasystems/elib/elog/event.tmpl]

package elog

import ()

var genEventType = &EventType{
	Name: "elog.genEvent",
}

func init() {
	t := genEventType
	t.Stringer = stringer_genEvent
	t.Encode = encode_genEvent
	t.Decode = decode_genEvent
	RegisterType(genEventType)
}

func stringer_genEvent(e *Event) string {
	var x genEvent
	x.Decode(e.Data[:])
	return x.String()
}

func encode_genEvent(b []byte, e *Event) int {
	var x genEvent
	x.Decode(e.Data[:])
	return x.Encode(b)
}

func decode_genEvent(b []byte, e *Event) int {
	var x genEvent
	x.Decode(b)
	return x.Encode(e.Data[:])
}

func (x genEvent) Log() { x.Logb(DefaultBuffer) }

func (x genEvent) Logb(b *Buffer) {
	e := b.Add(genEventType)
	x.Encode(e.Data[:])
}
