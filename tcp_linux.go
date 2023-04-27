//go:build aix || freebsd || linux || netbsd

package main

import (
	"time"

	"golang.org/x/sys/unix"
)

func SetTCPIDLE(fd int, idle time.Duration) {
	unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_KEEPIDLE, int(idle.Seconds()))
}

// why we don't use Golang SetReadDeadline
// because net.TCPConn ReadFrom will splice, it will be ignored.
func SetTCPTimedOut(fd int, timeout time.Duration) {
	unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT, int(timeout.Milliseconds()))
}
