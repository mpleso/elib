package event

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"
)

type Actor interface {
	EventAction(now cpu.Time)
}

type TimedActor interface {
	Actor
	EventTime() cpu.Time
}

//go:generate gentemplate -d Package=event -id actor  -d VecType=ActorVec -d Type=Actor github.com/platinasystems/elib/vec.tmpl

func (p *timedEventPool) Compare(i, j int) int {
	ei, ej := p.events[i], p.events[j]
	return int(ei.EventTime() - ej.EventTime())
}

//go:generate gentemplate -d Package=event -id timedEvent -d PoolType=timedEventPool -d Type=TimedActor -d Data=events github.com/platinasystems/elib/pool.tmpl

type Pool struct {
	pool    timedEventPool
	fibheap elib.FibHeap
}

func (p *Pool) Add(e TimedActor) (ei uint) {
	ei = p.pool.GetIndex()
	p.pool.events[ei] = e
	p.fibheap.Add(ei)
	return ei
}

func (p *Pool) Del(ei uint) {
	p.fibheap.Del(ei)
	p.pool.PutIndex(ei)
}

func (p *Pool) advance(t cpu.Time, iv *ActorVec) {
	for {
		ei, valid := p.fibheap.Min(&p.pool)
		if !valid {
			return
		}
		e := p.pool.events[ei]
		if e.EventTime() > t {
			break
		}
		p.fibheap.Del(ei)
		if iv != nil {
			*iv = append(*iv, e)
		}
		e.EventAction(t)
	}
}

func (p *Pool) NextTime() (t cpu.Time, valid bool) {
	ei, valid := p.fibheap.Min(&p.pool)
	if valid {
		t = p.pool.events[ei].EventTime()
	}
	return
}

func (p *Pool) Advance(t cpu.Time)                  { p.advance(t, nil) }
func (p *Pool) AdvanceAdd(t cpu.Time, iv *ActorVec) { p.advance(t, iv) }
