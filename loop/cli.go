package loop

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cli"
	"github.com/platinasystems/elib/iomux"

	"fmt"
	"sort"
)

type LoopCli struct {
	cli.Main
	Node
}

var Cli = &LoopCli{}

func init() {
	RegisterEventPoller(iomux.Default)
	Register(Cli, "loop-cli")
}

type CliFile struct{ *cli.File }

func rxReady(f *cli.File) {
	AddEvent(&CliFile{File: f}, Cli)
}

func (c *CliFile) EventAction() {
	if err := c.RxReady(); err == cli.ErrQuit {
		AddEvent(ErrQuit, nil)
	}
}

func (c *LoopCli) EventHandler() {}

func (c *LoopCli) LoopInit(l *Loop) {
	c.Main.RxReady = rxReady
	c.Main.Prompt = "cli# "
	c.Main.AddStdin()
	c.Main.Start()
}

func (c *LoopCli) LoopExit(l *Loop) {
	c.Main.End()
}

func CliAdd(c *cli.Command) { Cli.Main.AddCommand(c) }

func Logf(format string, args ...interface{})             { fmt.Fprintf(&Cli.Main, format, args...) }
func Fatalf(format string, args ...interface{})           { panic(fmt.Errorf(format, args...)) }
func (l *Loop) Logf(format string, args ...interface{})   { Logf(format, args...) }
func (l *Loop) Fatalf(format string, args ...interface{}) { Fatalf(format, args...) }

type rtNode struct {
	Name    string  `format:"%-30s"`
	Calls   uint64  `format:"%16d"`
	Vectors uint64  `format:"%16d"`
	Clocks  float64 `format:"%16.2f"`
}
type rtNodes []rtNode

func (ns rtNodes) Less(i, j int) bool { return ns[i].Name < ns[j].Name }
func (ns rtNodes) Swap(i, j int)      { ns[i], ns[j] = ns[j], ns[i] }
func (ns rtNodes) Len() int           { return len(ns) }

func (l *Loop) showRuntimeStats(c cli.Commander, w cli.Writer, s *cli.Scanner) {
	ns := rtNodes(make([]rtNode, len(l.dataNodes)))
	for i := range l.dataNodes {
		n := l.dataNodes[i].GetNode()
		var s nodeStats
		for _, a := range l.activePollers {
			if a.activeNodes != nil {
				s.add(&a.activeNodes[i])
			}
		}
		ns[i] = rtNode{
			Name:    n.name,
			Calls:   s.calls,
			Vectors: s.vectors,
			Clocks:  s.clocksPerVector(),
		}
	}

	// Summary
	{
		var s pollerStats
		for _, a := range l.activePollers {
			s.add(a)
		}
		if s.calls > 0 {
			vecsPerSec := float64(s.vectors) / l.Seconds(s.nonIdleClocks)
			clocksPerVec := float64(s.nonIdleClocks) / float64(s.vectors)
			vecsPerCall := float64(s.vectors) / float64(s.calls)
			fmt.Fprintf(w, "Vectors: %d, Vectors/sec: %.2e, Clocks/vector: %.2f, Vectors/call %.2f\n",
				s.vectors, vecsPerSec, clocksPerVec, vecsPerCall)
		}
	}

	sort.Sort(ns)
	elib.TabulateWrite(w, ns)
}

func (l *Loop) clearRuntimeStats(c cli.Commander, w cli.Writer, s *cli.Scanner) {
	for _, a := range l.activePollers {
		a.statsLastClear = a.pollerStats
		for j := range a.activeNodes {
			a.activeNodes[j].statsLastClear = a.activeNodes[j].nodeStats
		}
	}
}

func init() {
	CliAdd(&cli.Command{
		Name:      "show runtime",
		ShortHelp: "show main loop runtime statistics",
		Action:    defaultLoop.showRuntimeStats,
	})
	CliAdd(&cli.Command{
		Name:      "clear runtime",
		ShortHelp: "clear main loop runtime statistics",
		Action:    defaultLoop.clearRuntimeStats,
	})
}
