// autogenerated: do not edit!
// generated from gentemplate [gentemplate -d Package=main -id event -d Type=event github.com/platinasystems/elib/elog/event.tmpl]

package main

import (
	. "github.com/platinasystems/elib/elog"
)

var eventType = &EventType{
	Name: "main.event",
}

func init() {
	t := eventType
	t.Stringer = stringer_event
	t.Encode = encode_event
	t.Decode = decode_event
	RegisterType(eventType)
}

func stringer_event(e *Event) string {
	var x event
	x.Decode(e.Data[:])
	return x.String()
}

func encode_event(b []byte, e *Event) int {
	var x event
	x.Decode(e.Data[:])
	return x.Encode(b)
}

func decode_event(b []byte, e *Event) int {
	var x event
	x.Decode(b)
	return x.Encode(e.Data[:])
}

func (x event) Log() {
	e := Add(eventType)
	x.Encode(e.Data[:])
}