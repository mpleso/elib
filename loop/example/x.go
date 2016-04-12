package main

import (
	"github.com/platinasystems/elib/cli"
	"github.com/platinasystems/elib/iomux"
	"github.com/platinasystems/elib/loop"

	"fmt"
)

type myNode struct {
	loop.Node
	calls uint
}

type MyIn struct {
	loop.In
	data [loop.V]uint
}

type myOut struct {
	loop.Out
	MyIn
}

var myN = &myNode{}

func init() { loop.Register(myN) }

func (n *myNode) NewIn() loop.CallerIn                                    { return &MyIn{} }
func (n *myNode) NewOut() loop.CallerOut                                  { return &myOut{} }
func (n *myNode) Poll(l *loop.Loop, out loop.CallerOut)                   { call(l, n, (*MyIn)(nil), out) }
func (n *myNode) Call(l *loop.Loop, in loop.CallerIn, out loop.CallerOut) { call(l, n, in, out) }

func call(l *loop.Loop, n *myNode, ci loop.CallerIn, co loop.CallerOut) {
	in, o := ci.(*MyIn), co.(*myOut)
	done := n.calls >= 10
	if !done {
		nf := uint(len(in.data))
		if in != nil {
			nf = in.Len()
		}
		for i := uint(0); i < nf; i++ {
			o.data[i] = n.calls
		}
		o.MyIn.PutNext(l, nf)
	}
	fmt.Fprintf(cli.Default, "myNode poll %p %d\n", o, n.calls)
	if done {
		n.Activate(false)
		n.calls = 0
	} else {
		n.calls++
	}
}

func main() {
	cli.Default.AddStdin()

	cli.AddCommand(&cli.Command{
		Name:      "a",
		ShortHelp: "a short help",
		Action: func(c cli.Commander, w cli.Writer, s *cli.Scanner) {
			myN.ActivateOnce(true)
			fmt.Fprintf(w, "%T\n", c)
		},
	})
	cli.Default.Start()

	loop.RegisterEventPoller(iomux.Default)
	loop.Register(cli.Default)
	loop.Run()

	cli.Default.End()
}