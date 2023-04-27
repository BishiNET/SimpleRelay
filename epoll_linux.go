//go:build linux

package main

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

const (
	EPOLLET = 0x80000000
)

var (
	ErrEpollCreate = fmt.Errorf("epoll created error")
	ErrEpoll       = fmt.Errorf("epoll error")
	ErrConnExists  = fmt.Errorf("connection existed")
	ErrNotSupport  = fmt.Errorf("connection doesn't support")
)

type EpollEvent struct {
	Events uint32
	Data   [8]byte // unaligned uintptr
}

// use syscallconn wouldn't duplicate the file descriptor
// as we memtioned, use File() will make fd block again.
func getFD(c net.Conn) int {
	var fd int
	var fs syscall.RawConn
	switch t := c.(type) {
	case *net.TCPConn:
		fs, _ = t.SyscallConn()
	case *net.UDPConn:
		fs, _ = t.SyscallConn()
	case *net.IPConn:
		fs, _ = t.SyscallConn()
	case *net.UnixConn:
		fs, _ = t.SyscallConn()
	default:
	}
	if fs != nil {
		fs.Control(func(_fd uintptr) {
			fd = int(_fd)
		})
	}
	return fd

}

func EpollCtl(epfd, op, fd int, event *EpollEvent) (errno syscall.Errno) {
	_, _, e := syscall.Syscall6(
		syscall.SYS_EPOLL_CTL,
		uintptr(epfd),
		uintptr(op),
		uintptr(fd),
		uintptr(unsafe.Pointer(event)),
		0, 0)
	return e
}

func EpollWait(epfd int, events []EpollEvent, maxev, waitms int) (int, syscall.Errno) {
	var ev unsafe.Pointer
	var _zero uintptr
	if len(events) > 0 {
		ev = unsafe.Pointer(&events[0])
	} else {
		ev = unsafe.Pointer(&_zero)
	}
	r1, _, e := syscall.Syscall6(
		syscall.SYS_EPOLL_PWAIT,
		uintptr(epfd),
		uintptr(ev),
		uintptr(maxev),
		uintptr(waitms),
		0, 0)
	return int(r1), e
}

type epollRoutine struct {
	epollfd int
	stopped context.Context
	stop    context.CancelFunc
}

type hijack struct {
	Conn net.Conn
}

func NewEpollRoutine() (Epoll, error) {
	epfd, errno := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if errno != nil {
		return nil, ErrEpollCreate
	}
	e := &epollRoutine{
		epollfd: epfd,
	}
	e.stopped, e.stop = context.WithCancel(context.Background())
	go e.Run()
	return e, nil
}

func (e *epollRoutine) Open(c net.Conn) error {
	fd := getFD(c)
	if fd == 0 {
		return ErrNotSupport
	}
	var ev EpollEvent
	ev.Events = syscall.EPOLLRDHUP | EPOLLET
	*(**hijack)(unsafe.Pointer(&ev.Data)) = &hijack{c}
	if err := EpollCtl(e.epollfd, syscall.EPOLL_CTL_ADD, fd, &ev); err != 0 {
		return ErrEpoll
	}
	return nil
}
func (e *epollRoutine) Close(c net.Conn) {
	fd := getFD(c)
	if fd == 0 {
		return
	}
	EpollCtl(e.epollfd, syscall.EPOLL_CTL_DEL, fd, nil)
}

func (e *epollRoutine) Shutdown() {
	e.stop()
	syscall.Close(e.epollfd)
}

func (e *epollRoutine) Run() {
	var events [128]EpollEvent
retry:
	for {
		n, err := EpollWait(e.epollfd, events[:], 128, -1)
		if err != 0 {
			if err == syscall.EINTR {
				continue retry
			}
			select {
			case <-e.stopped.Done():
				return
			default:
				continue retry
			}
		}
		for i := 0; i < n; i++ {
			if events[i].Events&(syscall.EPOLLERR|syscall.EPOLLRDHUP|syscall.EPOLLHUP) != 0 {
				ev := events[i]
				c := *(**hijack)(unsafe.Pointer(&ev.Data))
				// unblock all Read/Write call now and report EOF
				c.Conn.Close()
			}
		}
	}
}

