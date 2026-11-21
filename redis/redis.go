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
	"github.com/snapp-incubator/proksi/internal/rediscluster"
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

	mainBackend, err := newBackend(c.Upstreams.Main)
	if err != nil {
		logging.L.Fatal("Error in connecting to the main upstream", zap.Error(err))
	}
	defer mainBackend.close()

	testBackend, err := newBackend(c.Upstreams.Test)
	if err != nil {
		logging.L.Fatal("Error in connecting to the test upstream", zap.Error(err))
	}
	defer testBackend.close()

	jobs := make(chan Job, c.Worker.QueueSize)

	for i := uint(0); i < c.Worker.Count; i++ {
		go func() {
			for job := range jobs {
				job.Do()
			}
		}()
	}

	s := &server{
		job:         jobs,
		mainBackend: mainBackend,
		testBackend: testBackend,
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
	job         chan Job
	mainBackend backend
	testBackend backend
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
	reply, err := s.mainBackend.send(&cmd)
	timer.ObserveDuration()
	if err != nil {
		metrics.RedisCmdCounter.WithLabelValues("client_error", "main_upstream").Inc()
		logging.L.Error("error in sending the command to the main upstream", loggingFieldsWithError(err)...)
		conn.WriteError("ERR main upstream error: " + err.Error())
		return
	}

	metrics.RedisCmdCounter.WithLabelValues(command, "main_upstream").Inc()

	// If the main cluster reports that the key's slot has moved, refresh the topology
	// in the background so subsequent commands route to the new owner. The reply is
	// still returned verbatim; the client (or a retry) will land on the right node.
	if isMovedReply(reply) {
		s.mainBackend.refresh()
	}

	// Write the raw reply back to the client. The reply is the exact RESP bytes
	// received from the main upstream, preserving the reply type (status, error,
	// integer, bulk, array, ...).
	conn.WriteRaw(reply)

	// Enqueue the command to the test upstream for asynchronous execution.
	select {
	case s.job <- &upstreamTestJob{
		command: command,
		cmd:     &cmd,
		backend: s.testBackend,
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
	cmd       *redcon.Command
	backend   backend
	logFields func(err error) []zap.Field
}

func (j *upstreamTestJob) Do() {
	timer := prometheus.NewTimer(metrics.RedisCmdDuration.WithLabelValues(j.command, "test_upstream"))
	_, err := j.backend.send(j.cmd)
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
	addr     string
	password string
	idle     chan net.Conn

	// mu guards closed. wg tracks in-flight sends so close() can wait for them to
	// finish (and their connections to be released) before tearing down the pool.
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// newUpstreamClient creates a new pooled upstream client and verifies the upstream is
// reachable by dialing a probe connection.
func newUpstreamClient(addr string, password string) (*upstreamClient, error) {
	c := &upstreamClient{
		addr:     addr,
		password: password,
		idle:     make(chan net.Conn, 32),
	}

	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	c.idle <- conn

	return c, nil
}

// dial opens a new TCP connection to the upstream and, when a password is
// configured, authenticates it with AUTH before handing it out.
func (c *upstreamClient) dial() (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
	if err != nil {
		return nil, err
	}

	if c.password != "" {
		authCmd := redcon.AppendArray(nil, 2)
		authCmd = redcon.AppendBulkString(authCmd, "AUTH")
		authCmd = redcon.AppendBulkString(authCmd, c.password)
		reply, err := roundTrip(conn, authCmd)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		// A successful AUTH replies with the simple string "OK". Anything else
		// (e.g. an -ERR reply) means the credentials were rejected.
		if string(reply) != "+OK\r\n" {
			_ = conn.Close()
			return nil, errors.New("authentication to the upstream failed: " + string(reply))
		}
	}

	return conn, nil
}

// send writes a raw RESP command to the upstream and reads back one complete raw RESP
// reply. The connection is returned to the pool on success. send registers itself with
// the wait group so close() waits for in-flight commands before closing the pool.
func (c *upstreamClient) send(raw []byte) ([]byte, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("upstream client is closed")
	}
	c.wg.Add(1)
	c.mu.Unlock()
	defer c.wg.Done()

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

// close marks the client closed, waits for in-flight sends to release their
// connections, then closes the idle channel and every pooled connection.
func (c *upstreamClient) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	c.wg.Wait()

	close(c.idle)
	for conn := range c.idle {
		_ = conn.Close()
	}
}

// backend is a single upstream target (standalone redis or a redis cluster). It routes
// a command to the right node and returns the raw RESP reply.
type backend interface {
	send(cmd *redcon.Command) ([]byte, error)
	// refresh re-pulls cluster topology (no-op for standalone backends).
	refresh()
	close()
}

// newBackend builds a backend for one upstream config: a cluster router when the
// upstream is configured as a cluster, otherwise a standalone single-address client.
func newBackend(u config.RedisUpstream) (backend, error) {
	if u.Cluster.Enabled {
		return newClusterBackend(u.Cluster.Addresses, u.Password)
	}
	return newStandaloneBackend(u.Address, u.Password)
}

// standaloneBackend forwards every command to a single upstream address.
type standaloneBackend struct {
	client *upstreamClient
}

func newStandaloneBackend(addr string, password string) (*standaloneBackend, error) {
	c, err := newUpstreamClient(addr, password)
	if err != nil {
		return nil, err
	}
	return &standaloneBackend{client: c}, nil
}

func (b *standaloneBackend) send(cmd *redcon.Command) ([]byte, error) {
	return b.client.send(cmd.Raw)
}

func (b *standaloneBackend) refresh() {}

func (b *standaloneBackend) close() { b.client.close() }

// clusterBackend routes each keyed command to the cluster node that owns its slot,
// maintaining a connection pool per node. Keyless commands go to the fallback seed.
type clusterBackend struct {
	router   *rediscluster.Router
	password string

	mu    sync.Mutex
	pools map[string]*upstreamClient
}

func newClusterBackend(seeds []string, password string) (*clusterBackend, error) {
	router, err := rediscluster.NewRouter(seeds, func(addr string) (net.Conn, error) {
		return net.DialTimeout("tcp", addr, 5*time.Second)
	})
	if err != nil {
		return nil, err
	}
	return &clusterBackend{
		router:   router,
		password: password,
		pools:    make(map[string]*upstreamClient),
	}, nil
}

// poolFor returns (creating on first use) the connection pool for a node address.
func (b *clusterBackend) poolFor(addr string) (*upstreamClient, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.pools[addr]; ok {
		return c, nil
	}
	c, err := newUpstreamClient(addr, b.password)
	if err != nil {
		return nil, err
	}
	b.pools[addr] = c
	return c, nil
}

func (b *clusterBackend) send(cmd *redcon.Command) ([]byte, error) {
	addr := b.router.Fallback()
	if key := firstKey(cmd); key != nil {
		addr = b.router.Route(key)
	}

	pool, err := b.poolFor(addr)
	if err != nil {
		return nil, err
	}
	return pool.send(cmd.Raw)
}

func (b *clusterBackend) refresh() {
	// Re-pull topology in the background so a MOVED reply doesn't stall the client.
	go func() {
		if err := b.router.Refresh(nil); err != nil {
			logging.L.Warn("failed to refresh cluster topology", zap.Error(err))
		}
	}()
}

func (b *clusterBackend) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.pools {
		c.close()
	}
}

// firstKey returns the first key of a command (cmd.Args[1] for commands with at least
// one argument after the command name), or nil for keyless commands (PING, INFO, ...).
func firstKey(cmd *redcon.Command) []byte {
	if len(cmd.Args) >= 2 {
		return cmd.Args[1]
	}
	return nil
}

// isMovedReply reports whether a raw RESP reply is a -MOVED or -ASK cluster redirect.
func isMovedReply(reply []byte) bool {
	if len(reply) < 2 || reply[0] != '-' {
		return false
	}
	line := string(reply[1:])
	return hasPrefixFold(line, "MOVED ") || hasPrefixFold(line, "ASK ")
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		a, c := s[i], prefix[i]
		if a >= 'a' && a <= 'z' {
			a -= 'a' - 'A'
		}
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if a != c {
			return false
		}
	}
	return true
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
