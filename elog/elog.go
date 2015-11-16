// High speed event logging
package elog

import (
	"github.com/platinasystems/elib"

	"bytes"
	"fmt"
	"io"
	"math"
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

	typeIndex uint16
	track     uint16

	Data [EventDataBytes]byte
}

type Track struct {
	Name string
}

type EventType struct {
	Name     string
	Stringer func(e *Event) string
	Decode   func(b []byte, e *Event) int
	Encode   func(b []byte, e *Event) int

	index       uint32
	lock        sync.Mutex // protects following
	Tags        []string
	IndexForTag map[string]int
}

type Log struct {
	// Circular buffer of events.
	events [1 << log2NEvents]Event

	// Index into circular buffer.
	index uint64

	// Disable logging when index reaches limit.
	disableIndex uint64

	// Timestamp when log was created.
	cpuStartTime Time

	StartTime time.Time

	// Timer tick in nanosecond units.
	timeUnitNsec float64

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
	i := atomic.AddUint64(&l.index, 1)
	e := &l.events[int(i-1)&(1<<log2NEvents-1)]
	e.timestamp = Now()
	e.typeIndex = uint16(t.index)
	return e
}

var (
	eventTypesLock sync.Mutex
	eventTypes     []*EventType
	typeByName     = make(map[string]*EventType)
)

func addTypeNoLock(t *EventType) {
	t.index = uint32(len(eventTypes))
	eventTypes = append(eventTypes, t)
}

func addType(t *EventType) {
	eventTypesLock.Lock()
	defer eventTypesLock.Unlock()
	addTypeNoLock(t)
}

func getTypeByIndex(i int) *EventType {
	eventTypesLock.Lock()
	defer eventTypesLock.Unlock()
	return eventTypes[i]
}

func (e *Event) getType() *EventType { return getTypeByIndex(int(e.typeIndex)) }

func RegisterType(t *EventType) {
	eventTypesLock.Lock()
	defer eventTypesLock.Unlock()
	if _, ok := typeByName[t.Name]; ok {
		panic("duplicate event type name: " + t.Name)
	}
	typeByName[t.Name] = t
	addTypeNoLock(t)
}

func getTypeByName(n string) (t *EventType, ok bool) {
	eventTypesLock.Lock()
	defer eventTypesLock.Unlock()
	t, ok = typeByName[n]
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
	l.cpuStartTime = Now()
	l.StartTime = time.Now()
	return l
}

func Now() Time { return Time(elib.Timestamp()) }

func (l *Log) timeUnitNsecs() (u float64) {
	u = l.timeUnitNsec
	if u == 0 {
		elib.CPUTimeInit()
		l.timeUnitNsec = 1e9 / elib.CPUCyclesPerSec()
		u = l.timeUnitNsec
	}
	return
}

// Time event happened in seconds relative to start of log.
func (e *Event) ElapsedTime(l *Log) float64 {
	return 1e-9 * float64(e.timestamp-l.cpuStartTime) * l.timeUnitNsecs()
}

func (e *Event) Time(l *Log) time.Time {
	nsec := float64(e.timestamp-l.cpuStartTime) * l.timeUnitNsecs()
	return l.StartTime.Add(time.Duration(nsec))
}

type LogTimeBounds struct {
	// Starting time truncated to nearest second.
	Start                 time.Time
	Min, Max, Unit, Round float64
	UnitName              string
}

func (l *Log) TimeBounds() (tb *LogTimeBounds) {
	e0, e1 := l.GetEvent(0), l.GetEvent(l.Len()-1)
	t0, t1 := e0.ElapsedTime(l), e1.ElapsedTime(l)

	tUnit := float64(1)
	mult := float64(1)
	unitName := "sec"
	if t1 > t0 {
		v := math.Floor(math.Log10(t1 - t0))
		iv := float64(0)
		switch {
		case v < -6:
			iv = -9.
			tUnit = 1e-9
			unitName = "nsec"
		case v < -3:
			iv = -6.
			tUnit = 1e-6
			unitName = "μsec"
		case v < 0:
			iv = -3.
			tUnit = 1e-3
			unitName = "msec"
		}
		mult = math.Pow10(int(math.Floor(v - iv)))
	}

	// Round absolute Go start time to seconds and add difference (nanoseconds part) to times.
	startSecs := l.StartTime.Truncate(time.Second)
	dt := 1e-9 * float64(l.StartTime.Sub(startSecs))
	t0 += dt
	t1 += dt

	t0 = math.Floor(t0 / tUnit)
	t1 = math.Ceil(t1 / tUnit)

	t0 = tUnit * mult * math.Floor(t0/mult)
	t1 = tUnit * mult * math.Ceil(t1/mult)

	return &LogTimeBounds{
		Min:      t0,
		Max:      t1,
		Round:    mult,
		Unit:     tUnit,
		Start:    startSecs,
		UnitName: unitName,
	}
}

func (e *Event) EventString(l *Log) (s string) {
	t := e.getType()
	s = fmt.Sprintf("%s: %s",
		e.Time(l).Format("2006-01-02 15:04:05.000000000"),
		t.Stringer(e))
	return
}

func StringLen(b []byte) (l int) {
	l = bytes.IndexByte(b, 0)
	if l < 0 {
		l = len(b)
	}
	return
}

func String(b []byte) string {
	return string(b[:StringLen(b)])
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

func (l *Log) GetEvent(index int) *Event {
	f := l.firstIndex()
	return &l.events[(f+index)&(1<<log2NEvents-1)]
}

// fixme locking
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
type genEvent struct {
	s [EventDataBytes]byte
}

func (e *genEvent) String() string      { return String(e.s[:]) }
func (e *genEvent) Encode(b []byte) int { return copy(b, e.s[:]) }
func (e *genEvent) Decode(b []byte) int { return copy(e.s[:], b) }

func GenEvent(format string, args ...interface{}) {
	e := genEvent{}
	Printf(e.s[:], format, args...)
	e.Log()
}

//go:generate gentemplate -d Package=elog -id genEvent -d Type=genEvent github.com/platinasystems/elib/elog/event.tmpl
