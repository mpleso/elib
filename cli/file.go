package cli

import (
	"github.com/platinasystems/elib/iomux"
	"github.com/platinasystems/elib/loop"

	"fmt"
	"strings"
	"syscall"
)

func (c *File) ReadReady() (err error) {
	err = c.FileReadWriteCloser.ReadReady()
	if l := len(c.Read(0)); err == nil && l > 0 {
		switch {
		case c.main.RxReady != nil:
			c.main.RxReady <- c.poolIndex
		default:
			loop.AddEvent(c, c.main)
		}
	}
	return
}

func (c *File) EventAction() {
	if err := c.rxReady(); err == ErrQuit {
		loop.AddEvent(loop.ErrQuit, nil)
	}
}

func (m *Main) EventHandler() {}

func (c *File) writePrompt() {
	if l := len(c.main.Prompt); l > 0 {
		c.Write([]byte(c.main.Prompt))
	}
}

func (c *File) rxReady() (err error) {
	b := c.Read(0)
	nl := strings.Index(string(b), "\n")
	if nl == -1 {
		return
	}
	end := nl
	if end > 0 && b[end-1] == '\r' {
		end--
	}
	if end > 0 {
		err = c.main.Exec(c, strings.NewReader(string(b[:end])))
		if err != nil {
			fmt.Fprintf(c, "%s\n", err)
		}
		if err == ErrQuit {
			// Quit is only quit from stdin; otherwise just close file.
			if !c.isStdin() {
				c.Close()
				err = nil
			}
			return
		}
		// The only error we bubble to callers is ErrQuit
		err = nil
	}
	c.writePrompt()
	// Advance read buffer.
	c.Read(nl + 1)
	return
}

func (c *Main) AddFile(f iomux.FileReadWriteCloser) {
	i := c.FilePool.GetIndex()
	x := &c.Files[i]
	*x = File{
		main:                c,
		FileReadWriteCloser: f,
		poolIndex:           fileIndex(i),
	}
	iomux.Add(x)
	x.writePrompt()
}

func (c *Main) AddStdin() {
	c.AddFile(iomux.NewFileBuf(syscall.Stdin))
}

func (f *File) isStdin() bool {
	if f, ok := f.FileReadWriteCloser.(*iomux.FileBuf); ok {
		return f.Fd == syscall.Stdin
	}
	return false
}

func (m *Main) Write(p []byte) (n int, err error) {
	for i := range m.FilePool.Files {
		if !m.FilePool.IsFree(uint(i)) {
			n, err = m.FilePool.Files[i].Write(p)
			return
		}
	}
	return
}

func (m *Main) Start() {
	for _, c := range builtins {
		m.AddCommand(c)
	}

	for _, cmd := range m.allCmds {
		if l, ok := cmd.(LoopStarter); ok {
			l.CliLoopStart(m)
		}
	}
}

func (c *Main) End() {
	// Restore Stdin to blocking on exit.
	for i := range c.Files {
		if !c.FilePool.IsFree(uint(i)) && c.Files[i].isStdin() {
			syscall.SetNonblock(syscall.Stdin, false)
		}
	}
}

func (c *Main) Loop() {
	if c.RxReady == nil {
		c.RxReady = make(chan fileIndex)
	}
	c.Start()
	defer c.End()
	for {
		i := <-c.RxReady
		if err := c.Files[i].rxReady(); err == ErrQuit {
			break
		}
	}
}
