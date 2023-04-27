package simplerelay

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	var ConfigFile string
	var EnableEpoll bool
	flag.StringVar(&ConfigFile, "config", "", "Config file")
	flag.BoolVar(&EnableEpoll, "epoll", false, "Enable epoll to manage connections")
	flag.Parse()

	if ConfigFile == "" {
		log.Fatal("no config file")
	}
	if EnableEpoll && runtime.GOOS != "linux" {
		log.Fatal("cannot enable epoll, which is used in linux only")
	}
	var err error
	var ep *epollRoutine
	if EnableEpoll {
		ep, err = NewEpollRoutine()
		if err != nil {
			log.Fatal(err)
		}
	}
	relayGroup := []*Relay{}
	relays := ReadConfig(ConfigFile)
	for _, relay := range relays {
		r := NewRelay(relay)
		if EnableEpoll {
			r.EnableEpoll(func(c net.Conn) {
				ep.Open(c)
			}, func(c net.Conn) {
				ep.Close(c)
			})
		}
		relayGroup = append(relayGroup, r)
	}
	defer func() {
		if EnableEpoll {
			ep.Close()
		}
		for _, r := range relayGroup {
			r.Close()
		}
	}()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
