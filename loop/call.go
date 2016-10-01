package loop

import (
	"github.com/platinasystems/go/elib"
	"github.com/platinasystems/go/elib/cpu"

	"fmt"
	"reflect"
)

type stats struct {
	calls, vectors uint64
	clocks         cpu.Time
}

type nodeStats struct {
	current, lastClear stats
}

func (s *nodeStats) clear() { s.lastClear = s.current }
func (s *nodeStats) zero() {
	var z stats
	s.current = z
	s.lastClear = z
}

func (s *stats) add_helper(n *nodeStats, raw bool) {
	c, v, l := n.current.calls, n.current.vectors, n.current.clocks
	if !raw {
		c -= n.lastClear.calls
		v -= n.lastClear.vectors
		l -= n.lastClear.clocks
	}
	s.calls += c
	s.vectors += v
	s.clocks += l
}

func (s *stats) add(n *nodeStats)     { s.add_helper(n, false) }
func (s *stats) add_raw(n *nodeStats) { s.add_helper(n, true) }

func (s *stats) clocksPerVector() (v float64) {
	if s.vectors != 0 {
		v = float64(s.clocks) / float64(s.vectors)
	}
	return
}

type activeNode struct {
	// Index in activePoller.activeNodes and also loop.dataNodes.
	index                   uint32
	loopInMaker             loopInMaker
	inOutLooper             inOutLooper
	outLooper               outLooper
	looperOut               LooperOut
	out                     *Out
	outIns                  looperInVec
	outSlice                *reflect.Value
	inputStats, outputStats nodeStats
}

//go:generate gentemplate -d Package=loop -id looperIn -d VecType=looperInVec -d Type=LooperIn github.com/platinasystems/go/elib/vec.tmpl

type activePoller struct {
	index       uint16
	timeNow     cpu.Time
	pollerNode  *Node
	currentNode *activeNode
	activeNodes []activeNode
	pending     []pending

	pollerStats nodeStats
}

//go:generate gentemplate -d Package=loop -id activePoller -d PoolType=activePollerPool -d Type=*activePoller -d Data=entries github.com/platinasystems/go/elib/pool.tmpl

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
			err = fmt.Errorf("loop.LooperIn field `%s' %s", s.Field(i).Name, err)
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
	n := l.DataNodes[a.index].GetNode()
	for i := range ins {
		nn := &n.nextNodes[i]
		nn.in = ins[i]
		a.addNext(ap, nn, uint(i))
	}
	if len(inSlices) > 0 {
		a.outSlice = &inSlices[0]
		for i := range n.nextNodes {
			a.addNext(ap, &n.nextNodes[i], uint(i))
		}
	}
	return
}

func ithLooperIn(as reflect.Value, i int) LooperIn { return as.Index(i).Addr().Interface().(LooperIn) }

func (a *activeNode) addNext(ap *activePoller, nn *nextNode, withIndex uint) {
	i := nn.in
	in := i.GetIn()
	in.activeIndex = ap.index
	x := int(withIndex)
	in.nextIndex = uint32(x)
	oi := i
	if a.outSlice != nil {
		as := *a.outSlice

		c, l := as.Cap(), as.Len()
		if x >= c {
			c = int(elib.NextResizeCap(elib.Index(x)))
			na := reflect.MakeSlice(as.Type(), c, c)
			reflect.Copy(na, as)
			for j := 0; j < l; j++ {
				a.outIns[j] = ithLooperIn(na, j)
			}
			as = na
		}

		if l <= x {
			l = x + 1
		}

		as = as.Slice(0, l)

		// In as a reflect value.
		vi := reflect.ValueOf(i).Elem().Convert(as.Type().Elem())
		as.Index(x).Set(vi)
		oi = ithLooperIn(as, x)

		(*a.outSlice).Set(as)
	}
	a.outIns.Validate(uint(x))
	a.outIns[x] = oi
	a.out.addNext(uint(x), nn.nodeIndex)
}

func (n *Node) findNext(name string, create bool) (x uint, ok bool) {
	if n.nextIndexByNodeName == nil {
		n.nextIndexByNodeName = make(map[string]uint)
	}
	if x, ok = n.nextIndexByNodeName[name]; !ok && create {
		x = uint(len(n.nextNodes))
		n.nextIndexByNodeName[name] = x
	}
	return
}

func (l *Loop) AddNamedNextWithIndex(nr Noder, nextName string, withIndex uint) (nextIndex uint, err error) {
	n := nr.GetNode()

	var (
		xr Noder
		xi inNoder
		x  *Node
		ok bool
	)
	if l.initialNodesRegistered {
		if xr, ok = l.dataNodeByName[nextName]; !ok {
			err = fmt.Errorf("add-next %s: unknown next %s", n.name, nextName)
			return
		}
		if xi, ok = xr.(inNoder); !ok {
			err = fmt.Errorf("add-next %s: %s is not input node", n.name, nextName)
			return
		}
		x = xr.GetNode()
	}

	if nextIndex, ok = n.findNext(nextName, true); ok {
		if nextIndex != withIndex && withIndex != ^uint(0) {
			err = fmt.Errorf("add-next %s: inconsistent next for %s", n.name, nextName)
		}
		withIndex = nextIndex
	}

	if withIndex == ^uint(0) {
		withIndex = n.nextNodes.Len()
	}
	nextIndex = withIndex
	n.nextNodes.Validate(nextIndex)
	nn := &n.nextNodes[nextIndex]

	nn.name = nextName

	if xr != nil {
		nn.nodeIndex = x.index
		nn.in = xi.MakeLoopIn()
		for i := range l.activePollerPool.entries {
			p := l.activePollerPool.entries[i]
			if p != nil {
				p.activeNodes[n.index].addNext(p, nn, withIndex)
			}
		}
	}
	return
}

