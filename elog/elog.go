// High speed event logging
package elog

import (
	"github.com/platinasystems/elib"

	"bytes"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const (
	log2NEvents    = 10
	log2EventBytes = 6
	EventDataBytes = 1<<log2EventBytes - (8 + 2*2)
)

// Event time stamp (CPU clock cycles when gathered)
type Time uint64

type Event struct {
	timestamp Time

	Type, Track uint16

	Data [EventDataBytes]byte
}

type Track struct {
	Name string
}

type EventType struct {
	Name     string
	Stringer func(e *Event) string
	Decoder  func(b []byte, e *Event) int
	Encoder  func(b []byte, e *Event) int

	registerOnce sync.Once
	index        uint32
	lock         sync.Mutex // protects following
	Tags         []string
	IndexForTag  map[string]int
}

type Log struct {
	// Circular buffer of events.
	events [1 << log2NEvents]Event

	// Index into circular buffer.
	index uint64

	// Disable logging when index reaches limit.
	disableIndex uint64

	// Timestamp when log was created.
	zeroTime Time

	unixZeroTime int64

	// Timer tick in seconds.
	timeUnitSecs float64

	// Dummy event to use when logging is disabled.
	disabledEvent Event
}

func (l *Log) Enable(v bool) {
	l.index = 0
	l.disableIndex = 0
	if v {
		l.disableIndex = ^l.disableIndex
	}
}

// Disable logging after specified number of events have been logged.
// This is used as a "debug trigger" when a certain target event has occurred.
// Events will be logged both before and after the target event.
func (l *Log) DisableAfter(n uint64) {
	if n > 1<<(log2NEvents-1) {
		n = 1 << (log2NEvents - 1)
	}
	l.disableIndex = l.index + n
}

func (l *Log) Enabled() bool {
	return l.index < l.disableIndex
}

func (l *Log) Add(t *EventType) *Event {
	if !l.Enabled() {
		return &l.disabledEvent
	}
	t.registerOnce.Do(func() { addType(t) })
	i := atomic.AddUint64(&l.index, 1)
	e := &l.events[int(i-1)&(1<<log2NEvents-1)]
	e.timestamp = Now()
	e.Type = uint16(t.index)
	return e
}

var (
	eventTypesLock sync.Mutex
	eventTypes     []*EventType
)

func addType(t *EventType) {
	eventTypesLock.Lock()
	defer eventTypesLock.Unlock()
	t.index = uint32(len(eventTypes))
	eventTypes = append(eventTypes, t)
}

func getTypeByIndex(i int) *EventType {
	eventTypesLock.Lock()
	defer eventTypesLock.Unlock()
	return eventTypes[i]
}

func (e *Event) getType() *EventType { return getTypeByIndex(int(e.Type)) }

var (
	registeredTypeMap  = make(map[string]*EventType)
	registeredTypeLock sync.Mutex
)

func RegisterType(t *EventType) {
	registeredTypeLock.Lock()
	defer registeredTypeLock.Unlock()
	if _, ok := registeredTypeMap[t.Name]; ok {
		panic("duplicate event type name: " + t.Name)
	}
	registeredTypeMap[t.Name] = t
}

func typeByName(n string) (t *EventType, ok bool) {
	registeredTypeLock.Lock()
	defer registeredTypeLock.Unlock()
	t, ok = registeredTypeMap[n]
	return
}

var DefaultLog = New()

func Add(t *EventType) *Event { return DefaultLog.Add(t) }
func Print(w io.Writer)       { DefaultLog.Print(w) }
func Len() (n int)            { return DefaultLog.Len() }
func Enabled() bool           { return DefaultLog.Enabled() }
func Enable(v bool)           { DefaultLog.Enable(v) }

func New() *Log {
	l := &Log{}
	l.zeroTime = Now()
	l.unixZeroTime = time.Now().UnixNano()
	return l
}

func Now() Time { return Time(elib.Timestamp()) }

func (l *Log) timeUnit() (u float64) {
	u = l.timeUnitSecs
	if u == 0 {
		elib.CPUTimeInit()
		l.timeUnitSecs = elib.CPUSecsPerCycle()
		u = l.timeUnitSecs
	}
	return
}

// Time event happened in seconds relative to start of log.
func (e *Event) Time(l *Log) float64 { return float64(e.timestamp-l.zeroTime) * l.timeUnit() }

// Absolute time in nanosecs from Unix epoch.
func (e *Event) TimeUnixNano(l *Log) int64 { return l.unixZeroTime + int64(1e9*e.Time(l)) }

func (e *Event) EventString(l *Log) (s string) {
	t := e.getType()
	s = fmt.Sprintf("%s: %s",
		time.Unix(0, e.TimeUnixNano(l)).Format("2006-01-02 15:04:05.000000000"),
		t.Stringer(e))
	return
}

func String(b []byte) string {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return string(b[:])
	} else {
		return string(b[:i])
	}
}

func PutData(b []byte, data []byte) {
	b = PutUvarint(b, len(data))
	copy(b, data)
}

func HexData(p []byte) string {
	b, l := Uvarint(p)
	m := l
	dots := ""
	if m > len(b) {
		m = len(b)
		dots = "..."
	}
	return fmt.Sprintf("%d %x%s", l, b[:m], dots)
}

func Printf(b []byte, format string, a ...interface{}) {
	copy(b, fmt.Sprintf(format, a...))
}

func (l *Log) Len() (n int) {
	n = int(l.index)
	max := 1 << log2NEvents
	if n > max {
		n = max
	}
	return
}

func (l *Log) firstIndex() (f int) {
	f = int(l.index - 1<<log2NEvents)
	if f < 0 {
		f = 0
	}
	f &= 1<<log2NEvents - 1
	return
}

func (l *Log) ForeachEvent(f func(e *Event)) {
	i := l.firstIndex()
	for n := l.Len(); n > 0; n-- {
		e := &l.events[int(i)&(1<<log2NEvents-1)]
		f(e)
		i++
	}
}

func (l *Log) Print(w io.Writer) {
	l.ForeachEvent(func(e *Event) {
		fmt.Fprintln(w, e.EventString(l))
	})
}

func (t *EventType) Tag(i int, sep string) (tag string) {
	tag = ""
	if i < len(t.Tags) {
		tag = t.Tags[i] + sep
	}
	return
}

func (t *EventType) TagIndex(s string) (i int) {
	t.lock.Lock()
	defer t.lock.Unlock()
	l := len(t.Tags)
	if t.IndexForTag == nil {
		t.IndexForTag = make(map[string]int)
	}
	i, ok := t.IndexForTag[s]
	if !ok {
		i = l
		t.IndexForTag[s] = i
		t.Tags = append(t.Tags, s)
	}
	return
}

// Generic events
type GenEvent struct {
	s [EventDataBytes]byte
}

func (e *GenEvent) String() string {
	return String(e.s[:])
}

func Gen(format string, args ...interface{}) {
	e := GenEvent{}
	Printf(e.s[:], format, args...)
	e.Log()
}

//go:generate gentemplate -d Package=elog -id GenEvent -d Type=GenEvent github.com/platinasystems/elib/elog/event.tmpl
