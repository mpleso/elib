package loop

import (
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/elib/dep"
	"github.com/platinasystems/elib/elog"
	"github.com/platinasystems/elib/event"

	"fmt"
	"os"
	"sync"
	"sync/atomic"
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
	polling              bool
	suspended            bool
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
func (n *Node) GetLoop() *Loop { return n.loop }
func (n *Node) ThreadId() uint { return n.activePollerIndex }
func nodeName(n Noder) string  { return n.GetNode().name }

func (n *Node) getActivePoller(l *Loop) *activePoller {
	return l.activePollerPool.entries[n.activePollerIndex]
}

func (n *Node) allocActivePoller(l *Loop) {
	i := l.activePollerPool.GetIndex()
	a := l.activePollerPool.entries[i]
	if a == nil {
		a = &activePoller{}
		l.activePollerPool.entries[i] = a
	}
	a.index = uint16(i)
	n.activePollerIndex = i
	a.pollerNode = n
	elog.GenEventf("alloc poller %d %s", i, n.name)
}

func (n *Node) freeActivePoller(l *Loop) {
	a := n.getActivePoller(l)
	a.pollerNode = nil
	i := n.activePollerIndex
	l.activePollerPool.PutIndex(i)
	n.activePollerIndex = ^uint(0)
	a.index = ^uint16(0)
	elog.GenEventf("free poller %d %s", i, n.name)
}

func (n *Node) Activate(enable bool) {
	if n.active != enable {
		n.active = enable
		// Interrupt wait to poll active nodes.
		if enable && n.loop.eventWaiting {
			n.loop.Interrupt()
		}
	}
}

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
	DataNodes      []Noder
	dataNodeByName map[string]Noder

	eventPollers  []EventPoller
	eventHandlers []EventHandler

	loopIniters []Initer
	loopExiters []Exiter

	dataPollers      []inLooper
	activePollerPool activePollerPool
	nActivePollers   uint
	pollerStats      pollerStats

	events       chan loopEvent
	eventPool    event.Pool
	eventWaiting bool
	wg           sync.WaitGroup

	registrationsNeedStart bool
	startTime              cpu.Time
	now                    cpu.Time
	cyclesPerSec           float64
	secsPerCycle           float64
	timeDurationPerCycle   float64

	cli LoopCli
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
		le := eventElogEvent{}
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

func (l *Loop) doEventNoWait() (quit *quitEvent) {
	l.now = cpu.TimeNow()
	select {
	case e := <-l.events:
		var ok bool
		if quit, ok = e.actor.(*quitEvent); ok {
			return
		}
		e.EventAction()
	default:
	}
	return
}

func (l *Loop) doEventWait() (quit *quitEvent) {
	l.now = cpu.TimeNow()
	dt := time.Duration(1<<63 - 1)
	if t, ok := l.eventPool.NextTime(); ok {
		dt = time.Duration(float64(t-l.now) * l.timeDurationPerCycle)
	}
	select {
	case e := <-l.events:
		var ok bool
		if quit, ok = e.actor.(*quitEvent); ok {
			return
		}
		e.EventAction()
	case <-time.After(dt):
	}
	return
}

func (l *Loop) doEvents() (quitLoop bool) {
	// Handle discrete events.
	var quit *quitEvent
	if l.nActivePollers > 0 {
		quit = l.doEventNoWait()
	} else {
		l.eventWaiting = true
		quit = l.doEventWait()
		l.eventWaiting = false
	}
	if quit != nil {
		quitLoop = quit.Type == quitEventExit
		return
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

func (l *Loop) Suspend(in *In) {
	a := l.activePollerPool.entries[in.activeIndex]
	p := a.pollerNode
	p.pollerElog(poller_suspend, byte(a.index))
	p.suspended = true
	p.toLoop <- struct{}{}
	<-p.fromLoop
	p.suspended = false
	p.pollerElog(poller_resume, byte(a.index))
}

func (l *Loop) dataPoll(p inLooper) {
	c := p.GetNode()
	for {
		<-c.fromLoop
		ap := c.getActivePoller(l)
		if ap.activeNodes == nil {
			ap.initNodes(l)
		}
		n := &ap.activeNodes[c.index]
		ap.currentNode = n
		t0 := cpu.TimeNow()
		ap.timeNow = t0
		p.LoopInput(l, n.looperOut)
		nVec := n.out.call(l, ap)
		ap.pollerStats.update(nVec, t0)
		l.pollerStats.update(nVec)
		c.pollerElog(poller_sig, byte(ap.index))
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
	for _, p := range l.dataPollers {
		n := p.GetNode()
		if !(n.active || n.suspended) {
			continue
		}
		if n.activePollerIndex == ^uint(0) {
			n.allocActivePoller(n.loop)
		}
		n.polling = true
		n.pollerElog(poller_start, byte(n.activePollerIndex))
		n.fromLoop <- struct{}{}
	}

	// Wait for pollers to finish.
	nActive := uint(0)
	for i := uint(0); i < l.activePollerPool.Len(); i++ {
		if l.activePollerPool.IsFree(i) {
			continue
		}
		n := l.activePollerPool.entries[i].pollerNode
		if !n.polling {
			continue
		}

		<-n.toLoop
		n.polling = false
		n.pollerElog(poller_done, byte(n.activePollerIndex))

		// If not active anymore we can free it now.
		if !(n.active || n.suspended) {
			if !l.activePollerPool.IsFree(n.activePollerIndex) {
				n.freeActivePoller(l)
			}
		} else {
			nActive++
		}
	}

	if nActive == 0 && l.nActivePollers > 0 {
		l.resetPollerStats()
	} else {
		l.doPollerStats()
	}
	l.nActivePollers = nActive
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
				panic(fmt.Errorf("%s: unknown next named %s", nodeName(n), name))
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
	l.cliInit()
	l.callInitHooks()
	l.eventInit()
	l.startPollers()
	l.registrationsNeedStart = true
	l.doInitNodes()
	l.nodeGraphInit()
	for {
		if quit := l.doEvents(); quit {
			break
		}
		l.doPollers()
	}
	l.doExit()
}

type pollerCounts struct {
	nActiveNodes   uint32
	nActiveVectors uint32
}

type pollerStats struct {
	loopCount          uint64
	updateCount        uint64
	current            pollerCounts
	history            [1 << log2PollerHistorySize]pollerCounts
	interruptsDisabled bool
}

const (
	log2LoopsPerStatsUpdate = 7
	loopsPerStatsUpdate     = 1 << log2LoopsPerStatsUpdate
	log2PollerHistorySize   = 1
	// When vector rate crosses threshold disable interrupts and switch to polling mode.
	interruptDisableThreshold float64 = 10
)

type InterruptEnabler interface {
	InterruptEnable(enable bool)
}

func (l *Loop) resetPollerStats() {
	s := &l.pollerStats
	s.loopCount = 0
	for i := range s.history {
		s.history[i].reset()
	}
	s.current.reset()
	if s.interruptsDisabled {
		l.disableInterrupts(false)
	}
}

func (l *Loop) disableInterrupts(disable bool) {
	enable := !disable
	for _, n := range l.dataPollers {
		if x, ok := n.(InterruptEnabler); ok {
			x.InterruptEnable(enable)
			n.GetNode().Activate(disable)
		}
	}
	l.pollerStats.interruptsDisabled = disable
	if elog.Enabled() {
		elog.GenEventf("loop: irq disable %v", disable)
	}
}

func (l *Loop) doPollerStats() {
	s := &l.pollerStats
	s.loopCount++
	if s.loopCount&(1<<log2LoopsPerStatsUpdate-1) == 0 {
		s.history[s.updateCount&(1<<log2PollerHistorySize-1)] = s.current
		s.updateCount++
		disable := s.current.vectorRate() > interruptDisableThreshold
		if disable != s.interruptsDisabled {
			l.disableInterrupts(disable)
		}
		s.current.reset()
	}
}

func (s *pollerStats) update(nVec uint) {
	v := uint32(0)
	if nVec > 0 {
		v = 1
	}
	c := &s.current
	atomic.AddUint32(&c.nActiveVectors, uint32(nVec))
	atomic.AddUint32(&c.nActiveNodes, v)
}

func (c *pollerCounts) vectorRate() float64 {
	return float64(c.nActiveVectors) / float64(1<<log2LoopsPerStatsUpdate)
}

func (c *pollerCounts) reset() {
	c.nActiveVectors = 0
	c.nActiveNodes = 0
}

func (s *pollerStats) VectorRate() float64 {
	return s.history[(s.updateCount-1)&(1<<log2PollerHistorySize-1)].vectorRate()
}

func (l *Loop) addDataNode(r Noder) {
	n := r.GetNode()
	n.index = uint(len(l.DataNodes))
	n.activePollerIndex = ^uint(0)
	l.DataNodes = append(l.DataNodes, r)
	if l.dataNodeByName == nil {
		l.dataNodeByName = make(map[string]Noder)
	}
	if _, ok := l.dataNodeByName[n.name]; ok {
		panic(fmt.Errorf("%s: more than one node with this name", n.name))
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

type quitEvent struct{ Type quitEventType }
type quitEventType uint8

const (
	quitEventExit quitEventType = iota
	quitEventInterrupt
)

var quitEventTypeStrings = [...]string{
	quitEventExit:      "quit",
	quitEventInterrupt: "interrupt",
}

var (
	ErrQuit      = &quitEvent{Type: quitEventExit}
	ErrInterrupt = &quitEvent{Type: quitEventInterrupt}
)

func (e *quitEvent) String() string { return quitEventTypeStrings[e.Type] }
func (e *quitEvent) Error() string  { return e.String() }
func (e *quitEvent) EventAction()   {}
func (l *Loop) Quit()               { l.AddEvent(ErrQuit, nil) }
func (l *Loop) Interrupt()          { l.AddEvent(ErrInterrupt, nil) }
