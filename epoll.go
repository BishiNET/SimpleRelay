package main

import "net"

type Epoll interface {
	Open(c net.Conn) error
	Close(c net.Conn)
	Shutdown()
}
