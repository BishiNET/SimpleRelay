//go:build !linux

package main

func NewEpollRoutine() (Epoll, error) {
	return nil, nil
}

