package loop

import (
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/elib/event"

	"fmt"
	"sync"
	"time"
)

type Node struct {
	name              string
	index             uint
	loop              *Loop
	rxEvents          chan event.Actor
	toLoop            chan struct{}
	fromLoop          chan struct{}
	eventVec          event.ActorVec
	active            bool
	oneShot           bool
	dataCaller        inOutLooper
	activePollerIndex uint
	initOnce          sync.Once
	initWg            sync.WaitGroup
	outIns            []LooperIn
}

func (n *Node) GetNode() *Node { return n }
func (n *Node) Index() uint    { return n.index }
func (n *Node) Name() string   { return n.name }
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

func (n *Node) activate(enable, oneShot bool) {
	if n.active != enable {
		n.active = enable
		n.oneShot = oneShot
		n.loop.countActive(enable)
	}
}

func (n *Node) Activate(enable bool)     { n.activate(enable, false) }
func (n *Node) ActivateOnce(enable bool) { n.activate(enable, true) }

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
	dataNodes              []Noder
	dataNodeByName         map[string]Noder
	loopIniters            []Initer
	loopExiters            []Exiter
	activePollers          []activePoller
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
}

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

func (l *Loop) doEvent(e event.Actor) {
	defer func() {
		if err := recover(); err == ErrQuit {
			l.Quit()
		} else if err != nil {
			fmt.Printf("%s\n", err)
			l.Quit()
		}
	}()
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
		ap := &l.activePollers[c.activePollerIndex]
		if ap.activeNodes == nil {
			ap.init(l, c.activePollerIndex)
		}
		n := &ap.activeNodes[c.index]
		ap.currentNode = n
		p.LoopInput(l, n.looperOut)
		n.out.call(l, ap)
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
	if n := len(l.dataPollers); cap(l.activePollers) < n {
		l.activePollers = make([]activePoller, n)
	}
	nActive := uint(0)
	for _, p := range l.dataPollers {
		c := p.GetNode()
		if c.active {
			l.activePollers[nActive].pollerNode = c
			c.activePollerIndex = nActive
			nActive++
			if c.oneShot {
				c.active = false
				c.oneShot = false
				l.countActive(false)
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
	l.secsPerCycle = t.Seconds()
	l.timeDurationPerCycle = l.secsPerCycle / float64(time.Second)
}

func (l *Loop) callInit(n Initer, isCall bool) {
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
func (l *Loop) CallInit(n Initer)  { l.callInit(n, true) }
func (l *Loop) startInit(n Initer) { l.callInit(n, false) }

func (l *Loop) doInit() {
	for _, i := range l.loopIniters {
		l.startInit(i)
	}
	l.wg.Wait()
}

func (l *Loop) doExit() {
	for i := range l.loopExiters {
		l.loopExiters[i].LoopExit(l)
	}
}

func (l *Loop) Run() {
	l.timerInit()
	l.startTime = cpu.TimeNow()
	l.eventInit()
	l.startPollers()
	l.registrationsNeedStart = true
	l.doInit()
	for !l.doEvents() {
		l.doPollers()
	}
	l.doExit()
}

func (l *Loop) addDataNode(r Noder) {
	n := r.GetNode()
	n.index = uint(len(l.dataNodes))
	l.dataNodes = append(l.dataNodes, r)
	if l.dataNodeByName == nil {
		l.dataNodeByName = make(map[string]Noder)
	}
	l.dataNodeByName[n.name] = r
}

func (l *Loop) Register(n Noder, format string, args ...interface{}) {
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
			l.startInit(p)
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

var defaultLoop = &Loop{}

func AddEvent(e event.Actor, h EventHandler)               { defaultLoop.AddEvent(e, h) }
func Register(n Noder, format string, args ...interface{}) { defaultLoop.Register(n, format, args...) }
func RegisterEventPoller(p EventPoller)                    { defaultLoop.RegisterEventPoller(p) }
func Run()                                                 { defaultLoop.Run() }

type quitEvent struct{}

var ErrQuit = &quitEvent{}

func (e *quitEvent) Error() string      { return "quit" }
func (e *loopEvent) isQuit() (yes bool) { _, yes = e.actor.(*quitEvent); return }
func (q *quitEvent) EventAction()       {}
func (l *Loop) Quit()                   { l.AddEvent(&quitEvent{}, nil) }
