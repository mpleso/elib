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

type pollerStats struct {
	nonIdleClocks cpu.Time
	calls         uint64
	vectors       uint64
}

func (s *pollerStats) add(a *activePoller) {
	s.nonIdleClocks += a.nonIdleClocks - a.statsLastClear.nonIdleClocks
	s.vectors += a.vectors - a.statsLastClear.vectors
	s.calls += a.calls - a.statsLastClear.calls
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
	index       uint32
	loopInMaker loopInMaker
	inOutLooper inOutLooper
	outLooper   outLooper
	looperOut   LooperOut
	out         *Out
	outIns      []LooperIn
	outSlice    *reflect.Value

	nodeStats
	statsLastClear nodeStats
}

type activePoller struct {
	index       uint16
	timeNow     cpu.Time
	pollerNode  *Node
	currentNode *activeNode
	activeNodes []activeNode
	pollerStats
	statsLastClear pollerStats
}

var looperInType = reflect.TypeOf((*LooperIn)(nil)).Elem()

func asLooperIn(v reflect.Value) (in LooperIn, err error) {
	if reflect.PtrTo(v.Type()).Implements(looperInType) {
		if a := v.Addr(); a.CanInterface() {
			in = a.Interface().(LooperIn)
		} else {
			err = fmt.Errorf("value must be exported")
		}
	}
	return
}

func asLooperInSlice(v reflect.Value) (slice reflect.Value, ok bool) {
	if ok = v.Kind() == reflect.Slice &&
		reflect.PtrTo(v.Type().Elem()).Implements(looperInType); ok {
		slice = v
	}
	return
}

func (a *activeNode) inType() reflect.Type {
	return reflect.TypeOf(a.loopInMaker.MakeLoopIn())
}

func (a *activeNode) analyze(l *Loop, ap *activePoller) (err error) {
	if a.looperOut == nil {
		return
	}

	ptr := reflect.TypeOf(a.looperOut)
	if ptr.Kind() != reflect.Ptr {
		err = fmt.Errorf("not pointer")
		return
	}
	s := ptr.Elem()
	if s.Kind() != reflect.Struct {
		err = fmt.Errorf("not struct")
		return
	}

	v := reflect.ValueOf(a.looperOut).Elem()
	ins := []LooperIn{}
	inSlices := []reflect.Value{}
	for i := 0; i < s.NumField(); i++ {
		vi := v.Field(i)
		var ini LooperIn
		ini, err = asLooperIn(vi)
		if err != nil {
			err = fmt.Errorf("loop.LooperIn field `%s' must be exported", s.Field(i).Name)
			return
		}
		if ini != nil {
			ins = append(ins, ini)
		} else if s, ok := asLooperInSlice(vi); ok {
			inSlices = append(inSlices, s)
		}
	}
	if len(ins)+len(inSlices) == 0 {
		err = fmt.Errorf("data node has no inputs")
		return
	}
	if len(inSlices) > 1 {
		err = fmt.Errorf("data node has more than one slice input")
		return
	}
	for i := range ins {
		a.addNext(ins[i])
	}
	if len(inSlices) > 0 {
		a.outSlice = &inSlices[0]
		n := l.dataNodes[a.index].GetNode()
		for i := range n.outIns {
			a.addNext(n.outIns[i])
		}
	}
	return
}

func (a *activeNode) addNext(i LooperIn) {
	in := i.GetIn()
	in.nextIndex = uint32(len(a.outIns))
	oi := i
	if a.outSlice != nil {
		as := *a.outSlice
		sliceType := as.Type().Elem()
		ai := int(in.nextIndex)
		vi := reflect.ValueOf(i).Elem().Convert(sliceType)
		as = reflect.Append(as, vi)
		oi = as.Index(ai).Addr().Interface().(LooperIn)
		(*a.outSlice).Set(as)
	}
	a.outIns = append(a.outIns, oi)
}

func (this *Node) findNext(next *Node, create bool) (nextIndex uint, found bool) {
	if this.nextIndexByNodeIndex == nil {
		this.nextIndexByNodeIndex = make(map[uint]uint)
	}
	if nextIndex, found = this.nextIndexByNodeIndex[next.index]; !found && create {
		nextIndex = uint(len(this.outIns))
		this.nextIndexByNodeIndex[next.index] = nextIndex
	}
	return
}

