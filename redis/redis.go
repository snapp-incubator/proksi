package main

import (
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tidwall/redcon"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/config"
	"github.com/snapp-incubator/proksi/internal/logging"
	"github.com/snapp-incubator/proksi/internal/metrics"
)

var (
	help       bool   // Indicates whether to show the help or not
	configPath string // Path of config file
)

func init() {
	flag.BoolVar(&help, "help", false, "Show help")
	flag.StringVar(&configPath, "config", "", "The path of config file")
}

func main() {
	// Parse the terminal flags
	flag.Parse()

	// Usage Demo
	if help {
		flag.Usage()
		return
	}

	c := config.LoadRedis(configPath)

	if c.Upstreams.Main.Address == "" {
		logging.L.Fatal("Main upstream backend can not be empty.")
	}

	if c.Upstreams.Test.Address == "" {
		logging.L.Fatal("Test upstream backend can not be empty.")
	}

	mainClient, err := newUpstreamClient(c.Upstreams.Main.Address)
	if err != nil {
		logging.L.Fatal("Error in connecting to the main upstream", zap.Error(err))
	}
	defer mainClient.close()

	testClient, err := newUpstreamClient(c.Upstreams.Test.Address)
	if err != nil {
		logging.L.Fatal("Error in connecting to the test upstream", zap.Error(err))
	}
	defer testClient.close()

	jobs := make(chan Job, c.Worker.QueueSize)

	for i := uint(0); i < c.Worker.Count; i++ {
		go func() {
			for job := range jobs {
				job.Do()
			}
		}()
	}

	s := &server{
		job:        jobs,
		mainClient: mainClient,
		testClient: testClient,
	}

	srv := redcon.NewServer(c.Bind, s.handle, accept, closed)

	go func() {
		logging.L.Info("Starting Redis server",
			zap.String("address", c.Bind),
			zap.String("main_upstream", c.Upstreams.Main.Address),
			zap.String("test_upstream", c.Upstreams.Test.Address),
		)
		if err := srv.ListenAndServe(); err != nil {
			logging.L.Fatal("Redis server ListenAndServe Error", zap.Error(err))
		}
	}()

	if c.Metrics.Enabled {
		go metrics.InitializeRedis(c.Metrics.Bind)
	}

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint

	logging.L.Debug("Closing Redis server")
	if err := srv.Close(); err != nil {
		logging.L.Error("Error in shutting down the Redis server", zap.Error(err))
	}
	close(jobs)

	logging.L.Info("Redis server is shut down")
}

type server struct {
	job        chan Job
	mainClient *upstreamClient
	testClient *upstreamClient
}

// handle is the redcon command handler. It forwards the command to the main upstream
// synchronously and writes the raw reply back to the client. It then enqueues the
// command to the test upstream for asynchronous execution.
func (s *server) handle(conn redcon.Conn, cmd redcon.Command) {
	command := commandName(cmd.Args)

	loggingFieldsWithError := func(err error) []zap.Field {
		return []zap.Field{
			zap.String("command", command),
			zap.String("remote_addr", conn.RemoteAddr()),
			zap.Error(err),
		}
	}

	// Forward the command to the main upstream synchronously.
	timer := prometheus.NewTimer(metrics.RedisCmdDuration.WithLabelValues(command, "main_upstream"))
	reply, err := s.mainClient.send(cmd.Raw)
	timer.ObserveDuration()
	if err != nil {
		metrics.RedisCmdCounter.WithLabelValues("client_error", "main_upstream").Inc()
		logging.L.Error("error in sending the command to the main upstream", loggingFieldsWithError(err)...)
		conn.WriteError("ERR main upstream error: " + err.Error())
		return
	}

	metrics.RedisCmdCounter.WithLabelValues(command, "main_upstream").Inc()

	// Write the raw reply back to the client. The reply is the exact RESP bytes
	// received from the main upstream, preserving the reply type (status, error,
	// integer, bulk, array, ...).
	conn.WriteRaw(reply)

	// Enqueue the command to the test upstream for asynchronous execution.
	select {
	case s.job <- &upstreamTestJob{
		command: command,
		raw:     cmd.Raw,
		client:  s.testClient,
		logFields: func(err error) []zap.Field {
			return loggingFieldsWithError(err)
		},
	}:
	default:
		logging.L.Warn("dropping test upstream job due to full queue",
			zap.String("command", command),
			zap.String("remote_addr", conn.RemoteAddr()),
		)
	}
}

// Job is a unit of work executed by the worker pool.
type Job interface {
	Do()
}

// upstreamTestJob forwards a command to the test upstream asynchronously and discards
// the reply. Errors are logged but never returned to the client.
type upstreamTestJob struct {
	command   string
	raw       []byte
	client    *upstreamClient
	logFields func(err error) []zap.Field
}

