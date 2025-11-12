package main

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tidwall/redcon"
)

type Proxy struct {
	ctx        context.Context
	mux        sync.RWMutex
	mainClient *redis.Client
	cache      map[string]string
	errSig     chan bool
}

func NewProxy() *Proxy {
	return &Proxy{
		ctx:    context.Background(),
		cache:  make(map[string]string),
		errSig: make(chan bool),
	}
}

func (p *Proxy) ConnectToServer(address string) {
	if p.mainClient != nil {
		return
	}

	opts := &redis.Options{Addr: address, DB: 0}
	p.mainClient = redis.NewClient(opts)
}

func (p *Proxy) ping(conn redcon.Conn, cmd redcon.Command) {
	result, _ := p.mainClient.Ping(p.ctx).Result()
	p.cache = map[string]string{string(cmd.Args[0]): result}

	conn.WriteString(result)
}

func (p *Proxy) set(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 3 {
		conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
		return
	}

	expiration, _ := strconv.ParseInt(string(cmd.Args[3]), 10, 64)
	result, _ := p.mainClient.Set(p.ctx, string(cmd.Args[1]), string(cmd.Args[2]), time.Duration(expiration)).Result()
	p.cache = map[string]string{string(cmd.Args[0]): result}

	conn.WriteString(result)
}

func (p *Proxy) get(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) != 2 {
		conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
		return
	}

	result, _ := p.mainClient.Get(p.ctx, string(cmd.Args[1])).Result()
	p.cache = map[string]string{string(cmd.Args[0]): result}

	conn.WriteString(result)
}

func (p *Proxy) delete(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) != 2 {
		conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
		return
	}

	result, _ := p.mainClient.Del(p.ctx, string(cmd.Args[1])).Result()
	p.cache = map[string]string{string(cmd.Args[0]): strconv.Itoa(int(result))}

	conn.WriteInt(int(result))
}