func (l *Loop) AddNext(thisNoder Noder, nextNoder inNoder) (nextIndex uint) {
	this, next := thisNoder.GetNode(), nextNoder.GetNode()

	var ok bool
	if nextIndex, ok = this.findNext(next, true); ok {
		return
	}

	li := nextNoder.MakeLoopIn()
	this.outIns = append(this.outIns, li)
	this.nodeIndexByNext = append(this.nodeIndexByNext, next.index)
	for i := range l.activePollers {
		l.activePollers[i].activeNodes[this.index].addNext(li)
	}
	return
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
			a.looperOut = d.MakeLoopOut()
			a.out = a.looperOut.GetOut()
		}
		if d, ok := n.(loopInMaker); ok {
			a.loopInMaker = d
		}
		if d, ok := n.(inOutLooper); ok {
			a.inOutLooper = d
		}
		if d, ok := n.(outLooper); ok {
			a.outLooper = d
		}
		if err := a.analyze(l, ap); err != nil {
			l.Fatalf("%s: %s", nodeName(n), err)
		}
	}

	for ni := range ap.activeNodes {
		a := &ap.activeNodes[ni]
		if a.out == nil {
			continue
		}
		a.out.alloc(uint(len(a.outIns)))
		for xi := range a.outIns {
			oi := a.outIns[xi]
			aNode := l.dataNodes[a.index].GetNode()
			a.out.nextNodes[xi] = uint32(aNode.nodeIndexByNext[xi])
			i := oi.GetIn()
			i.activeIndex = ap.index
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

func (f *Out) nextVectors(xi uint) (nVec uint) {
	nVec = uint(f.Len[xi])
	if nVec == 0 {
		nVec = V
	}
	return
}

func (f *Out) totalVectors() (nVec uint) {
	for i := range f.pending {
		p := &f.pending[i]
		nVec += f.nextVectors(uint(p.nextIndex))
	}
	return
}

func (n *activeNode) updateStats(nVec uint, tStart cpu.Time) (tNow cpu.Time) {
	tNow = cpu.TimeNow()
	n.calls++
	n.vectors += uint64(nVec)
	n.clocks += tNow - tStart
	return
}

func (f *Out) call(l *Loop, a *activePoller) (nVec uint) {
	prevNode := a.currentNode
	nVec = f.totalVectors()
	a.timeNow = prevNode.updateStats(nVec, a.timeNow)
	if nVec == 0 {
		return
	}
	pendingIndex := 0
	t0 := a.timeNow
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
		nextN := f.nextVectors(uint(xi))

		in := p.in
		in.activeIndex = uint16(a.index)
		in.len = uint16(nextN)

		// Reset this frame.
		f.Len[xi] = 0
		f.isPending.Unset(uint(xi))

		// Call next node.
		a.currentNode = next
		nextIn := prevNode.outIns[xi]
		if next.out != nil {
			next.inOutLooper.LoopInputOutput(l, nextIn, next.looperOut)
		} else {
			next.outLooper.LoopOutput(l, nextIn)
		}

		t0 = next.updateStats(nextN, t0)
	}
	f.pending = f.pending[:0]
	a.timeNow = t0
	return
}

func (o *Out) GetOut() *Out { return o }

type In struct {
	len         uint16
	activeIndex uint16
	nextIndex   uint32
}

func (i *In) GetIn() *In     { return i }
func (i *In) Len() uint      { return uint(i.len) }
func (i *In) ThreadId() uint { return uint(i.activeIndex) }

type LooperOut interface {
	GetOut() *Out
}

type LooperIn interface {
	GetIn() *In
}

type loopOutMaker interface {
	MakeLoopOut() LooperOut
}

type loopInMaker interface {
	MakeLoopIn() LooperIn
}

type inNoder interface {
	Noder
	loopInMaker
}

type outNoder interface {
	Noder
	loopOutMaker
}

type InputLooper interface {
	loopOutMaker
	LoopInput(l *Loop, o LooperOut)
}

type OutputLooper interface {
	loopInMaker
	LoopOutput(l *Loop, i LooperIn)
}

type inLooper interface {
	Noder
	InputLooper
}

type outLooper interface {
	Noder
	OutputLooper
}

type inOutLooper interface {
	Noder
	loopInMaker
	loopOutMaker
	LoopInputOutput(l *Loop, i LooperIn, o LooperOut)
}
