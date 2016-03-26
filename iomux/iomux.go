package iomux

import (
	"github.com/platinasystems/elib/event"

	"sync"
)

type Mux struct {
	// Poll/epoll file descriptor.
	fd       int
	once     sync.Once
	poolLock sync.Mutex // protects following
	filePool
}

type File struct {
	Fd        int
	poolIndex uint
}

func (f *File) GetFile() *File { return f }
func (f *File) Index() uint    { return f.poolIndex }

type Filer interface {
	GetFile() *File
	// OS indicates that file is ready to read and/or write.
	ReadReady() error
	WriteReady() error
	ErrorReady() error
	// User has data available to write to file.
	WriteAvailable() bool
}

//go:generate gentemplate -d Package=iomux -id file -d Data=files -d PoolType=filePool -d Type=Filer github.com/platinasystems/elib/pool.tmpl

var DefaultMux = &Mux{}

func Add(f Filer)                 { DefaultMux.Add(f) }
func Del(f Filer)                 { DefaultMux.Del(f) }
func Update(f Filer)              { DefaultMux.Update(f) }
func Wait()                       { DefaultMux.Wait() }
func EventWait(v *event.ActorVec) { DefaultMux.EventWait(v) }
