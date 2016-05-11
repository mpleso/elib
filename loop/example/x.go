package main

import (
	"github.com/platinasystems/elib/cli"
	"github.com/platinasystems/elib/loop"

	"fmt"
	"time"
)

type n0 struct {
	loop.Node
	calls uint
}

type N0In struct {
	loop.In
	data [loop.V]uint
}

type n0Out struct {
	loop.Out
	In N0In
}

var node0 = &n0{}

func init() { loop.Register(node0, "node0") }

func (n *n0) MakeLoopIn() loop.LooperIn   { return &N0In{} }
func (n *n0) MakeLoopOut() loop.LooperOut { return &n0Out{} }
func (n *n0) LoopInput(l *loop.Loop, out loop.LooperOut) {
	n.call(l, (*N0In)(nil), &out.(*n0Out).In)
}
func (n *n0) LoopInputOutput(l *loop.Loop, in loop.LooperIn, out loop.LooperOut) {
	n.call(l, in.(*N0In), &out.(*n0Out).In)
}
func (n *n0) LoopInit(l *loop.Loop) { time.Sleep(1 * time.Second); fmt.Printf("done\n") }

func (n *n0) call(l *loop.Loop, in *N0In, outIn *N0In) {
	done := n.calls >= 10
	if !done {
		nf := uint(len(in.data))
		if in != nil {
			nf = in.Len()
		}
		for i := uint(0); i < nf; i++ {
			outIn.data[i] = n.calls
		}
		outIn.SetLen(l, nf)
	}
	if done {
		n.Activate(false)
		n.calls = 0
	} else {
		n.calls++
	}
}

type n1 n0

var node1 = &n1{}

type N1In N0In

type n1Out struct {
	loop.Out
	Ins []N1In
}

func init() { loop.Register(node1, "node1") }

func (n *n1) MakeLoopIn() loop.LooperIn   { return &N1In{} }
func (n *n1) MakeLoopOut() loop.LooperOut { return &n1Out{} }

func (n *n1) LoopInit(l *loop.Loop) {
	l.AddNext(node1, node1)
}

func (n *n1) LoopInput(l *loop.Loop, out loop.LooperOut) {
	(*n0)(n).call(l, (*N0In)(nil), (*N0In)(&out.(*n1Out).Ins[0]))
}
func (n *n1) LoopInputOutput(l *loop.Loop, in loop.LooperIn, out loop.LooperOut) {
	(*n0)(n).call(l, (*N0In)(in.(*N1In)), (*N0In)(&out.(*n1Out).Ins[0]))
}

func init() {
	loop.CliAdd(&cli.Command{
		Name:      "a",
		ShortHelp: "a short help",
		Action: func(c cli.Commander, w cli.Writer, s *cli.Scanner) {
			node0.ActivateOnce(true)
			node1.ActivateOnce(true)
		},
	})
}

func main() {
	loop.Run()
}
