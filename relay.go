package simplerelay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

const ENABLE_LOG = false

var (
	logger  = log.New(os.Stderr, "", log.Lshortfile|log.LstdFlags)
	errPool = sync.Pool{
		New: func() any {
			return new(strings.Builder)
		},
	}
)

type Relay struct {
	r          *SingleRelay
	listener   []net.Listener
	dialer     *net.Dialer
	is         context.Context
	done       context.CancelFunc
	preHook    func(c net.Conn)
	isEpoll    bool
	epollOpen  func(c net.Conn)
	epollClose func(c net.Conn)
}

func muteErr(err error) error {
	if err == nil {
		return err
	}
	if ENABLE_LOG {
		return err
	} else {
		switch {
		case errors.Is(err, io.EOF):
		case errors.Is(err, syscall.EPIPE):
		case errors.Is(err, os.ErrDeadlineExceeded):
		case errors.Is(err, syscall.ECONNRESET):
		case errors.Is(err, net.ErrClosed):
		default:
			return err
		}
		return nil
	}
}

func printErr(err error) {
	if err = muteErr(err); err != nil {
		logger.Output(2, err.Error())
	}
}

func connToString(c net.Conn) string {
	return c.LocalAddr().Network() + " -> " + c.RemoteAddr().String()
}

func relayErr(local, remote net.Conn, err ...error) {
	if len(err) == 0 {
		return
	}
	allErr := errPool.Get().(*strings.Builder)
	defer errPool.Put(allErr)
	allErr.Reset()

	if errs := muteErr(err[0]); errs != nil {
		allErr.WriteString(connToString(local) + ":" + errs.Error())
		allErr.WriteString("\n")
	}

	if errs := muteErr(err[1]); errs != nil {
		allErr.WriteString(connToString(remote) + ":" + errs.Error())
		allErr.WriteString("\n")
	}

	if allErr.Len() == 0 {
		return
	}

	logger.Output(2, allErr.String())

}

func NewRelay(r *SingleRelay) *Relay {
	ry := &Relay{
		r: r,
		dialer: &net.Dialer{
			Timeout:   r.DialTimeout,
			KeepAlive: r.Keepalive,
		},
	}
	localGroups := r.Locals()
	remoteGroups := r.Remotes()
	if len(localGroups) != len(remoteGroups) {
		log.Fatalf("Relay: %s Local groups doesn't match with the remote peer groups.", r.Remote)
	}
	ry.is, ry.done = context.WithCancel(context.Background())
	ry.preHook = func(c net.Conn) {
		fs, _ := c.(*net.TCPConn).SyscallConn()
		fs.Control(func(fd uintptr) {
			if ry.r.KeepIDLE > 0 {
				SetTCPIDLE(int(fd), ry.r.KeepIDLE)
			}
			if ry.r.Timeout > 0 {
				SetTCPTimedOut(int(fd), ry.r.Timeout)
			}
		})
	}
	for i, singleLocal := range localGroups {
		if len(singleLocal) != len(remoteGroups[i]) {
			log.Fatalf("Relay: Local: %v doesn't match with the remote peer: %v.", singleLocal, remoteGroups[i])
		}
		for j, local := range singleLocal {
			l, err := net.Listen("tcp", local)
			if err != nil {
				log.Fatalf("Relay: Listen Fail: %s %v", local, err)
			}
			ry.listener = append(ry.listener, l)
			go ry.Run(l, remoteGroups[i][j])
		}
	}
	return ry
}

func (r *Relay) EnableEpoll(open, close func(c net.Conn)) {
	r.isEpoll = true
	r.epollOpen = open
	r.epollClose = close
}

func (r *Relay) HandleTCPConn(local net.Conn, remote string) {
	defer local.Close()
	var err error
	var err1 error

	peer, err := r.dialer.Dial("tcp", remote)
	if err != nil {
		printErr(err)
		return
	}
	defer peer.Close()
	r.preHook(local)
	r.preHook(peer)
	if r.isEpoll {
		r.epollOpen(local)
		r.epollOpen(peer)
		defer r.epollClose(local)
		defer r.epollClose(peer)
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err1 = io.Copy(peer, local)
		peer.SetReadDeadline(time.Now())
	}()
	_, err = io.Copy(local, peer)
	local.SetReadDeadline(time.Now())
	wg.Wait()

	if err != nil || err1 != nil {
		relayErr(local, peer, err, err1)
	}
}

func (r *Relay) Close() {
	r.done()
	for _, l := range r.listener {
		l.Close()
	}
}
func (r *Relay) Run(l net.Listener, remote string) {
	logger.Output(2, fmt.Sprintf("Relay: %s <-> %s", l.Addr().String(), remote))
retry:
	for {
		c, err := l.Accept()
		if err != nil {
			select {
			case <-r.is.Done():
				return
			default:
				printErr(err)
				continue retry
			}
		}
		go r.HandleTCPConn(c, remote)
	}
}
