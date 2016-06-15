package loop

import (
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/elib/dep"
	"github.com/platinasystems/elib/elog"
	"github.com/platinasystems/elib/event"

	"fmt"
	"os"
	"sync"
	"time"
)

type Node struct {
	name                 string
	index                uint
	loop                 *Loop
	rxEvents             chan event.Actor
	toLoop               chan struct{}
	fromLoop             chan struct{}
	eventVec             event.ActorVec
	active               bool
	activeCount          uint
	dataCaller           inOutLooper
	activePollerIndex    uint
	initOnce             sync.Once
	initWg               sync.WaitGroup
	outIns               []LooperIn
	nextIndexByNodeIndex map[uint]uint
	nodeIndexByNext      []uint
	Next                 []string
}

func (n *Node) GetNode() *Node { return n }
func (n *Node) Index() uint    { return n.index }
func (n *Node) Name() string   { return n.name }
func (n *Node) ThreadId() uint { return uint(n.activePollerIndex) }
func (n *Node) GetLoop() *Loop { return n.loop }
func nodeName(n Noder) string  { return n.GetNode().name }

func (l *Loop) countActive(enable bool) {
	if enable {
		l.nActivePollers++
	} else {
		if l.nActivePollers == 0 {
			panic("decrement zero active pollers")
		}
		l.nActivePollers--
	}
}

func (n *Node) activate(enable bool, count uint) {
	if n.active != enable {
		n.active = enable
		n.activeCount = count
		n.loop.countActive(enable)
	}
}

func (n *Node) Activate(enable bool)     { n.activate(enable, 0) }
func (n *Node) ActivateCount(count uint) { n.activate(true, count) }
func (n *Node) ActivateOnce(enable bool) { n.activate(enable, 1) }

type Noder interface {
	GetNode() *Node
}

type EventPoller interface {
	EventPoll()
}

type EventHandler interface {
	Noder
	EventHandler()
}

type Initer interface {
	Noder
	LoopInit(l *Loop)
}

type Exiter interface {
	LoopExit(l *Loop)
}

type Loop struct {
	eventPollers           []EventPoller
	eventHandlers          []EventHandler
	dataPollers            []inLooper
	DataNodes              []Noder
	dataNodeByName         map[string]Noder
	loopIniters            []Initer
	loopExiters            []Exiter
	activePollers          []*activePoller
	nActivePollers         uint32
	events                 chan loopEvent
	eventPool              event.Pool
	registrationsNeedStart bool
	startTime              cpu.Time
	now                    cpu.Time
	cyclesPerSec           float64
	secsPerCycle           float64
	timeDurationPerCycle   float64
	wg                     sync.WaitGroup
	cli                    LoopCli
}

func (l *Loop) Seconds(t cpu.Time) float64 { return float64(t) * l.secsPerCycle }

type loopEvent struct {
	actor event.Actor
	dst   *Node
	time  cpu.Time
}

func (e *loopEvent) EventTime() cpu.Time { return e.time }

func (l *Loop) AddEvent(e event.Actor, dst EventHandler) {
	le := loopEvent{actor: e}
	if dst != nil {
		le.dst = dst.GetNode()
	}
	l.events <- le
}

func (l *Loop) AddTimedEvent(e event.Actor, dst EventHandler, dt float64) {
	l.eventPool.Add(&loopEvent{
		actor: e,
		dst:   dst.GetNode(),
		time:  cpu.TimeNow() + cpu.Time(dt*l.cyclesPerSec),
	})
}

func (e *loopEvent) EventAction() {
	if e.dst != nil {
		e.dst.rxEvents <- e.actor
		e.dst.active = true
	}
}

func (e *loopEvent) String() string { return "loop event" }

type loopLogEvent struct {
	s [elog.EventDataBytes]byte
}

func (e *loopLogEvent) String() string      { return fmt.Sprintf("loop event: %s", elog.String(e.s[:])) }
func (e *loopLogEvent) Encode(b []byte) int { return copy(b, e.s[:]) }
func (e *loopLogEvent) Decode(b []byte) int { return copy(e.s[:], b) }

//go:generate gentemplate -d Package=loop -id loopLogEvent -d Type=loopLogEvent github.com/platinasystems/elib/elog/event.tmpl

func (l *Loop) doEvent(e event.Actor) {
	defer func() {
		if err := recover(); err == ErrQuit {
			l.Quit()
		} else if err != nil {
			fmt.Printf("%s\n", err)
			l.Quit()
		}
	}()
	if elog.Enabled() {
		le := loopLogEvent{}
		copy(le.s[:], e.String())
		le.Log()
	}
	e.EventAction()
}

