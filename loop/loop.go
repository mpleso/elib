package loop

import (
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/elib/dep"
	"github.com/platinasystems/elib/elog"

	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Node struct {
	name                    string
	noder                   Noder
	index                   uint
	loop                    *Loop
	toLoop                  chan struct{}
	fromLoop                chan struct{}
	active                  bool
	polling                 bool
	suspended               bool
	activePollerIndex       uint
	initOnce                sync.Once
	initWg                  sync.WaitGroup
	Next                    []string
	nextNodes               nextNodeVec
	nextIndexByNodeName     map[string]uint
	inputStats, outputStats nodeStats
	eventNode
}

type nextNode struct {
	name      string
	nodeIndex uint
	in        LooperIn
}

//go:generate gentemplate -d Package=loop -id nextNode -d VecType=nextNodeVec -d Type=nextNode github.com/platinasystems/elib/vec.tmpl

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
}

func (a *activePoller) flushNodeStats(l *Loop) {
	for i := range a.activeNodes {
		ani := &a.activeNodes[i]
		ni := l.DataNodes[ani.index].GetNode()

		ni.inputStats.current.add_raw(&ani.inputStats)
		ani.inputStats.zero()

		ni.outputStats.current.add_raw(&ani.outputStats)
		ani.outputStats.zero()
	}
}

func (n *Node) freeActivePoller(l *Loop) {
	a := n.getActivePoller(l)
	a.flushNodeStats(l)
	a.pollerNode = nil
	i := n.activePollerIndex
	l.activePollerPool.PutIndex(i)
	n.activePollerIndex = ^uint(0)
}

func (n *Node) Activate(enable bool) (was bool) {
	was = n.active
	if was != enable {
		n.active = enable
		// Interrupt event wait to poll active nodes.
		if enable {
			n.loop.Interrupt()
		}
	}
	return
}

type activateEvent struct{ n *Node }

func (e *activateEvent) EventAction()   { e.n.Activate(true) }
func (e *activateEvent) String() string { return fmt.Sprintf("activate %s", e.n.name) }

func (n *Node) ActivateAfterTime(dt float64) {
	if n.active {
		n.active = false
		n.activateEvent.n = n
		le := n.loop.getLoopEvent(&n.activateEvent)
		n.loop.addTimedEvent(le, dt)
	}
}

type Noder interface {
	GetNode() *Node
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

	loopIniters []Initer
	loopExiters []Exiter

	dataPollers      []inLooper
	activePollerPool activePollerPool
	nActivePollers   uint
	pollerStats      pollerStats

	wg sync.WaitGroup

	registrationsNeedStart bool
	initialNodesRegistered bool
	startTime              cpu.Time
	now                    cpu.Time
	cyclesPerSec           float64
	secsPerCycle           float64
	timeDurationPerCycle   float64
	timeLastRuntimeClear   time.Time

	Cli LoopCli
	eventLoop
}

func (l *Loop) Seconds(t cpu.Time) float64 { return float64(t) * l.secsPerCycle }

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
}

func (l *Loop) Resume(in *In) {
	a := l.activePollerPool.entries[in.activeIndex]
	if p := a.pollerNode; p != nil {
		p.active = true
		p.suspended = false
		p.pollerElog(poller_resume, byte(a.index))
		l.Interrupt()
	}
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
		if !n.active || n.suspended {
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
	l.timeDurationPerCycle = l.secsPerCycle * float64(time.Second)
}

func (l *Loop) TimeDiff(t0, t1 cpu.Time) float64 { return float64(t1-t0) * l.secsPerCycle }

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
	l.timeLastRuntimeClear = time.Now()
	l.cliInit()
	l.eventInit()
	l.startPollers()
	l.registrationsNeedStart = true
	l.callInitHooks()
	l.doInitNodes()
	// Now that all initial nodes have been registered, initialize node graph.
	l.graphInit()
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
	n.noder = r
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
	for i := range x.Next {
		if _, err := l.AddNamedNext(n, x.Next[i]); err != nil {
			panic(err)
		}
	}

	start := l.registrationsNeedStart
	nOK := 0
	if h, ok := n.(EventHandler); ok {
		l.eventLoop.handlers = append(l.eventLoop.handlers, h)
		if start {
			l.startHandler(h)
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
		if nok == 0 {
			// Accept output only node.
			nok = 1
		}
		l.addDataNode(n)
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
