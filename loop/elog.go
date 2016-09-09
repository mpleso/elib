package loop

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/elog"

	"fmt"
)

type poller_elog_event_type byte

const (
	poller_start poller_elog_event_type = iota + 1
	poller_done
	poller_suspend
	poller_resume
)

var poller_elog_event_type_names = [...]string{
	poller_start:   "start",
	poller_done:    "done",
	poller_suspend: "suspend",
	poller_resume:  "resume",
}

func (t poller_elog_event_type) String() string {
	return elib.Stringer(poller_elog_event_type_names[:], int(t))
}

type pollerElogEvent struct {
	poller_index byte
	event_type   poller_elog_event_type
	name         [elog.EventDataBytes - 2]byte
}

func (n *Node) pollerElog(t poller_elog_event_type, poller_index byte) {
	if elog.Enabled() {
		le := pollerElogEvent{
			event_type:   t,
			poller_index: poller_index,
		}
		copy(le.name[:], n.name)
		le.Log()
	}
}

func (e *pollerElogEvent) String() string {
	return fmt.Sprintf("poller %s %d %s ", e.event_type, e.poller_index, elog.String(e.name[:]))
}
func (e *pollerElogEvent) Encode(b []byte) int {
	b = elog.PutUvarint(b, int(e.poller_index))
	b = elog.PutUvarint(b, int(e.event_type))
	return copy(b, e.name[:])
}
func (e *pollerElogEvent) Decode(b []byte) int {
	var i [2]int
	b, i[0] = elog.Uvarint(b)
	b, i[1] = elog.Uvarint(b)
	e.poller_index = byte(i[0])
	e.event_type = poller_elog_event_type(i[1])
	return copy(e.name[:], b)
}

//go:generate gentemplate -d Package=loop -id pollerElogEvent -d Type=pollerElogEvent github.com/platinasystems/elib/elog/event.tmpl

type eventElogEvent struct {
	s [elog.EventDataBytes]byte
}

func (e *eventElogEvent) String() string {
	return "loop event: " + elog.String(e.s[:])
}
func (e *eventElogEvent) Encode(b []byte) int { return copy(b, e.s[:]) }
func (e *eventElogEvent) Decode(b []byte) int { return copy(e.s[:], b) }

//go:generate gentemplate -d Package=loop -id eventElogEvent -d Type=eventElogEvent github.com/platinasystems/elib/elog/event.tmpl
