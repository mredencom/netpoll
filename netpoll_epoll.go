//go:build linux
// +build linux

package netpoll

import (
	"errors"
	"log"
	"sync"
	"syscall"
)

// Epoll 数据结构
type Epoll struct {
	fd      int           // 描述符
	eventFd int           // 事件描述符
	buf     []byte        //
	mu      sync.RWMutex  // 读写锁
	done    chan struct{} // 是否完成
	closed  bool          // 关闭状态
	//pool    *sync.Pool        // 池化
}

// EpollCreate 创建一个epoll
func (e *Epoll) EpollCreate() (*Epoll, error) {
	// 创建epoll
	fd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	// 创建event
	r1, _, errno := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0, 0)
	if errno != 0 {
		syscall.Close(fd)
		return nil, errno
	}
	eventFd := int(r1)
	err = syscall.EpollCtl(fd, syscall.EPOLL_CTL_ADD, eventFd, &syscall.EpollEvent{
		Events: syscall.EPOLLIN | syscall.EPOLLPRI,
		Fd:     int32(eventFd),
	})
	if err != nil {
		_ = syscall.Close(fd)
		_ = syscall.Close(eventFd)
		return nil, err
	}
	return &Epoll{
		fd:      fd,
		eventFd: eventFd,
		buf:     make([]byte, 8),
		done:    make(chan struct{}),
	}, nil
}

// Close 关闭
func (e *Epoll) Close() (err error) {
	if e.closed {
		return errors.New("已经关闭")
	}
	<-e.done
	e.closed = true
	err = syscall.Close(e.fd)
	err = syscall.Close(e.eventFd)
	return
}

var wakeBytes = []byte{1, 0, 0, 0, 0, 0, 0, 0}

//Wake 唤醒
func (e *Epoll) Wake() error {
	_, err := syscall.Write(e.eventFd, wakeBytes)
	return err

}

//Read 读
func (e *Epoll) Read() error {
	n, err := syscall.Read(e.eventFd, e.buf)
	if err != nil || n != 8 {
		return err
	}
	return nil
}

//RegisterRead 注册一个读事件
func (e *Epoll) RegisterRead(fd int) error {
	return e.add(fd, syscall.EPOLLIN|syscall.EPOLLPRI)
}

//RegisterWrite 注册一个写事件
func (e *Epoll) RegisterWrite(fd int) error {
	return e.add(fd, syscall.EPOLLOUT)
}

// EnableReadWrite 开启读写事件
func (e *Epoll) EnableReadWrite(fd int) error {
	return e.mod(fd, syscall.EPOLLIN|syscall.EPOLLPRI|syscall.EPOLLOUT)
}

// EnableRead 开启读事件
func (e *Epoll) EnableRead(fd int) error {
	return e.mod(fd, syscall.EPOLLIN|syscall.EPOLLPRI)
}

// Delete 删除一个事件
func (e *Epoll) Delete(fd int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return syscall.EpollCtl(e.fd, syscall.EPOLL_CTL_DEL, fd, nil)
}

// add 添加一个时间
func (e *Epoll) add(fd int, events uint32) error {
	return syscall.EpollCtl(e.fd, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{
		Fd:     int32(fd),
		Events: events,
	})
}

// mod 修改一个时间
func (e *Epoll) mod(fd int, events uint32) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return syscall.EpollCtl(e.fd, syscall.EPOLL_CTL_MOD, fd, &syscall.EpollEvent{
		Fd:     int32(fd),
		Events: events,
	})
}

// Wait 等待事件
func (e *Epoll) Wait(handle HandleEvent) {
	defer func() {
		close(e.done)
		err := e.Close()
		if err != nil {
			return
		}
	}()

	events := make([]syscall.EpollEvent, maxWaitEventsBegin)
	var msec int
	for {
		n, err := syscall.EpollWait(e.fd, events, msec)
		//  EINTR：在任何请求的事件发生或超时到期之前，信号处理程序中断了该调用
		//  EINVAL：epfd不是epoll文件描述符，或者maxevents小于或等于零。
		if err != nil && err != syscall.EINTR && err != syscall.EINVAL {
			log.Println("epollWait: " + err.Error())
			continue
		}
		e.mu.RLock()
		for i := 0; i < n; i++ {
			event := events[i]
			fd := int(event.Fd)
			if fd == e.eventFd {
				e.mu.RUnlock()
				continue
			}
			var e Event
			if event.Events&(syscall.EPOLLIN|syscall.EPOLLPRI|syscall.EPOLLRDHUP) != 0 {
				e |= syscall.EPOLLIN | syscall.EPOLLPRI
			}
			if (event.Events&syscall.EPOLLERR != 0) || (event.Events&syscall.EPOLLOUT != 0) {
				e |= syscall.EPOLLOUT
			}
			if (event.Events&syscall.EPOLLHUP != 0) && (event.Events&syscall.EPOLLIN == 0) {
				e |= syscall.EPOLLRDBAND
			}
			handle(fd, e)
		}
		e.mu.RUnlock()
		if n == len(events) {
			events = make([]syscall.EpollEvent, maxWaitEventsBegin<<1)
		}
	}
}
