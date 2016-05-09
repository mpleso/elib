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
	N0In
}

var node0 = &n0{}

func init() { loop.Register(node0, "node0") }

func (n *n0) NewIn() loop.CallerIn                                    { return &N0In{} }
func (n *n0) NewOut() loop.CallerOut                                  { return &n0Out{} }
func (n *n0) Poll(l *loop.Loop, out loop.CallerOut)                   { call(l, n, (*N0In)(nil), out) }
func (n *n0) Call(l *loop.Loop, in loop.CallerIn, out loop.CallerOut) { call(l, n, in, out) }
func (n *n0) LoopInit(l *loop.Loop)                                   { time.Sleep(1 * time.Second); fmt.Printf("done\n") }

func call(l *loop.Loop, n *n0, ci loop.CallerIn, co loop.CallerOut) {
	in, out := ci.(*N0In), co.(*n0Out)
	done := n.calls >= 10
	if !done {
		nf := uint(len(in.data))
		if in != nil {
			nf = in.Len()
		}
		for i := uint(0); i < nf; i++ {
			out.data[i] = n.calls
		}
		out.N0In.SetLen(l, nf)
	}
	l.Logf("%s: poll %p %d\n", n.Name(), out, n.calls)
	if done {
		n.Activate(false)
		n.calls = 0
	} else {
		n.calls++
	}
}

type n1 struct{ loop.Node }

type N1In struct {
	loop.In
	data [loop.V]uint
}

type n1Out struct {
	loop.Out
	ins []loop.CallerIn
}

func init() { loop.Register(&n1{}, "node1") }

func (n *n1) NewIn() loop.CallerIn                                    { return &N1In{} }
func (n *n1) NewOut() loop.CallerOut                                  { return &n1Out{} }
func (n *n1) Call(l *loop.Loop, in loop.CallerIn, out loop.CallerOut) {}

func init() {
	loop.CliAdd(&cli.Command{
		Name:      "a",
		ShortHelp: "a short help",
		Action: func(c cli.Commander, w cli.Writer, s *cli.Scanner) {
			node0.ActivateOnce(true)
			fmt.Fprintf(w, "%T\n", c)
		},
	})
}

func main() {
	loop.Run()
}
