package main

import (
	"flag"
	"log"
	"strconv"

	"github.com/tidwall/redcon"

	"github.com/snapp-incubator/proksi/internal/config"
)

type ServerType string

const (
	Main ServerType = "main"
	Test ServerType = "test"
)

type FunctionType string

const (
	Del FunctionType = "del"
)

var (
	help       bool   // Indicates whether to show the help or not
	configPath string // Path of config file
)

func init() {
	flag.BoolVar(&help, "help", false, "Show help")
	flag.StringVar(&configPath, "config", "", "The path of config file")

	// Parse the terminal flags
	flag.Parse()
}

func main() {
	// Usage Demo
	if help {
		flag.Usage()
		return
	}

	c := config.LoadRedis(configPath)

	proxy := NewProxy()

	proxy.ConnectToServer(c.Backend.Address)

	errSig := make(chan bool)

	log.Printf("proxing from %v to %v\n", c.TestFrontend.Bind, c.Backend.Address)

	go proxy.serve(Main, c.MainFrontend.Bind, errSig)
	go proxy.serve(Test, c.TestFrontend.Bind, errSig)
	<-proxy.errSig
}

// serve implements the Redis server.
func (p *Proxy) serve(serverType ServerType, address string, errSig chan bool) {
	var err error

	for {
		log.Printf("%s server started at %s", serverType, address)

		if serverType == Main {
			err = redcon.ListenAndServe(address, p.mainHandler(), accept, p.closed())
			if err != nil {
				log.Print(err)
				errSig <- true
				return
			}
		} else {
			err = redcon.ListenAndServe(address, p.testHandler(), accept, p.closed())
			if err != nil {
				log.Print(err)
				errSig <- true
				return
			}
		}
	}
}

// mainHandler is an RESP handler for the main Redis server that responds to command and fills a cache.
func (p *Proxy) mainHandler() func(conn redcon.Conn, cmd redcon.Command) {
	mux := redcon.NewServeMux()
	mux.HandleFunc("ping", p.ping)
	mux.HandleFunc("set", p.set)
	mux.HandleFunc("get", p.get)
	mux.HandleFunc("del", p.delete)

	return mux.ServeRESP
}

// testHandler is an RESP handler for the test Redis server that lookup the cache and sends responses.
func (p *Proxy) testHandler() func(conn redcon.Conn, cmd redcon.Command) {
	return func(conn redcon.Conn, cmd redcon.Command) {
		result, found := p.cache[string(cmd.Args[0])]

		if found {
			if string(cmd.Args[0]) == string(Del) {
				i, _ := strconv.Atoi(result)
				conn.WriteInt(i)
			} else {
				conn.WriteString(result)
			}
		}
	}
}

// accept is called to accept or deny the connection.
func accept(conn redcon.Conn) bool {
	log.Printf("The connection between %s and %s haas been accepted", conn.NetConn().LocalAddr().String(), conn.RemoteAddr())
	return true
}

// closed is called when the connection has been closed.
func (p *Proxy) closed() func(conn redcon.Conn, err error) {
	return func(conn redcon.Conn, err error) {
		log.Printf("The connection between %s, %s has been closed, err: %v", conn.NetConn().LocalAddr().String(), conn.RemoteAddr(), err)
		<-p.errSig
	}
}
