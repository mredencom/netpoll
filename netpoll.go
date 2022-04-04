package netpoll

import "syscall"

type Event uint32

type HandleEvent func(fd int, event Event)

const (
	EventRead  Event = syscall.EV_ADD
	EventWrite Event = syscall.EV_DELETE
	EventErr   Event = syscall.EV_DISPATCH
	EventNone  Event = 0
)
