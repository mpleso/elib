package event

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"
)

type Interface interface {
	EventAction(now cpu.Time)
}

//go:generate gentemplate -d Package=event -id interface  -d VecType=InterfaceVec -d Type=Interface github.com/platinasystems/elib/vec.tmpl

type timedEvent struct {
	timestamp cpu.Time
	i         Interface
}

func (p *timedEventPool) Compare(i, j int) int {
	ei, ej := &p.events[i], &p.events[j]
	return int(ei.timestamp - ej.timestamp)
}

//go:generate gentemplate -d Package=event -id timedEvent -d PoolType=timedEventPool -d Type=timedEvent -d Data=events github.com/platinasystems/elib/pool.tmpl

type Pool struct {
	pool    timedEventPool
	fibheap elib.FibHeap
}

func (p *Pool) Add(t cpu.Time, i Interface) (ei uint) {
	ei = p.pool.GetIndex()
	p.pool.events[ei] = timedEvent{timestamp: t, i: i}
	p.fibheap.Add(ei)
	return ei
}

func (p *Pool) Del(ei uint) {
	p.fibheap.Del(ei)
	p.pool.PutIndex(ei)
}

func (p *Pool) advance(t cpu.Time, iv *InterfaceVec) {
	if p.pool.Elts() == 0 {
		return
	}
	for {
		ei := p.fibheap.Min(&p.pool)
		e := &p.pool.events[ei]
		if e.timestamp > t {
			break
		}
		p.fibheap.Del(ei)
		if iv != nil {
			*iv = append(*iv, e.i)
		}
		e.i.EventAction(t)
	}
}

func (p *Pool) Advance(t cpu.Time)                   { p.advance(t, nil) }
func (iv *InterfaceVec) Advance(p *Pool, t cpu.Time) { p.advance(t, iv) }
