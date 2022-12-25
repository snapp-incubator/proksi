package redis

import (
	"sync"

	"github.com/tidwall/redcon"
)

type Proxy struct {
	mux    sync.RWMutex
	items  map[string][]byte
	cache  map[string][]byte
	errSig chan bool
}

func NewProxy() *Proxy {
	return &Proxy{
		items:  make(map[string][]byte),
		cache:  make(map[string][]byte),
		errSig: make(chan bool),
	}
}

func (p *Proxy) ping(conn redcon.Conn, cmd redcon.Command) {
	p.cache = map[string][]byte{string(cmd.Args[0]): []byte("PONG")}
	conn.WriteString("PONG")
}

func (p *Proxy) set(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 3 {
		conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
		return
	}

	p.mux.Lock()
	p.items[string(cmd.Args[1])] = cmd.Args[2]
	p.mux.Unlock()

	p.cache = map[string][]byte{string(cmd.Args[0]): []byte("OK")}

	conn.WriteString("OK")
}

func (p *Proxy) get(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) != 2 {
		conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
		return
	}

	p.mux.RLock()
	value, found := p.items[string(cmd.Args[1])]
	p.mux.RUnlock()

	if !found {
		p.cache = map[string][]byte{string(cmd.Args[0]): []byte("")}
		conn.WriteNull()
	}
	p.cache = map[string][]byte{string(cmd.Args[0]): value}
	conn.WriteBulk(value)
}

func (p *Proxy) delete(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) != 2 {
		conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
		return
	}

	p.mux.RLock()
	_, found := p.items[string(cmd.Args[1])]
	delete(p.items, string(cmd.Args[1]))
	p.mux.Unlock()

	if !found {
		p.cache = map[string][]byte{string(cmd.Args[0]): []byte("0")}
		conn.WriteInt(0)
	}
	p.cache = map[string][]byte{string(cmd.Args[0]): []byte("1")}
	conn.WriteInt(1)
}
