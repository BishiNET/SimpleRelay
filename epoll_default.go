//go:build !linux

package simplerelay

func NewEpollRoutine() (Epoll, error) {
	return nil, nil
}

