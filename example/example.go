package main

import (
	"syscall"
)

func main() {
	//println(4 & 2)
	//println(1 & 0)
	println(1024 << 1)
	//println(0 & syscall.EV_ADD)
	//println(getEvents(0, syscall.EV_ADD, 0))
}

func getEvents(old uint32, new uint32, fd int) (ret []syscall.Kevent_t) {
	if new&syscall.EV_ADD != 0 {
		if old&syscall.EV_ADD == 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_ADD | syscall.EV_ENABLE, Filter: syscall.EVFILT_READ})
			println("========1")
		}
	} else {
		if old&syscall.EV_ADD != 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_DELETE | syscall.EV_ONESHOT, Filter: syscall.EVFILT_READ})
			println("========2")
		}
	}

	if new&syscall.EV_DELETE != 0 {
		if old&syscall.EV_DELETE == 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_ADD | syscall.EV_ENABLE, Filter: syscall.EVFILT_WRITE})
			println("========3")
		}
	} else {
		if old&syscall.EV_DELETE != 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_DELETE | syscall.EV_ONESHOT, Filter: syscall.EVFILT_WRITE})
			println("========4")
		}
	}
	return
}
