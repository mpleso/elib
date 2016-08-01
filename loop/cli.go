package loop

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cli"
	"github.com/platinasystems/elib/elog"
	"github.com/platinasystems/elib/iomux"

	"fmt"
	"sort"
)

type LoopCli struct {
	cli.Main
	Node
}

func (l *Loop) CliAdd(c *cli.Command) { l.cli.AddCommand(c) }

func init() {
	AddInit(func(l *Loop) {
		l.RegisterEventPoller(iomux.Default)
		l.RegisterNode(&l.cli, "loop-cli")
	})
}

type fileEvent struct {
	loop *Loop
	*cli.File
}

func (l *Loop) rxReady(f *cli.File) {
	l.AddEvent(&fileEvent{loop: l, File: f}, &l.cli)
}

func (c *fileEvent) EventAction() {
	if err := c.RxReady(); err == cli.ErrQuit {
		c.loop.AddEvent(ErrQuit, nil)
	}
}

func (c *fileEvent) String() string { return "rx-ready " + c.File.String() }

func (c *LoopCli) EventHandler() {}

func (c *LoopCli) LoopInit(l *Loop) {
	c.Main.RxReady = l.rxReady
	c.Main.Prompt = "cli# "
	c.Main.AddStdin()
	c.Main.Start()
}

func (c *LoopCli) LoopExit(l *Loop) {
	c.Main.End()
}

func (l *Loop) Logf(format string, args ...interface{})   { fmt.Fprintf(&l.cli.Main, format, args...) }
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

func (l *Loop) showRuntimeStats(c cli.Commander, w cli.Writer, in *cli.Input) (err error) {
	ns := rtNodes{}
	for i := range l.DataNodes {
		n := l.DataNodes[i].GetNode()
		var s [2]stats
		l.activePollerPool.Foreach(func(a *activePoller) {
			if a.activeNodes != nil {
				s[0].add(&a.activeNodes[i].inputStats)
				s[1].add(&a.activeNodes[i].outputStats)
			}
		})
		name := n.name
		for j := range s {
			io := " input"
			if j == 1 {
				io = " output"
			}
			if s[j].calls > 0 {
				ns = append(ns, rtNode{
					Name:    name + io,
					Calls:   s[j].calls,
					Vectors: s[j].vectors,
					Clocks:  s[j].clocksPerVector(),
				})
			}
		}
	}

	// Summary
	{
		var s stats
		l.activePollerPool.Foreach(func(a *activePoller) {
			s.add(&a.pollerStats)
		})
		if s.calls > 0 {
			vecsPerSec := float64(s.vectors) / l.Seconds(s.clocks)
			clocksPerVec := float64(s.clocks) / float64(s.vectors)
			vecsPerCall := float64(s.vectors) / float64(s.calls)
			fmt.Fprintf(w, "Vectors: %d, Vectors/sec: %.2e, Clocks/vector: %.2f, Vectors/call %.2f\n",
				s.vectors, vecsPerSec, clocksPerVec, vecsPerCall)
		}
	}

	sort.Sort(ns)
	elib.TabulateWrite(w, ns)
	return
}

func (l *Loop) clearRuntimeStats(c cli.Commander, w cli.Writer, in *cli.Input) (err error) {
	l.activePollerPool.Foreach(func(a *activePoller) {
		a.pollerStats.clear()
		for j := range a.activeNodes {
			a.activeNodes[j].inputStats.clear()
			a.activeNodes[j].outputStats.clear()
		}
	})
	return
}

func (l *Loop) showEventLog(c cli.Commander, w cli.Writer, in *cli.Input) (err error) {
	v := elog.NewView()
	v.Print(w)
	return
}

func (l *Loop) clearEventLog(c cli.Commander, w cli.Writer, in *cli.Input) (err error) {
	elog.Clear()
	return
}

func init() {
	AddInit(func(l *Loop) {
		l.cli.AddCommand(&cli.Command{
			Name:      "show runtime",
			ShortHelp: "show main loop runtime statistics",
			Action:    l.showRuntimeStats,
		})
		l.cli.AddCommand(&cli.Command{
			Name:      "clear runtime",
			ShortHelp: "clear main loop runtime statistics",
			Action:    l.clearRuntimeStats,
		})
		l.cli.AddCommand(&cli.Command{
			Name:      "show event-log",
			ShortHelp: "show events in event log",
			Action:    l.showEventLog,
		})
		l.cli.AddCommand(&cli.Command{
			Name:      "clear event-log",
			ShortHelp: "clear events in event log",
			Action:    l.clearEventLog,
		})
	})
}
