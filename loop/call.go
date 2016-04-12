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

type activeNode struct {
	index      uint32
	dataCaller DataCaller
	callerOut  CallerOut
	out        *Out
	inType     reflect.Type
	outIns     []CallerIn
	nodeStats
}

type activePoller struct {
	index             uint16
	pollerNode        *Node
	activeNode        *activeNode
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
	a.inType = reflect.TypeOf(a.dataCaller.NewIn())
	if _, ok := ap.nodeIndexByInType[a.inType]; ok {
		err = fmt.Errorf("duplicate nodes handle input type %T", a.inType)
	}
	ap.nodeIndexByInType[a.inType] = a.index

	tIn := reflect.TypeOf((*CallerIn)(nil)).Elem()
	v := reflect.ValueOf(a.callerOut).Elem()
	for i := 0; i < s.NumField(); i++ {
		fi := s.Field(i)
		ft := fi.Type
		if reflect.PtrTo(ft).Implements(tIn) {
			fv := v.Field(i).Addr()
			ini := fv.Interface().(CallerIn)
			in := ini.GetIn()
			in.nextIndex = uint32(len(a.outIns))
			a.outIns = append(a.outIns, ini)
		}
	}
	return
}

func (ap *activePoller) init(l *Loop, api uint) {
	if ap.activeNodes != nil {
		return
	}
	nNodes := uint(len(l.dataNodes))
	ap.index = uint16(api)
	ap.activeNodes = make([]activeNode, nNodes)
	for ni := range ap.activeNodes {
		a := &ap.activeNodes[ni]
		n := l.dataNodes[ni]

		a.index = uint32(ni)
		a.callerOut = n.NewOut()
		a.out = a.callerOut.GetOut()
		if d, ok := n.(DataCaller); ok {
			a.dataCaller = d
		}

		if err := a.analyzeOut(ap); err != nil {
			panic(err)
		}
	}

	for ni := range ap.activeNodes {
		a := &ap.activeNodes[ni]
		a.out.nextFrame.init(uint(len(a.outIns)))
		for xi := range a.outIns {
			in := reflect.TypeOf(a.outIns[xi])
			a.out.nextFrame.nextNodes[xi] = ap.nodeIndexByInType[in]
		}
	}
}

// Maximum vector length.
const V = 256

// Vector index.  Loop from 0 to FrameLen
type Vi uint8

type pending struct {
	in        *In
	nextIndex uint32
	nodeIndex uint32
}

type nextFrame struct {
	nextNodes []uint32
	Len       []Vi
	isPending elib.BitmapVec
	pending   []pending
}

func (f *nextFrame) init(nNext uint) {
	f.Len = make([]Vi, nNext)
	f.pending = make([]pending, 0, nNext)
	f.isPending.Alloc(nNext)
	f.nextNodes = make([]uint32, nNext)
}

func (i *In) PutNext(l *Loop, nVec uint) {
	xi := uint(i.nextIndex)
	ap := &l.activePollers[i.activeIndex]
	f := &ap.activeNode.out.nextFrame
	is := nVec > 0
	if is && !f.isPending.Set(xi, is) {
		f.pending = append(f.pending, pending{in: i, nextIndex: uint32(xi), nodeIndex: f.nextNodes[xi]})
		f.Len[xi] = Vi(nVec)
	}
}

func (f *nextFrame) call(l *Loop, a *activePoller) {
	if len(f.pending) == 0 {
		return
	}
	var t0 cpu.Time
	t0 = cpu.TimeNow()
	i := 0
	prev := a.activeNode
	for {
		if i >= len(f.pending) {
			break
		}
		p := &f.pending[i]
		i++
		ni, xi := p.nodeIndex, p.nextIndex
		next := &a.activeNodes[ni]
		in := p.in
		nVec := uint16(f.Len[xi])
		if nVec == 0 {
			nVec = uint16(V)
		}
		in.activeIndex = uint16(a.index)
		in.len = uint16(nVec)
		f.Len[xi] = 0
		f.isPending.Unset(uint(ni))

		a.activeNode = next
		next.dataCaller.Call(l, prev.outIns[xi], next.callerOut)

		t := cpu.TimeNow()
		next.calls++
		next.vectors += uint64(nVec)
		next.clocks += t - t0
		t0 = t
	}
	f.pending = f.pending[:0]
}

type Out struct{ nextFrame nextFrame }

func (o *Out) GetOut() *Out { return o }
func (o *Out) NewOut() *Out { return &Out{} }

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

type dataOutNoder interface {
	Noder
	NewOut() CallerOut
}

type DataPoller interface {
	dataOutNoder
	Poll(l *Loop, o CallerOut)
}

type DataCaller interface {
	dataOutNoder
	NewIn() CallerIn
	Call(l *Loop, i CallerIn, o CallerOut)
}
