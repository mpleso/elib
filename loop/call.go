package loop

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cpu"

	"fmt"
	"reflect"
)

type nodeStats struct {
	calls, vectors uint64
	clocks         cpu.Time
}

func (s *nodeStats) add(a *activeNode) {
	s.calls += a.calls - a.statsLastClear.calls
	s.vectors += a.vectors - a.statsLastClear.vectors
	s.clocks += a.clocks - a.statsLastClear.clocks
}

func (s *nodeStats) clocksPerVector() (v float64) {
	if s.vectors != 0 {
		v = float64(s.clocks) / float64(s.vectors)
	}
	return
}

type activeNode struct {
	// Index in activePoller.activeNodes and also loop.dataNodes.
	index      uint32
	dataCaller inOutLooper
	callerOut  CallerOut
	out        *Out
	outIns     []CallerIn

	nodeStats
	statsLastClear nodeStats
}

type activePoller struct {
	index             uint16
	pollerNode        *Node
	currentNode       *activeNode
	activeNodes       []activeNode
	nodeIndexByInType map[reflect.Type]uint32
}

func (a *activeNode) analyzeOut(ap *activePoller) (err error) {
	ptr := reflect.TypeOf(a.callerOut)
	if ptr.Kind() != reflect.Ptr {
		err = fmt.Errorf("not pointer")
		return
	}
	s := ptr.Elem()
	if s.Kind() != reflect.Struct {
		err = fmt.Errorf("not struct")
		return
	}

	if ap.nodeIndexByInType == nil {
		ap.nodeIndexByInType = make(map[reflect.Type]uint32)
	}
	inType := reflect.TypeOf(a.dataCaller.NewLoopIn())
	if _, ok := ap.nodeIndexByInType[inType]; ok {
		err = fmt.Errorf("duplicate nodes handle input type %T", inType)
		return
	}
	ap.nodeIndexByInType[inType] = a.index

	tIn := reflect.TypeOf((*CallerIn)(nil)).Elem()
	v := reflect.ValueOf(a.callerOut).Elem()
	for i := 0; i < s.NumField(); i++ {
		if reflect.PtrTo(s.Field(i).Type).Implements(tIn) {
			ini := v.Field(i).Addr().Interface().(CallerIn)
			a.addNext(ini)
		}
	}
	return
}

func (a *activeNode) addNext(i CallerIn) {
	in := i.GetIn()
	in.nextIndex = uint32(len(a.outIns))
	a.outIns = append(a.outIns, i)
}

func (ap *activePoller) init(l *Loop, api uint) {
	nNodes := uint(len(l.dataNodes))
	ap.index = uint16(api)
	ap.activeNodes = make([]activeNode, nNodes)
	for ni := range ap.activeNodes {
		a := &ap.activeNodes[ni]
		n := l.dataNodes[ni]

		a.index = uint32(ni)
		if d, ok := n.(outNoder); ok {
			a.callerOut = d.NewLoopOut()
			a.out = a.callerOut.GetOut()
		}
		if d, ok := n.(inOutLooper); ok {
			a.dataCaller = d
		}

		if err := a.analyzeOut(ap); err != nil {
			panic(err)
		}
	}

	for ni := range ap.activeNodes {
		a := &ap.activeNodes[ni]
		a.out.alloc(uint(len(a.outIns)))
		for xi := range a.outIns {
			in := reflect.TypeOf(a.outIns[xi])
			a.out.nextNodes[xi] = ap.nodeIndexByInType[in]
		}
	}
}

// Maximum vector length.
const V = 256

// Vector index.
type Vi uint8

type pending struct {
	in        *In
	nextIndex uint32
	nodeIndex uint32
}

type Out struct {
	Len       []Vi
	nextNodes []uint32
	isPending elib.BitmapVec
	pending   []pending
}

func (f *Out) alloc(nNext uint) {
	f.Len = make([]Vi, nNext)
	f.pending = make([]pending, 0, nNext)
	f.isPending.Alloc(nNext)
	f.nextNodes = make([]uint32, nNext)
}

// Fetch out frame for current active node.
func (i *In) currentOut(l *Loop) *Out { return l.activePollers[i.activeIndex].currentNode.out }

// Set vector length for given in.
// As vector length becomes positive, add to pending vector.
func (i *In) SetLen(l *Loop, nVec uint) {
	xi, o := uint(i.nextIndex), i.currentOut(l)
	o.Len[xi] = Vi(nVec)
	if isPending := nVec > 0; isPending && !o.isPending.Set(xi, isPending) {
		o.pending = append(o.pending, pending{in: i, nextIndex: uint32(xi), nodeIndex: o.nextNodes[xi]})
	}
}

func (f *Out) call(l *Loop, a *activePoller) {
	if len(f.pending) == 0 {
		return
	}
	var t0 cpu.Time
	t0 = cpu.TimeNow()
	pendingIndex := 0
	prevNode := a.currentNode
	for {
		// Advance pending
		if pendingIndex >= len(f.pending) {
			break
		}
		p := &f.pending[pendingIndex]
		pendingIndex++

		// Fetch next node.
		ni, xi := p.nodeIndex, p.nextIndex
		next := &a.activeNodes[ni]

		// Determine vector length; 0 on pending vector means wrap to V (256).
		nVec := uint16(f.Len[xi])
		if nVec == 0 {
			nVec = uint16(V)
		}

		in := p.in
		in.activeIndex = uint16(a.index)
		in.len = uint16(nVec)

		// Reset this frame.
		f.Len[xi] = 0
		f.isPending.Unset(uint(xi))

		// Call next node.
		a.currentNode = next
		next.dataCaller.LoopInputOutput(l, prevNode.outIns[xi], next.callerOut)

		t := cpu.TimeNow()
		next.calls++
		next.vectors += uint64(nVec)
		next.clocks += t - t0
		t0 = t
	}
	f.pending = f.pending[:0]
}

func (o *Out) GetOut() *Out { return o }

type CallerOut interface {
	GetOut() *Out
}

type In struct {
	len         uint16
	activeIndex uint16
	nextIndex   uint32
}

func (i *In) GetIn() *In { return i }
func (i *In) Len() uint  { return uint(i.len) }

type CallerIn interface {
	GetIn() *In
}

type outer interface {
	NewLoopOut() CallerOut
}

type iner interface {
	NewLoopIn() CallerIn
}

type inNoder interface {
	Noder
	iner
}

type outNoder interface {
	Noder
	outer
}

type inLooper interface {
	Noder
	outer
	LoopInput(l *Loop, o CallerOut)
}

type outLooper interface {
	Noder
	iner
	LoopOutput(l *Loop, i CallerIn)
}

type inOutLooper interface {
	Noder
	iner
	outer
	LoopInputOutput(l *Loop, i CallerIn, o CallerOut)
}
