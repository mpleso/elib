// Copyright 2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package loop

import (
	"github.com/platinasystems/go/elib"
	"github.com/platinasystems/go/elib/cli"
	"github.com/platinasystems/go/elib/elog"
	"github.com/platinasystems/go/elib/iomux"

	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type LoopCli struct {
	Node
	cli.Main
}

func (l *Loop) CliAdd(c *cli.Command) { l.Cli.AddCommand(c) }

type fileEvent struct {
	c *LoopCli
	*cli.File
}

func (c *LoopCli) rxReady(f *cli.File) {
	c.AddEvent(&fileEvent{c: c, File: f}, c)
}

func (c *fileEvent) EventAction() {
	if err := c.RxReady(); err == cli.ErrQuit {
		c.c.AddEvent(ErrQuit, nil)
	}
}

func (c *fileEvent) String() string { return "rx-ready " + c.File.String() }

func (c *LoopCli) EventHandler() {}

func (c *LoopCli) LoopInit(l *Loop) {
	if len(c.Main.Prompt) == 0 {
		c.Main.Prompt = "cli# "
	}
	c.Main.Start()
}

func (c *LoopCli) LoopExit(l *Loop) {
	c.Main.End()
}

func (l *Loop) Logf(format string, args ...interface{})   { fmt.Fprintf(&l.Cli.Main, format, args...) }
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
		s[0].add(&n.inputStats)
		s[1].add(&n.outputStats)
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
				if _, ok := n.noder.(inOutLooper); ok {
					io = ""
				} else {
					io = " output"
				}
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
			dt := time.Since(l.timeLastRuntimeClear).Seconds()
			vecsPerSec := float64(s.vectors) / dt
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
	l.timeLastRuntimeClear = time.Now()
	for i := range l.DataNodes {
		n := l.DataNodes[i].GetNode()
		n.inputStats.clear()
		n.outputStats.clear()
	}
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

func (l *Loop) exec(c cli.Commander, w cli.Writer, in *cli.Input) (err error) {
	var files []*os.File
	for !in.End() {
		var (
			pattern string
			names   []string
			f       *os.File
		)
		in.Parse("%s", &pattern)
		if names, err = filepath.Glob(pattern); err != nil {
			return
		}
		if len(names) == 0 {
			err = fmt.Errorf("no files matching pattern: `%s'", pattern)
			return
		}
		for _, name := range names {
			if f, err = os.OpenFile(name, os.O_RDONLY, 0); err != nil {
				return
			}
			files = append(files, f)
		}
	}
	for _, f := range files {
		var i [2]cli.Input
		i[0].Init(f)
		for !i[0].End() {
			i[1].Init(nil)
			if !i[0].Parse("%l", &i[1].Input) {
				err = i[0].Error()
				return
			}
			if err = l.Cli.ExecInput(w, &i[1]); err != nil {
				return
			}
		}
		f.Close()
	}
	return
}

func (l *Loop) comment(c cli.Commander, w cli.Writer, in *cli.Input) (err error) {
	in.Skip()
	return
}

func (l *Loop) cliInit() {
	l.RegisterEventPoller(iomux.Default)
	c := &l.Cli
	c.Main.RxReady = c.rxReady
	l.RegisterNode(c, "loop-cli")
	c.AddCommand(&cli.Command{
		Name:      "show runtime",
		ShortHelp: "show main loop runtime statistics",
		Action:    l.showRuntimeStats,
	})
	c.AddCommand(&cli.Command{
		Name:      "clear runtime",
		ShortHelp: "clear main loop runtime statistics",
		Action:    l.clearRuntimeStats,
	})
	c.AddCommand(&cli.Command{
		Name:      "show event-log",
		ShortHelp: "show events in event log",
		Action:    l.showEventLog,
	})
	c.AddCommand(&cli.Command{
		Name:      "clear event-log",
		ShortHelp: "clear events in event log",
		Action:    l.clearEventLog,
	})
	c.AddCommand(&cli.Command{
		Name:      "exec",
		ShortHelp: "execute cli commands from given file(s)",
		Action:    l.exec,
	})
	c.AddCommand(&cli.Command{
		Name:      "//",
		ShortHelp: "comment",
		Action:    l.comment,
	})
}
