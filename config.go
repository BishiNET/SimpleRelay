package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DEFAULT_TIMEOUT   = 5 * time.Second
	DEFAULT_KEEPALIVE = 30 * time.Second
)

type SingleRelay struct {
	Local       string        `json:"local"`
	Remote      string        `json:"remote"`
	DialTimeout time.Duration `json:"dialtimeout"`
	Timeout     time.Duration `json:"timeout"`
	Keepalive   time.Duration `json:"keepalive"`
	KeepIDLE    time.Duration `json:"keepidle"`
}
type Config []*SingleRelay

func multi(needle string) [][]string {
	// like 1.1.1.1:80, 127.0.0.1:90
	mt := strings.Split(strings.ReplaceAll(needle, " ", ""), ",")
	locals := make([][]string, len(mt))
	for index, m := range mt {
		// like 1.1.1.1:1000-2000
		rg := strings.Split(m, "-")
		locals[index] = append(locals[index], rg[0])
		if len(rg) > 1 {
			host, from, _ := net.SplitHostPort(rg[0])
			to := rg[1]
			fromPort, _ := strconv.Atoi(from)
			toPort, _ := strconv.Atoi(to)
			for port := fromPort + 1; port <= toPort; port++ {
				locals[index] = append(locals[index], fmt.Sprintf("%s:%d", host, port))
			}
		}
	}
	return locals
}
func (s *SingleRelay) Locals() [][]string {
	return multi(s.Local)
}

func (s *SingleRelay) Remotes() [][]string {
	return multi(s.Remote)
}

func ReadConfig(f string) Config {
	b, err := os.ReadFile(f)
	if err != nil {
		log.Fatal("Cannot Read Config: ", err)
	}
	var c Config
	json.Unmarshal(b, &c)
	if len(c) == 0 {
		log.Fatal("no relay")
	}
	for _, r := range c {
		if r.Local == "" {
			log.Fatal("relay config error: no local listener")
		}
		if r.Remote == "" {
			log.Fatal("relay config error: no remote peer")
		}
		if r.Keepalive == 0 {
			// we don't use golang's default keepalive,
			// because it's too short.
			r.Keepalive = DEFAULT_KEEPALIVE
		} else {
			r.Keepalive *= time.Second
		}
		if r.DialTimeout == 0 {
			r.DialTimeout = DEFAULT_TIMEOUT
		} else {
			r.DialTimeout *= time.Second
		}
		if r.Timeout != 0 {
			r.Timeout *= time.Second
		}
		if r.KeepIDLE != 0 {
			r.KeepIDLE *= time.Second
		}
	}
	return c
}
