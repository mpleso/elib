package cli

import (
	"errors"
	"fmt"
	"sort"
)

var builtins []Command

func addBuiltin(c Command) { builtins = append(builtins, c) }

type quitCmd struct{}

var ErrQuit = errors.New("Quit")

func (c *quitCmd) Name() string                         { return "quit" }
func (c *quitCmd) Action(w Writer, args []string) error { return ErrQuit }
func init()                                             { addBuiltin(&quitCmd{}) }

type cmds []Command

func (c cmds) Len() int           { return len(c) }
func (c cmds) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c cmds) Less(i, j int) bool { return c[i].Name() < c[j].Name() }

type helpCmd struct{ cmds }

func (c *helpCmd) Name() string { return "help" }
func (c *helpCmd) LoopStart(m *Main) {
	c.cmds = nil
	for _, cmd := range m.allCmds {
		c.cmds = append(c.cmds, cmd)
	}
	sort.Sort(c.cmds)
}
func (c *helpCmd) Action(w Writer, args []string) (err error) {
	for _, c := range c.cmds {
		fmt.Fprintf(w, "%v\n", c.Name())
	}
	return
}
func init() { addBuiltin(&helpCmd{}) }
