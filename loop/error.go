package loop

import (
	"github.com/platinasystems/elib"
	"github.com/platinasystems/elib/cli"

	"fmt"
	"sort"
)

type ErrorRef uint32

type BufferError struct {
	nodeIndex  uint32
	errorIndex uint16
}

type errorThread struct {
	counts          elib.Uint64Vec
	countsLastClear elib.Uint64Vec
	cache           ErrorRef
}

//go:generate gentemplate -d Package=loop -id errorThread -d VecType=errorThreadVec -d Type=*errorThread github.com/platinasystems/elib/vec.tmpl
//go:generate gentemplate -d Package=loop -id error -d VecType=errVec -d Type=err github.com/platinasystems/elib/vec.tmpl

type errorNode struct {
	Node
	threads errorThreadVec
	errs    errVec
}

func (n *errorNode) getThread(id uint) (t *errorThread) {
	n.threads.Validate(uint(id))
	if t = n.threads[id]; t == nil {
		t = &errorThread{}
		n.threads[id] = t
	}
	i := n.errs.Len()
	if i > 0 {
		t.counts.Validate(i - 1)
		t.countsLastClear.Validate(i - 1)
	}
	return
}

func (n *errorNode) MakeLoopIn() LooperIn { return &RefIn{} }

var ErrorNode = &errorNode{}

func init() {
	AddInit(func(l *Loop) {
		l.RegisterNode(ErrorNode, "error")
	})
}

func (en *errorNode) LoopOutput(l *Loop, in LooperIn) {
	ri := in.(*RefIn)
	ts := en.getThread(ri.ThreadId())

	cache := ts.cache
	cacheCount := uint64(0)
	i, n := uint(0), ri.Len()
	for i+2 <= n {
		cacheCount += 2
		i += 2
		if e0, e1 := ri.Refs[i-2].Err, ri.Refs[i-1].Err; e0 != cache || e1 != cache {
			cacheCount -= 2
			ts.counts[e0] += 1
			ts.counts[e1] += 1
			if e0 == e1 {
				ts.counts[cache] += cacheCount
				cache, cacheCount = e0, 0
			}
		}
	}

	for i < n {
		ts.counts[ri.Refs[i+0].Err] += 1
		i++
	}

	ts.counts[cache] += cacheCount
	ts.cache = cache
	ri.pool.FreeRefs(ri.Refs[:n])
}

type err struct {
	nodeIndex uint32
	str       string
}

func (n *Node) NewError(s string) (r ErrorRef) {
	e := err{nodeIndex: uint32(n.index), str: s}
	en := ErrorNode
	r = ErrorRef(len(en.errs))
	en.errs = append(en.errs, e)
	return
}

type errNode struct {
	Node  string `format:"%-30s"`
	Error string
	Count uint64 `format:"%16d"`
}
type errNodes []errNode

func (ns errNodes) Less(i, j int) bool {
	if ns[i].Node == ns[j].Node {
		return ns[i].Error < ns[j].Error
	}
	return ns[i].Node < ns[j].Node
}
func (ns errNodes) Swap(i, j int) { ns[i], ns[j] = ns[j], ns[i] }
func (ns errNodes) Len() int      { return len(ns) }

func (l *Loop) showErrors(c cli.Commander, w cli.Writer, s *cli.Scanner) (err error) {
	en := ErrorNode
	ns := []errNode{}
	for i := range en.errs {
		e := &en.errs[i]
		c := uint64(0)
		for _, t := range en.threads {
			if t != nil {
				c += t.counts[i]
				if i < len(t.countsLastClear) {
					c -= t.countsLastClear[i]
				}
			}
		}
		if c > 0 {
			n := l.dataNodes[e.nodeIndex].GetNode()
			ns = append(ns, errNode{
				Node:  n.name,
				Error: e.str,
				Count: c,
			})
		}
	}
	if len(ns) > 1 {
		sort.Sort(errNodes(ns))
	}
	if len(ns) > 0 {
		elib.TabulateWrite(w, ns)
	} else {
		fmt.Fprintln(w, "No errors since last clear.")
	}
	return
}

func (l *Loop) clearErrors(c cli.Commander, w cli.Writer, s *cli.Scanner) (err error) {
	for _, t := range ErrorNode.threads {
		if t != nil {
			copy(t.countsLastClear, t.counts)
		}
	}
	return
}

func init() {
	AddInit(func(l *Loop) {
		l.cli.AddCommand(&cli.Command{
			Name:      "show errors",
			ShortHelp: "show error counters",
			Action:    l.showErrors,
		})
		l.cli.AddCommand(&cli.Command{
			Name:      "clear errors",
			ShortHelp: "clear error counters",
			Action:    l.clearErrors,
		})
	})
}
