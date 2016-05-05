package loop

import (
	"github.com/platinasystems/elib/cpu"
	"github.com/platinasystems/elib/event"

	"fmt"
	"sync"
	"time"
)

type Node struct {
	loop              *Loop
	rxEvents          chan event.Actor
	toLoop            chan struct{}
	fromLoop          chan struct{}
	eventVec          event.ActorVec
	active            bool
	oneShot           bool
	work              chan Worker
	dataCaller        DataCaller
	index             uint
	activePollerIndex uint
}

func (n *Node) GetNode() *Node { return n }
func (n *Node) Index() uint    { return n.index }

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

type Worker interface {
	Noder
	Work(l *Loop)
}

type Initer interface {
	LoopInit(l *Loop)
}

type Exiter interface {
	LoopExit(l *Loop)
}

type Loop struct {
	loopIniters          []Initer
	loopExiters          []Exiter
	eventPollers         []EventPoller
	eventHandlers        []EventHandler
	dataPollers          []DataPoller
	dataNodes            []dataOutNoder
	workers              []Worker
	activePollers        []activePoller
	nActivePollers       uint32
	events               chan loopEvent
	eventPool            event.Pool
	startTime            cpu.Time
	now                  cpu.Time
	cyclesPerSec         float64
	secsPerCycle         float64
	timeDurationPerCycle float64
	wg                   sync.WaitGroup
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
	for _, n := range l.dataPollers {
		c := n.GetNode()
		c.toLoop = make(chan struct{}, 1)
		c.fromLoop = make(chan struct{}, 1)
		go l.dataPoll(n)
	}
	for _, n := range l.workers {
		c := n.GetNode()
		c.work = make(chan Worker, 256)
		go l.worker(n)
	}
}

func (l *Loop) AddWork(n *Node, w Worker) {
	l.wg.Add(1)
	n.work <- w
}

func (l *Loop) worker(w Worker) {
	c := w.GetNode()
	for {
		w := <-c.work
		w.Work(l)
		l.wg.Add(-1)
	}
}

func (l *Loop) dataPoll(p DataPoller) {
	c := p.GetNode()
	for {
		<-c.fromLoop
		ap := &l.activePollers[c.activePollerIndex]
		ap.init(l, c.activePollerIndex)
		an := &ap.activeNodes[c.index]
		ap.activeNode = an
		p.Poll(l, an.callerOut)
		an.out.nextFrame.call(l, ap)
		c.toLoop <- struct{}{}
	}
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

func (l *Loop) init() {
	i := 0
	for {
		if i >= len(l.loopIniters) {
			break
		}
		l.wg.Add(1)
		go func(i int) {
			l.loopIniters[i].LoopInit(l)
			l.wg.Done()
		}(i)
		i++
	}
	l.wg.Wait()
}

func (l *Loop) exit() {
	for i := range l.loopExiters {
		l.loopExiters[i].LoopExit(l)
	}
}

func (l *Loop) Run() {
	l.init()
	l.start()
	l.startTime = cpu.TimeNow()
	for !l.doEvents() {
		l.doPollers()
	}
	l.exit()
}

func (l *Loop) Register(n Noder) {
	x := n.GetNode()
	x.loop = l

	nOK := 0
	if h, ok := n.(EventHandler); ok {
		l.eventHandlers = append(l.eventHandlers, h)
		nOK++
	}
	if d, isIO := n.(dataOutNoder); isIO {
		nok := 0
		if q, ok := d.(DataPoller); ok {
			l.dataPollers = append(l.dataPollers, q)
			nok++
		}
		if _, ok := d.(DataCaller); ok {
			nok++
		}
		if nok > 0 {
			x.index = uint(len(l.dataNodes))
			l.dataNodes = append(l.dataNodes, d)
		} else {
			panic("node missing Poll and/or Call method")
		}
		nOK += nok
	}
	if p, ok := n.(Worker); ok {
		l.workers = append(l.workers, p)
		nOK++
	}
	if p, ok := n.(Initer); ok {
		l.loopIniters = append(l.loopIniters, p)
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
