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

func (l *Loop) Logf(format string, args ...interface{})   { fmt.Fprintf(&Cli.Main, format, args...) }
func (l *Loop) Fatalf(format string, args ...interface{}) { panic(fmt.Errorf(format, args...)) }

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
		for j := range l.activePollers {
			a := &l.activePollers[j]
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

	sort.Sort(ns)
	fmt.Fprintf(w, "%s", elib.Tabulate(ns))
}

func (l *Loop) clearRuntimeStats(c cli.Commander, w cli.Writer, s *cli.Scanner) {
	for i := range l.activePollers {
		a := &l.activePollers[i]
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
