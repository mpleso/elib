package iomux

import (
	"sync"
)

type Mux struct {
	// Poll/epoll file descriptor.
	fd       int
	once     sync.Once
	poolLock sync.Mutex // protects following
	pool     filePool
}

type File struct {
	Fd        int
	poolIndex uint
}

func (f *File) GetFile() *File { return f }
func (f *File) Index() uint    { return f.poolIndex }

type Interface interface {
	GetFile() *File
	// OS indicates that file is ready to read and/or write.
	ReadReady() error
	WriteReady() error
	ErrorReady() error
	// User has data available to write to file.
	WriteAvailable() bool
}

//go:generate gentemplate -d Package=iomux -id file -d Data=files -d Type=[]Interface github.com/platinasystems/elib/pool.tmpl

var DefaultMux = &Mux{}

func Add(f Interface)    { DefaultMux.Add(f) }
func Del(f Interface)    { DefaultMux.Del(f) }
func Update(f Interface) { DefaultMux.Update(f) }
func Wait(secs float64)  { DefaultMux.Wait(secs) }
