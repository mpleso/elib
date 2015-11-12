// High speed event logging
package elog

import (
	"github.com/platinasystems/elib"

	"bytes"
	"encoding/binary"
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

type Time uint64

type Event struct {
	Time

	Type, Track uint16

	Data [EventDataBytes]byte
}

type Track struct {
	Name string
}

type EventType struct {
	Stringer     func(e *Event) string
	registerOnce sync.Once
	index        int
	lock         sync.Mutex // protects following
	tags         []string
	indexForTag  map[string]int
}

type Log struct {
	types  []*EventType
	Events [1 << log2NEvents]Event
	// Dummy event to use when logging is disabled.
	disabledEvent Event
	index         uint64
	// Disable logging when index reaches limit.
	disableIndex uint64
	cpuTime
	// Timestamp when log was created.
	ZeroTime Time
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
	t.registerOnce.Do(func() { l.RegisterType(t) })
	i := atomic.AddUint64(&l.index, 1)
	e := &l.Events[int(i-1)&(1<<log2NEvents-1)]
	e.Time = Now()
	e.Type = uint16(t.index)
	return e
}

func (l *Log) RegisterType(t *EventType) {
	t.index = len(l.types)
	l.types = append(l.types, t)
}

var DefaultLog = New()

func Add(t *EventType) *Event   { return DefaultLog.Add(t) }
func RegisterType(t *EventType) { DefaultLog.RegisterType(t) }
func Print(w io.Writer)         { DefaultLog.Print(w) }
func Len() (n int)              { return DefaultLog.Len() }
func Enabled() bool             { return DefaultLog.Enabled() }
func Enable(v bool)             { DefaultLog.Enable(v) }

func New() *Log {
	l := &Log{}
	go estimateFrequency(10e-3, 1e6, 1e4, &l.cpuTime)
	l.ZeroTime = Now()
	return l
}

func Now() Time { return Time(elib.Timestamp()) }

func (e *Event) EventString(l *Log) (s string) {
	t := l.types[e.Type]
	s = fmt.Sprintf("%12.6f %s", float64(e.Time-l.ZeroTime)*l.secsPerTick(), t.Stringer(e))
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

func (l *Log) Print(w io.Writer) {
	i := l.firstIndex()
	for n := l.Len(); n > 0; n-- {
		e := &l.Events[int(i)&(1<<log2NEvents-1)]
		fmt.Fprintln(w, e.EventString(l))
		i++
	}
}

func measureCPUCyclesPerSec(wait float64) (freq float64) {
	var t0 [2]uint64
	var t1 [2]int64
	t1[0] = time.Now().UnixNano()
	t0[0] = elib.Timestamp()
	time.Sleep(time.Duration(1e9 * wait))
	t1[1] = time.Now().UnixNano()
	t0[1] = elib.Timestamp()
	freq = 1e9 * float64(t0[1]-t0[0]) / float64(t1[1]-t1[0])
	return
}

func round(x, unit float64) float64 {
	return unit * math.Floor(.5+x/unit)
}

type cpuTime struct {
	// Ticks per second of event timer (and inverse).
	TicksPerSec, SecsPerTick float64
}

func (l *Log) secsPerTick() float64 {
	// Wait until estimateFrequency is done.
	for l.cpuTime.TicksPerSec == 0 {
	}
	return l.cpuTime.SecsPerTick
}

func estimateFrequency(dt, unit, tolerance float64, result *cpuTime) {
	var sum, sum2, ave, rms, n float64
	for n = float64(1); true; n++ {
		f := measureCPUCyclesPerSec(dt)
		sum += f
		sum2 += f * f
		ave = sum / n
		rms = math.Sqrt((sum2/n - ave*ave) / n)
		if n >= 16 && rms < tolerance {
			break
		}
	}

	result.TicksPerSec = round(ave, unit)
	result.SecsPerTick = 1 / result.TicksPerSec
	return
}

func (t *EventType) Tag(i int, sep string) (tag string) {
	tag = ""
	if i < len(t.tags) {
		tag = t.tags[i] + sep
	}
	return
}

func (t *EventType) TagIndex(s string) (i int) {
	t.lock.Lock()
	defer t.lock.Unlock()
	l := len(t.tags)
	if t.indexForTag == nil {
		t.indexForTag = make(map[string]int)
	}
	i, ok := t.indexForTag[s]
	if !ok {
		i = l
		t.indexForTag[s] = i
		t.tags = append(t.tags, s)
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
