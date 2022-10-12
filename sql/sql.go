package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"

	"github.com/snapp-incubator/proksi/internal/config"
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
	var err error

	// Usage Demo
	if help {
		flag.Usage()
		return
	}

	c := config.LoadSQL(configPath)

	log.Printf("proxing from %v to %v\n", c.MainFrontend.Bind, c.Backend.Address)

	laddr, err := net.ResolveTCPAddr("tcp", c.MainFrontend.Bind)
	if err != nil {
		log.Printf("Failed to resolve local address: %s", err)
		os.Exit(1)
	}

	raddr, err := net.ResolveTCPAddr("tcp", c.Backend.Address)
	if err != nil {
		log.Printf("Failed to resolve remote address: %s", err)
		os.Exit(1)
	}
	listener, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		log.Printf("Failed to open local port to listen: %s", err)
		os.Exit(1)
	}

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Printf("Failed to accept connection '%s'", err)
			continue
		}

		p := &Proxy{
			lconn: conn,
			laddr: laddr,
			raddr: raddr,
		}

		errsig := make(chan bool)

		go func() {
			defer p.lconn.Close()

			var err error
			p.rconn, err = net.DialTCP("tcp", nil, p.raddr)
			if err != nil {
				log.Printf("Remote connection failed: %s\n", err)
				return
			}
			defer p.rconn.Close()

			//display both ends
			log.Printf("Opened %s >>> %s\n", p.laddr.String(), p.raddr.String())

			//bidirectional copy
			go p.pipe(p.lconn, p.rconn, errsig)
			go p.pipe(p.rconn, p.lconn, errsig)

			//wait for close...
			<-p.errsig
			log.Printf("Closed (%d bytes sent, %d bytes recieved)\n", 0, 0)
		}()
	}
}

// Proxy - Manages a Proxy connection, piping data between local and remote.
type Proxy struct {
	laddr, raddr *net.TCPAddr
	lconn, rconn io.ReadWriteCloser
	errsig       chan bool
}

func (p *Proxy) pipe(src, dst io.ReadWriter, errsig chan bool) {
	//directional copy (64k buffer)
	buff := make([]byte, 0xffff)
	for {
		n, err := src.Read(buff)
		if err != nil {
			if err != io.EOF {
				log.Printf("Read failed '%s'\n", err)
			}
			errsig <- true
			return
		}
		b := buff[:n]

		//log.Println(string(b))

		//write out result
		n, err = dst.Write(b)
		if err != nil {
			if err != io.EOF {
				log.Printf("Write failed '%s'\n", err)
			}

			errsig <- true
			return
		}
	}
}
