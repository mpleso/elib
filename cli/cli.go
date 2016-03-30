package cli

import (
	"github.com/platinasystems/elib/iomux"

	"fmt"
	"io"
	"strings"
	"sync"
)

type Writer interface {
	io.Writer
}

type Commander interface {
	CliName() string
	CliAction(w Writer, args []string) error
}

type Helper interface {
	CliHelp() string
}

type ShortHelper interface {
	CliShortHelp() string
}

type LoopStarter interface {
	CliLoopStart(m *Main)
}

type Action func(c Commander, w Writer, args []string)

type Command struct {
	// Command name separated by space; alias by commas.
	Name            string
	ShortHelp, Help string
	Action
}

func (c *Command) CliName() string                               { return c.Name }
func (c *Command) CliShortHelp() string                          { return c.ShortHelp }
func (c *Command) CliHelp() string                               { return c.Help }
func (c *Command) CliAction(w Writer, args []string) (err error) { c.Action(c, w, args); return }

type command struct {
	name  string
	names []string
}

type subCommand struct {
	name string
	cmds map[string]Commander
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
	allCmds map[string]Commander
	Prompt  string
	RxReady chan fileIndex
	FilePool
	servers  []*server
	initOnce sync.Once
}

func normalizeName(n string) string { return strings.ToLower(n) }

func (m *Main) AddCommand(C Commander) {
	ns := strings.Split(C.CliName(), ",")
	for i := range ns {
		m.addCommand(C, ns[i])
	}
}

func (m *Main) addCommand(C Commander, name string) {
	c := &command{name: name}

	if m.allCmds == nil {
		m.allCmds = make(map[string]Commander)
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
				sub.cmds = make(map[string]Commander)
			}
			sub.cmds[name] = C
		}
	}
}

func (sub *subCommand) uniqueCommand(matching string) (Commander, bool) {
	n := 0
	var c Commander
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

func (m *Main) lookup(args []string) (Commander, []string, error) {
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

	return nil, nil, fmt.Errorf("ambiguous: %s", strings.Join(args, " "))
}

func (m *Main) Exec(w Writer, args []string) error {
	c, a, err := m.lookup(args)
	if err == nil {
		err = c.CliAction(w, a)
	}
	return err
}

var Default = &Main{}

func AddCommand(c Commander)             { Default.AddCommand(c) }
func Add(name string, action Action)     { Default.AddCommand(&Command{Name: name, Action: action}) }
func Exec(w Writer, args []string) error { return Default.Exec(w, args) }