func (l *Loop) eventHandler(p EventHandler) {
	c := p.GetNode()
	for {
		e := <-c.rxEvents
		l.doEvent(e)
		c.toLoop <- struct{}{}
	}
}

func (l *Loop) startEventHandler(n EventHandler) {
	c := n.GetNode()
	c.toLoop = make(chan struct{}, 1)
	c.fromLoop = make(chan struct{}, 1)
	c.rxEvents = make(chan event.Actor, 256)
	go l.eventHandler(n)
}

func (l *Loop) eventPoller(p EventPoller) {
	for {
		p.EventPoll()
	}
}
func (l *Loop) startEventPoller(n EventPoller) { go l.eventPoller(n) }

func (l *Loop) doEventNoWait() (done bool) {
	l.now = cpu.TimeNow()
	select {
	case e := <-l.events:
		done = e.isQuit()
		e.EventAction()
	default:
	}
	return
}

func (l *Loop) doEventWait() (done bool) {
	l.now = cpu.TimeNow()
	dt := time.Duration(1<<63 - 1)
	if t, ok := l.eventPool.NextTime(); ok {
		dt = time.Duration(float64(t-l.now) * l.timeDurationPerCycle)
	}
	select {
	case e := <-l.events:
		done = e.isQuit()
		e.EventAction()
	case <-time.After(dt):
	}
	return
}

func (l *Loop) doEvents() (done bool) {
	// Handle discrete events.
	if l.nActivePollers > 0 {
		done = l.doEventNoWait()
	} else {
		done = l.doEventWait()
	}

	// Handle expired timed events.
	l.eventPool.Advance(l.now)

	// Wait for all event handlers to become inactive.
	for _, h := range l.eventHandlers {
		c := h.GetNode()
		if c.active {
			<-c.toLoop
			c.active = false
		}
	}

	return
}

func (l *Loop) eventInit() {
	l.events = make(chan loopEvent, 256)

	for _, n := range l.eventPollers {
		l.startEventPoller(n)
	}
	for _, n := range l.eventHandlers {
		l.startEventHandler(n)
	}
}

func (l *Loop) startPollers() {
	for _, n := range l.dataPollers {
		l.startDataPoller(n)
	}
}

func (l *Loop) dataPoll(p inLooper) {
	c := p.GetNode()
	for {
		<-c.fromLoop
		ap := l.activePollers[c.activePollerIndex]
		if ap.activeNodes == nil {
			ap.init(l, c.activePollerIndex)
		}
		n := &ap.activeNodes[c.index]
		ap.currentNode = n
		t0 := cpu.TimeNow()
		ap.timeNow = t0
		p.LoopInput(l, n.looperOut)
		nVec := n.out.call(l, ap)
		ap.pollerStats.update(nVec, t0)
		c.toLoop <- struct{}{}
	}
}

func (l *Loop) startDataPoller(n inLooper) {
	c := n.GetNode()
	c.toLoop = make(chan struct{}, 1)
	c.fromLoop = make(chan struct{}, 1)
	go l.dataPoll(n)
}

func (l *Loop) doPollers() {
	if l.activePollers == nil {
		l.activePollers = make([]*activePoller, len(l.dataPollers))
		for i := range l.activePollers {
			l.activePollers[i] = &activePoller{}
		}
	}
	nActive := uint(0)
	for _, p := range l.dataPollers {
		c := p.GetNode()
		if c.active {
			l.activePollers[nActive].pollerNode = c
			c.activePollerIndex = nActive
			nActive++
			if c.activeCount > 0 {
				c.activeCount--
				if c.activeCount == 0 {
					c.active = false
					l.countActive(false)
				}
			}
			c.fromLoop <- struct{}{}
		}
	}

	// Wait for pollers to finish.
	for i := uint(0); i < nActive; i++ {
		<-l.activePollers[i].pollerNode.toLoop
	}

	// Wait for workers to finish.
	l.wg.Wait()
}

func (l *Loop) timerInit() {
	t := cpu.Time(0)
	t.Cycles(1 * cpu.Second)
	l.cyclesPerSec = float64(t)
	l.secsPerCycle = 1 / l.cyclesPerSec
	l.timeDurationPerCycle = l.secsPerCycle / float64(time.Second)
}

type initHook func(l *Loop)

//go:generate gentemplate -id initHook -d Package=loop -d DepsType=initHookVec -d Type=initHook -d Data=hooks github.com/platinasystems/elib/dep/dep.tmpl