func (j *upstreamTestJob) Do() {
	timer := prometheus.NewTimer(metrics.RedisCmdDuration.WithLabelValues(j.command, "test_upstream"))
	_, err := j.client.send(j.raw)
	timer.ObserveDuration()
	if err != nil {
		metrics.RedisCmdCounter.WithLabelValues("client_error", "test_upstream").Inc()
		logging.L.Error("error in sending the command to the test upstream", j.logFields(err)...)
		return
	}

	metrics.RedisCmdCounter.WithLabelValues(j.command, "test_upstream").Inc()
}

// accept is called to accept or deny the connection.
func accept(conn redcon.Conn) bool {
	return true
}

// closed is called when the connection has been closed.
func closed(conn redcon.Conn, err error) {
	if err != nil && !errors.Is(err, io.EOF) {
		logging.L.Debug("connection closed with error",
			zap.String("remote_addr", conn.RemoteAddr()),
			zap.Error(err),
		)
	}
}

// commandName returns the lowercased name of the command (the first argument).
func commandName(args [][]byte) string {
	if len(args) == 0 {
		return ""
	}
	name := make([]byte, len(args[0]))
	for i := 0; i < len(args[0]); i++ {
		c := args[0][i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		name[i] = c
	}
	return string(name)
}

// readTimeout is the maximum time to wait for a single upstream reply.
const readTimeout = 30 * time.Second

// upstreamClient is a pooled client for a single redis upstream. It maintains a pool
// of idle connections and dials new ones on demand.
type upstreamClient struct {
	addr    string
	idle    chan net.Conn
	closed  bool
	closeMu sync.Mutex
}

// newUpstreamClient creates a new pooled upstream client and verifies the upstream is
// reachable by dialing a probe connection.
func newUpstreamClient(addr string) (*upstreamClient, error) {
	c := &upstreamClient{
		addr: addr,
		idle: make(chan net.Conn, 32),
	}

	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	c.idle <- conn

	return c, nil
}

func (c *upstreamClient) dial() (net.Conn, error) {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return nil, errors.New("upstream client is closed")
	}
	return net.DialTimeout("tcp", c.addr, 5*time.Second)
}

// send writes a raw RESP command to the upstream and reads back one complete raw RESP
// reply. The connection is returned to the pool on success.
func (c *upstreamClient) send(raw []byte) ([]byte, error) {
	conn, err := c.acquire()
	if err != nil {
		return nil, err
	}

	reply, err := roundTrip(conn, raw)
	if err != nil {
		// The connection may be in a bad state; discard it.
		_ = conn.Close()
		return nil, err
	}

	c.release(conn)
	return reply, nil
}

func (c *upstreamClient) acquire() (net.Conn, error) {
	select {
	case conn := <-c.idle:
		return conn, nil
	default:
		return c.dial()
	}
}

func (c *upstreamClient) release(conn net.Conn) {
	select {
	case c.idle <- conn:
	default:
		_ = conn.Close()
	}
}

func (c *upstreamClient) close() {
	c.closeMu.Lock()
	c.closed = true
	c.closeMu.Unlock()
	close(c.idle)
	for conn := range c.idle {
		_ = conn.Close()
	}
}

// roundTrip writes a raw RESP command to conn and reads back one complete raw RESP reply.
// It uses redcon.ReadNextRESP to frame the reply, reading more bytes from the socket
// until a full reply is available.
func roundTrip(conn net.Conn, raw []byte) ([]byte, error) {
	if err := conn.SetDeadline(time.Now().Add(readTimeout)); err != nil {
		return nil, err
	}

	if _, err := conn.Write(raw); err != nil {
		return nil, err
	}

	// Read directly from the connection into a growing buffer and frame the reply
	// with redcon.ReadNextRESP. Reading raw (rather than through a bufio.Reader) keeps
	// the pooled connection free of leftover buffered state between commands.
	var buf []byte
	scratch := make([]byte, 4096)

	for {
		n, resp := redcon.ReadNextRESP(buf)
		if n > 0 {
			// A complete reply has been parsed; return its raw bytes. Copy it so the
			// scratch buffer can be reused for the next command.
			out := make([]byte, n)
			copy(out, buf[:n])
			return out, nil
		}
		_ = resp // resp is zero-value when n == 0 (incomplete).

		nn, err := conn.Read(scratch)
		if nn > 0 {
			buf = append(buf, scratch[:nn]...)
			// Try framing again before handling a read error so a final short read
			// that completes the reply is not reported as an error.
			continue
		}
		if err != nil {
			if err == io.EOF && len(buf) > 0 {
				return nil, errors.New("unexpected EOF while reading upstream reply")
			}
			return nil, err
		}
	}
}