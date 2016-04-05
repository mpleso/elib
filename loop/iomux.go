package loop

import (
	"github.com/platinasystems/elib/event"
	"github.com/platinasystems/elib/iomux"
)

type Iomux struct{ Node }

func (p *Iomux) EventPoll(v *event.ActorVec) { iomux.EventWait(v) }
func (p *Iomux) EventHandler() EventHandler  { return p }
