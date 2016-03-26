package cli

import (
	"errors"
	"fmt"
	"sort"
)

var builtins []Commander

func addBuiltin(c Commander) { builtins = append(builtins, c) }

type quitCmd struct{}

var ErrQuit = errors.New("Quit")

func (c *quitCmd) CliName() string                         { return "quit" }
func (c *quitCmd) CliAction(w Writer, args []string) error { return ErrQuit }
func init()                                                { addBuiltin(&quitCmd{}) }

type cmd struct {
	name    string
	command Commander
}
type cmds []cmd

func (c cmds) Len() int           { return len(c) }
func (c cmds) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c cmds) Less(i, j int) bool { return c[i].name < c[j].name }

type helpCmd struct{ cmds }

func (c *helpCmd) CliName() string { return "help,?" }
func (c *helpCmd) CliLoopStart(m *Main) {
	c.cmds = nil
	for k, v := range m.allCmds {
		c.cmds = append(c.cmds, cmd{name: k, command: v})
	}
	sort.Sort(c.cmds)
}
func (c *helpCmd) CliAction(w Writer, args []string) (err error) {
	for _, c := range c.cmds {
		help := ""
		if h, ok := c.command.(ShortHelper); ok {
			help = h.CliShortHelp()
		} else if h, ok := c.command.(Helper); ok {
			help = h.CliHelp()
		}
		if len(help) > 0 {
			fmt.Fprintf(w, "%-25s%s\n", c.name, help)
		} else {
			fmt.Fprintf(w, "%s\n", c.name)
		}
	}
	return
}
func init() { addBuiltin(&helpCmd{}) }
