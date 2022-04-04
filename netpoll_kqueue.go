//go:build darwin || dragonfly || freebsd || netbsd || openbsd
// +build darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"errors"
	"log"
	"sync"
	"syscall"
	"time"
)

const (
	EventRead  Event = syscall.EV_ADD
	EventWrite Event = syscall.EV_DELETE
	EventErr   Event = syscall.EV_DISPATCH
	EventNone  Event = 0
)

// Kqueue 数据结构
type Kqueue struct {
	fd      int               // 描述符
	mu      sync.RWMutex      // 读写锁
	handle  map[int]Event     // 事件
	done    chan struct{}     // 是否完成
	closed  bool              // 关闭状态
	timeout *syscall.Timespec // 超时时间
	//pool    *sync.Pool        // 池化
}

// KqueueCreate 创建一个kqueue
func KqueueCreate() (*Kqueue, error) {
	fd, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	// 初始化事件状态
	//_, err = syscall.Kevent(fd, []syscall.Kevent_t{{
	//	Ident:  0,
	//	Filter: syscall.EVFILT_USER,
	//	Fflags: syscall.EV_ADD | syscall.EV_CLEAR,
	//}}, nil, nil)
	//if err != nil {
	//	return nil, err
	//}
	return &Kqueue{
		fd: fd,
		timeout: &syscall.Timespec{
			Sec:  10,
			Nsec: 0,
		},
	}, nil
}

// Wake 唤醒当前的kq
func (k *Kqueue) Wake() error {
	_, err := syscall.Kevent(k.fd, []syscall.Kevent_t{{
		Ident:  0,
		Filter: syscall.EVFILT_USER,
		Fflags: syscall.NOTE_TRIGGER,
	}}, nil, k.timeout)
	return err
}

// SetTimeout 设置处理超时时间
func (k *Kqueue) SetTimeout(d time.Duration) error {
	if d < time.Millisecond {
		return errors.New("时间区间错误")
	}
	k.timeout.Sec = int64(d / time.Second)
	k.timeout.Nsec = int64(d % time.Second)
	return nil
}

// RegisterRead 注册一个读事件
func (k *Kqueue) RegisterRead(fd int) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return errors.New("已经关闭")
	}
	// 处理事件
	_, err := syscall.Kevent(k.fd, k.getEvents(fd, EventNone, EventRead), nil, k.timeout)
	if err != nil {
		k.handle[fd] = EventRead
	}
	return err
}

// EnableReadWrite 开启读写事件
func (k *Kqueue) EnableReadWrite(fd int) error {
	return k.mod(fd, EventRead|EventWrite)
}

// EnableRead 开启读事件
func (k *Kqueue) EnableRead(fd int) error {
	return k.mod(fd, EventRead)
}

// mod 修改事件
func (k *Kqueue) mod(fd int, new Event) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return errors.New("已经关闭")
	}

	if _, has := k.handle[fd]; !has {
		return errors.New("fd 没有注册")
	}
	_, err := syscall.Kevent(k.fd, k.getEvents(fd, k.handle[fd], new), nil, k.timeout)
	if err != nil {
		k.handle[fd] = new
	}
	return err
}

// Delete 取消注册
func (k *Kqueue) Delete(fd int) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return errors.New("已经关闭")
	}
	if _, has := k.handle[fd]; !has {
		return errors.New("fd 没有注册")
	}
	_, err := syscall.Kevent(k.fd, k.getEvents(fd, k.handle[fd], EventNone), nil, k.timeout)
	if err != nil {
		delete(k.handle, fd)
	}
	return err
}

// Wait 等待事件
func (k *Kqueue) Wait(handle HandleEvent) {
	defer func() {
		close(k.done)
		err := k.Close()
		if err != nil {
			return
		}
	}()

	events := make([]syscall.Kevent_t, maxWaitEventsBegin)
	for {
		n, err := syscall.Kevent(k.fd, nil, events, k.timeout)
		if err != nil && err != syscall.EINTR {
			log.Println("kqueueWait: " + err.Error())
			continue
		}
		k.mu.RLock()

		for i := 0; i < n; i++ {
			event := events[i]
			fd := int(event.Ident)
			if fd == 0 {
				k.mu.RUnlock()
				continue
			}
			var e Event
			if events[i].Filter == syscall.EVFILT_READ && events[i].Flags&syscall.EV_ENABLE != 0 {
				e |= EventRead
			}
			if events[i].Filter == syscall.EVFILT_WRITE && events[i].Flags&syscall.EV_ENABLE != 0 {
				e |= EventWrite
			}
			if (event.Flags&syscall.EV_ERROR != 0) || (events[i].Flags&syscall.EV_EOF != 0) {
				e |= EventErr
			}
			handle(fd, e)
		}
		k.mu.RUnlock()
		if n == len(events) {
			events = make([]syscall.Kevent_t, maxWaitEventsBegin<<1)
		}
	}
}

// Close 关闭
func (k *Kqueue) Close() (err error) {
	if k.closed {
		return errors.New("已经关闭")
	}
	<-k.done
	return syscall.Close(k.fd)
}

// getEvents 获取时间需要修改的状态
func (k *Kqueue) getEvents(fd int, old Event, new Event) (ret []syscall.Kevent_t) {
	if new&syscall.EV_ADD == 0 {
		if old&syscall.EV_ADD != 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_DELETE | syscall.EV_ONESHOT, Filter: syscall.EVFILT_READ})

		}
	} else {
		if old&syscall.EV_ADD == 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_ADD | syscall.EV_ENABLE, Filter: syscall.EVFILT_READ})

		}
	}

	if new&syscall.EV_DELETE == 0 {
		if old&syscall.EV_DELETE != 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_DELETE | syscall.EV_ONESHOT, Filter: syscall.EVFILT_WRITE})
		}
	} else {
		if old&syscall.EV_DELETE == 0 {
			ret = append(ret, syscall.Kevent_t{Ident: uint64(fd), Flags: syscall.EV_ADD | syscall.EV_ENABLE, Filter: syscall.EVFILT_WRITE})
		}
	}
	return
}
