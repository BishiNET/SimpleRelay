//go:build !linux && !netbsd && !freebsd && !aix

package main

import (
	"time"
)

func SetTCPIDLE(fd int, idle time.Duration) {
}

func SetTCPTimedOut(fd int, timeout time.Duration) {
}