var initHooks, exitHooks initHookVec

func AddInit(f initHook, d ...*dep.Dep) { initHooks.Add(f, d...) }
func AddExit(f initHook, d ...*dep.Dep) { exitHooks.Add(f, d...) }

func (l *Loop) callInitHooks() {
	for i := range initHooks.hooks {
		initHooks.Get(i)(l)
	}
}

func (l *Loop) callExitHooks() {
	for i := range exitHooks.hooks {
		exitHooks.Get(i)(l)
	}
}

func (l *Loop) callInitNode(n Initer, isCall bool) {
	c := n.GetNode()
	wg := &l.wg
	if isCall {
		wg = &c.initWg
	}
	c.initOnce.Do(func() {
		wg.Add(1)
		go func() {
			n.LoopInit(l)
			wg.Done()
		}()
	})
}
func (l *Loop) CallInitNode(n Initer)  { l.callInitNode(n, true) }
func (l *Loop) startInitNode(n Initer) { l.callInitNode(n, false) }

func (l *Loop) doInitNodes() {
	for _, i := range l.loopIniters {
		l.startInitNode(i)
	}
	l.wg.Wait()
}

func (l *Loop) nodeGraphInit() {
	for _, n := range l.DataNodes {
		x := n.GetNode()
		for _, name := range x.Next {
			if _, ok := l.AddNamedNext(n, name); !ok {
				panic(fmt.Errorf("unknown next named %s", name))
			}
		}
	}
}

func (l *Loop) doExit() {
	l.callExitHooks()
	for i := range l.loopExiters {
		l.loopExiters[i].LoopExit(l)
	}
}

func (l *Loop) Run() {
	elog.Enable(true)
	go elog.PrintOnHangupSignal(os.Stderr)

	l.timerInit()
	l.startTime = cpu.TimeNow()
	l.callInitHooks()
	l.eventInit()
	l.startPollers()
	l.registrationsNeedStart = true
	l.doInitNodes()
	l.nodeGraphInit()
	for !l.doEvents() {
		l.doPollers()
	}
	l.doExit()
}

func (l *Loop) addDataNode(r Noder) {
	n := r.GetNode()
	n.index = uint(len(l.DataNodes))
	l.DataNodes = append(l.DataNodes, r)
	if l.dataNodeByName == nil {
		l.dataNodeByName = make(map[string]Noder)
	}
	l.dataNodeByName[n.name] = r
}

func (l *Loop) RegisterNode(n Noder, format string, args ...interface{}) {
	x := n.GetNode()
	x.name = fmt.Sprintf(format, args...)
	x.loop = l
	start := l.registrationsNeedStart

	nOK := 0
	if h, ok := n.(EventHandler); ok {
		l.eventHandlers = append(l.eventHandlers, h)
		if start {
			l.startEventHandler(h)
		}
		nOK++
	}
	if d, isOut := n.(outNoder); isOut {
		nok := 0
		if _, ok := d.(inOutLooper); ok {
			nok++
		}
		if q, ok := d.(inLooper); ok {
			l.dataPollers = append(l.dataPollers, q)
			if start {
				l.startDataPoller(q)
			}
			nok++
		}
		if nok > 0 {
			l.addDataNode(n)
		} else {
			panic(fmt.Errorf("%s: missing LoopInput/LoopInputOutput method", x.name))
		}
		nOK += nok
	} else if _, isIn := n.(inNoder); isIn {
		if _, ok := n.(outLooper); ok {
			l.addDataNode(n)
			nOK += 1
		} else {
			panic(fmt.Errorf("%s: missing LoopOutput method", x.name))
		}
	}
	if p, ok := n.(Initer); ok {
		l.loopIniters = append(l.loopIniters, p)
		if start {
			l.startInitNode(p)
		}
		nOK++
	}
	if p, ok := n.(Exiter); ok {
		l.loopExiters = append(l.loopExiters, p)
		nOK++
	}
	if nOK == 0 {
		panic(fmt.Errorf("unkown node type: %T", n))
	}
}

func (l *Loop) RegisterEventPoller(p EventPoller) {
	l.eventPollers = append(l.eventPollers, p)
}

type quitEvent struct{}

var ErrQuit = &quitEvent{}

func (e *quitEvent) Error() string      { return "quit" }
func (e *loopEvent) isQuit() (yes bool) { _, yes = e.actor.(*quitEvent); return }
func (q *quitEvent) EventAction()       {}
func (q *quitEvent) String() string     { return "quit" }
func (l *Loop) Quit()                   { l.AddEvent(&quitEvent{}, nil) }