func (l *Loop) AddNextWithIndex(n Noder, x inNoder, withIndex uint) (uint, error) {
	return l.AddNamedNextWithIndex(n, nodeName(x), withIndex)
}
func (l *Loop) AddNext(n Noder, x inNoder) (uint, error) { return l.AddNextWithIndex(n, x, ^uint(0)) }

func (l *Loop) AddNamedNext(thisNoder Noder, nextName string) (uint, error) {
	return l.AddNamedNextWithIndex(thisNoder, nextName, ^uint(0))
}

func (l *Loop) graphInit() {
	l.initialNodesRegistered = true
	for _, n := range l.DataNodes {
		x := n.GetNode()
		for i := range x.nextNodes {
			if xn := &x.nextNodes[i]; len(xn.name) > 0 {
				if _, err := l.AddNamedNextWithIndex(n, xn.name, uint(i)); err != nil {
					panic(err)
				}
			}
		}
	}
}

func (ap *activePoller) initNodes(l *Loop) {
	nNodes := uint(len(l.DataNodes))
	ap.activeNodes = make([]activeNode, nNodes)
	for ni := range ap.activeNodes {
		a := &ap.activeNodes[ni]
		n := l.DataNodes[ni]

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
}

// Maximum vector length.
const MaxVectorLen = 256

// Vector index.
type Vi uint8

//go:generate gentemplate -d Package=loop -id Vi -d VecType=viVec -d Type=Vi github.com/platinasystems/go/elib/vec.tmpl

type pending struct {
	in            *In
	out           *Out
	nodeIndex     uint32
	nextIndex     uint32
	nextNodeIndex uint32
}

type Out struct {
	Len       viVec
	nextNodes elib.Uint32Vec
	isPending elib.BitmapVec
}

func (f *Out) addNext(i, next_node_index uint) {
	f.Len.Validate(i)
	f.nextNodes.Validate(i)
	f.isPending.Alloc(i + 1)
	f.nextNodes[i] = uint32(next_node_index)
}

// Fetch out frame for current active node.
func (i *In) currentThread(l *Loop) *activePoller { return l.activePollerPool.entries[i.activeIndex] }
func (i *In) currentOut(l *Loop) *Out             { return i.currentThread(l).currentNode.out }

// Set vector length for given in.
// As vector length becomes positive, add to pending vector.
func (i *In) SetLen(l *Loop, nVec uint) {
	xi, a, o := uint(i.nextIndex), i.currentThread(l), i.currentOut(l)
	o.Len[xi] = Vi(nVec)
	if isPending := nVec > 0; isPending && !o.isPending.Set(xi, isPending) {
		a.pending = append(a.pending, pending{
			in:            i,
			out:           o,
			nodeIndex:     a.currentNode.index,
			nextNodeIndex: o.nextNodes[xi],
			nextIndex:     uint32(xi),
		})
	}
}
func (i *In) GetLen(l *Loop) uint {
	xi, o := uint(i.nextIndex), i.currentOut(l)
	return uint(o.Len[xi])
}

func (f *Out) nextVectors(xi uint) (nVec uint) {
	nVec = uint(f.Len[xi])
	if nVec == 0 {
		nVec = MaxVectorLen
	}
	return
}

func (f *Out) totalVectors(a *activePoller) (nVec uint) {
	for i := range a.pending {
		p := &a.pending[i]
		nVec += f.nextVectors(uint(p.nextIndex))
	}
	return
}

func (n *nodeStats) update(nVec uint, tStart cpu.Time) (tNow cpu.Time) {
	tNow = cpu.TimeNow()
	s := &n.current
	s.calls++
	s.vectors += uint64(nVec)
	s.clocks += tNow - tStart
	return
}

func (f *Out) call(l *Loop, a *activePoller) (nVec uint) {
	prevNode := a.currentNode
	nVec = f.totalVectors(a)
	a.timeNow = prevNode.inputStats.update(nVec, a.timeNow)
	if nVec == 0 {
		return
	}
	pendingIndex := 0
	t0 := a.timeNow
	for {
		// Advance pending
		if pendingIndex >= len(a.pending) {
			break
		}
		p := &a.pending[pendingIndex]
		pendingIndex++

		// Fetch next node.
		ni, xi := p.nextNodeIndex, p.nextIndex
		next := &a.activeNodes[ni]
		prevNode := &a.activeNodes[p.nodeIndex]

		// Determine vector length; 0 on pending vector means wrap to V (256).
		o := p.out
		nextN := o.nextVectors(uint(xi))

		in := p.in
		in.activeIndex = uint16(a.index)
		in.len = uint16(nextN)

		// Reset this frame.
		o.Len[xi] = 0
		o.isPending.Unset(uint(xi))

		// Call next node.
		a.currentNode = next
		nextIn := prevNode.outIns[xi]
		if next.inOutLooper != nil {
			next.inOutLooper.LoopInputOutput(l, nextIn, next.looperOut)
		} else {
			next.outLooper.LoopOutput(l, nextIn)
		}

		t0 = next.outputStats.update(nextN, t0)
	}
	a.pending = a.pending[:0]
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
