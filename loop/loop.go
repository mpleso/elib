package loop

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/elib/event"

	"fmt"
	"sync"
	"time"
)

type Node struct {
	loop     *Loop
	rxEvents chan event.Actor
	toLoop   chan struct{}
	fromLoop chan struct{}
	eventVec event.ActorVec
	active   bool
	oneShot  bool
	work     chan Worker
}

func (n *Node) GetNode() *Node { return n }

func (n *Node) ActivateOnce() {
	if !n.active {
		n.oneShot = true
		n.loop.nActivePollers++
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

type Poller interface {
	Noder
	Poll(l *Loop)
}

type Worker interface {
	Noder
	Work(l *Loop)
}

type Loop struct {
	eventPollers         []EventPoller
	eventHandlers        []EventHandler
	pollers              []Poller
	workers              []Worker
	pollerNodes          []*Node
	nActivePollers       uint32
	events               chan loopEvent
	eventPool            event.Pool
	frameHeap            elib.MemHeap
	startTime            cpu.Time
	now                  cpu.Time
	cyclesPerSec         float64
	secsPerCycle         float64
	timeDurationPerCycle float64
	workWg               sync.WaitGroup
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
			panic(err)
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

func (l *Loop) eventPoller(p EventPoller) {
	for {
		p.EventPoll()
	}
}

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

func (l *Loop) start() {
	// Initialize timer.
	t := cpu.Time(0)
	t.Cycles(1 * cpu.Second)
	l.cyclesPerSec = float64(t)
	l.secsPerCycle = t.Seconds()
	l.timeDurationPerCycle = l.secsPerCycle / float64(time.Second)

	l.events = make(chan loopEvent, 256)
	l.frameHeap.Init(64 << 20)

	for _, n := range l.eventPollers {
		go l.eventPoller(n)
	}
	for _, n := range l.eventHandlers {
		c := n.GetNode()
		c.toLoop = make(chan struct{}, 1)
		c.fromLoop = make(chan struct{}, 1)
		c.rxEvents = make(chan event.Actor, 256)
		go l.eventHandler(n)
	}
	for _, n := range l.pollers {
		c := n.GetNode()
		c.toLoop = make(chan struct{}, 1)
		c.fromLoop = make(chan struct{}, 1)
		go l.poll(n)
	}
	for _, n := range l.workers {
		c := n.GetNode()
		c.work = make(chan Worker, 256)
		go l.worker(n)
	}
}

func (n *Node) AddWork(w Worker) { n.work <- w }

func (l *Loop) worker(w Worker) {
	c := w.GetNode()
	for {
		w := <-c.work
		l.workWg.Add(1)
		w.Work(l)
		l.workWg.Add(-1)
	}
}

func (l *Loop) poll(p Poller) {
	c := p.GetNode()
	for {
		<-c.fromLoop
		p.Poll(l)
		c.toLoop <- struct{}{}
	}
}

func (l *Loop) doPollers() {
	if n := len(l.pollers); cap(l.pollerNodes) < n {
		l.pollerNodes = make([]*Node, n)
	}
	nActive := 0
	for _, p := range l.pollers {
		c := p.GetNode()
		if c.active || c.oneShot {
			l.pollerNodes[nActive] = c
			nActive++
			c.oneShot = false
			c.fromLoop <- struct{}{}
		}
	}

	// Wait for pollers to finish.
	for i := 0; i < nActive; i++ {
		<-l.pollerNodes[i].toLoop
	}

	// Wait for workers to finish.
	l.workWg.Wait()
}

func (l *Loop) Run() {
	l.start()
	l.startTime = cpu.TimeNow()
	for !l.doEvents() {
		l.doPollers()
	}
}

func (l *Loop) Register(n Noder) {
	i := 0
	if h, ok := n.(EventHandler); ok {
		l.eventHandlers = append(l.eventHandlers, h)
		i++
	}
	if p, ok := n.(Poller); ok {
		l.pollers = append(l.pollers, p)
		i++
	}
	if p, ok := n.(Worker); ok {
		l.workers = append(l.workers, p)
		i++
	}
	if i == 0 {
		panic(fmt.Errorf("unkown node type: %T", n))
	}

	x := n.GetNode()
	x.loop = l
}

func (l *Loop) RegisterEventPoller(p EventPoller) {
	l.eventPollers = append(l.eventPollers, p)
}

var defaultLoop = &Loop{}

func AddEvent(e event.Actor, h EventHandler) { defaultLoop.AddEvent(e, h) }
func Register(n Noder)                       { defaultLoop.Register(n) }
func RegisterEventPoller(p EventPoller)      { defaultLoop.RegisterEventPoller(p) }
func Run()                                   { defaultLoop.Run() }

type quitEvent struct{}

var ErrQuit = &quitEvent{}

func (e *quitEvent) Error() string      { return "quit" }
func (e *loopEvent) isQuit() (yes bool) { _, yes = e.actor.(*quitEvent); return }
func (q *quitEvent) EventAction()       {}
func (l *Loop) Quit()                   { l.AddEvent(&quitEvent{}, nil) }
