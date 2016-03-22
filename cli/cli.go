package cli

import (
	"github.com/platinasystems/elib/iomux"

	"fmt"
	"io"
	"strings"
	"sync"
)

type Command interface {
	Name() string
	Action(w io.Writer, args []string) error
}

type Helper interface {
	Help() string
}

type LoopStarter interface {
	LoopStart(m *Main)
}

type command struct {
	name  string
	names []string
}

type subCommand struct {
	name string
	cmds map[string]Command
	subs map[string]*subCommand
}

func (c *subCommand) Elts() int { return len(c.cmds) + len(c.subs) }

type File struct {
	*Main
	poolIndex fileIndex
	iomux.FileReadWriteCloser
}

type fileIndex uint

//go:generate gentemplate -d Package=cli -id file -d Data=Files -d PoolType=FilePool -d Type=File github.com/platinasystems/elib/pool.tmpl

type Main struct {
	// Root of command tree.
	rootCmd subCommand
	allCmds map[string]Command
	Prompt  string
	RxReady chan fileIndex
	FilePool
	servers  []*server
	initOnce sync.Once
}

func normalizeName(n string) string { return strings.ToLower(n) }

func (m *Main) addCommand(C Command) {
	c := &command{}
	c.name = C.Name()

	if m.allCmds == nil {
		m.allCmds = make(map[string]Command)
	}
	m.allCmds[c.name] = C

	c.names = strings.Split(c.name, " ")
	n := len(c.names)
	if n == 0 {
		panic(fmt.Errorf("name only whitespace: `%s'", c.name))
	}
	sub := &m.rootCmd
	for i := 0; i < n; i++ {
		// Normalize to lower case.
		c.names[i] = normalizeName(c.names[i])
		name := c.names[i]

		if i+1 < n {
			if sub.subs == nil {
				sub.subs = make(map[string]*subCommand)
			}
			var (
				x  *subCommand
				ok bool
			)
			if x, ok = sub.subs[name]; !ok {
				x = &subCommand{}
				sub.subs[name] = x
			}
			sub = x
		} else {
			if sub.cmds == nil {
				sub.cmds = make(map[string]Command)
			}
			sub.cmds[name] = C
		}
	}
}

func (m *Main) AddCommand(c Command) {
	m.initOnce.Do(func() {
		for _, c := range builtins {
			m.addCommand(c)
		}
	})
	m.addCommand(c)
}

func (sub *subCommand) uniqueCommand(matching string) (Command, bool) {
	n := 0
	var c Command
	for k, v := range sub.cmds {
		if strings.Index(k, matching) == 0 {
			c = v
			n++
		}
	}
	ok := n == 1
	if !ok {
		c = nil
	}
	return c, ok
}

func (sub *subCommand) uniqueSubCommand(matching string) (*subCommand, bool) {
	n := 0
	var c *subCommand
	for k, v := range sub.subs {
		if strings.Index(k, matching) == 0 {
			c = v
			n++
		}
	}
	ok := n == 1
	if !ok {
		c = nil
	}
	return c, ok
}

func (m *Main) lookup(args []string) (Command, []string, error) {
	sub := &m.rootCmd
	n := len(args)
	for i := 0; i < n; i++ {
		name := normalizeName(args[i])

		// Exact match for sub-command.
		if x, ok := sub.subs[name]; ok {
			sub = x
			continue
		}

		// Unique match for sub-command.
		if x, ok := sub.uniqueSubCommand(name); ok {
			sub = x
			continue
		}

		// Exact match.
		if x, ok := sub.cmds[name]; ok {
			return x, args[i+1:], nil
		}

		// Unique match for command.
		if x, ok := sub.uniqueCommand(name); ok {
			return x, args[i+1:], nil
		}

		// Not found
		return nil, nil, fmt.Errorf("unknown: %s", name)
	}
	return nil, nil, nil
}

func (m *Main) Exec(w io.Writer, args []string) error {
	c, a, err := m.lookup(args)
	if err == nil {
		err = c.Action(w, a)
	}
	return err
}

var defaultMain = &Main{}

func AddCommand(c Command)                  { defaultMain.AddCommand(c) }
func Exec(w io.Writer, args []string) error { return defaultMain.Exec(w, args) }
