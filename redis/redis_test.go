package main

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tidwall/redcon"

	"github.com/snapp-incubator/proksi/internal/rediscluster"
)

// fakeUpstream is a minimal redis upstream backed by a redcon server. It records every
// command it receives and replies with a canned response per command.
type fakeUpstream struct {
	addr     string
	server   *redcon.Server
	mu       sync.Mutex
	received [][]byte
	reply    func(cmd redcon.Command) []byte
}

func newFakeUpstream(reply func(cmd redcon.Command) []byte) (*fakeUpstream, error) {
	u := &fakeUpstream{reply: reply}

	srv := redcon.NewServer("127.0.0.1:0",
		func(conn redcon.Conn, cmd redcon.Command) {
			u.mu.Lock()
			// Copy raw so it survives after redcon reuses its buffers (the live server
			// already copies, but be defensive).
			raw := append([]byte(nil), cmd.Raw...)
			u.received = append(u.received, raw)
			u.mu.Unlock()

			conn.WriteRaw(reply(cmd))
		},
		func(redcon.Conn) bool { return true },
		func(redcon.Conn, error) {},
	)
	u.server = srv

	signal := make(chan error, 1)
	go func() { _ = srv.ListenServeAndSignal(signal) }()
	if err := <-signal; err != nil {
		return nil, fmt.Errorf("fake upstream failed to listen: %v", err)
	}
	u.addr = srv.Addr().String()

	// Wait until the listener accepts connections.
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", u.addr, 50*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return u, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("fake upstream %s did not come up", u.addr)
}

func (u *fakeUpstream) stop() {
	_ = u.server.Close()
}

func (u *fakeUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.received)
}

// respClient is a minimal raw RESP client used to drive commands through the proxy.
type respClient struct {
	conn net.Conn
}

func dialResp(addr string) (*respClient, error) {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return &respClient{conn: c}, nil
}

func (c *respClient) close() { _ = c.conn.Close() }

// do writes a command (as RESP array of bulk strings) and reads back one raw RESP reply.
func (c *respClient) do(args ...string) ([]byte, error) {
	var wr redcon.Writer
	wr.WriteArray(len(args))
	for _, a := range args {
		wr.WriteBulkString(a)
	}
	if _, err := c.conn.Write(wr.Buffer()); err != nil {
		return nil, err
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 0, 256)
	scratch := make([]byte, 4096)
	for {
		n, _ := redcon.ReadNextRESP(buf)
		if n > 0 {
			out := make([]byte, n)
			copy(out, buf[:n])
			return out, nil
		}
		nn, err := c.conn.Read(scratch)
		if nn > 0 {
			buf = append(buf, scratch[:nn]...)
			continue
		}
		if err != nil {
			return nil, err
		}
	}
}

// TestProxyEndToEnd verifies that:
//   - the proxy returns the main upstream's raw reply to the client verbatim;
//   - the test upstream receives the same command asynchronously;
//   - reply types (status, integer, bulk, array, error, nil) are preserved.
func TestProxyEndToEnd(t *testing.T) {
	// Main upstream: replies depend on the command to exercise every RESP type.
	main, err := newFakeUpstream(func(cmd redcon.Command) []byte {
		switch strings.ToLower(string(cmd.Args[0])) {
		case "ping":
			return []byte("+PONG\r\n") // status
		case "incr":
			return []byte(":42\r\n") // integer
		case "get":
			return []byte("$5\r\nhello\r\n") // bulk
		case "hgetall":
			return []byte("*4\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$3\r\nbaz\r\n$3\r\nqux\r\n") // array
		case "del":
			return []byte("$-1\r\n") // nil bulk
		case "fail":
			return []byte("-ERR boom\r\n") // error
		default:
			return []byte("+OK\r\n")
		}
	})
	if err != nil {
		t.Fatalf("start main upstream: %v", err)
	}
	defer main.stop()

	// Test upstream: records the command, replies OK (reply is discarded by the proxy).
	test, err := newFakeUpstream(func(cmd redcon.Command) []byte {
		return []byte("+OK\r\n")
	})
	if err != nil {
		t.Fatalf("start test upstream: %v", err)
	}
	defer test.stop()

	mainBackend, err := newStandaloneBackend(main.addr)
	if err != nil {
		t.Fatalf("main upstream backend: %v", err)
	}
	defer mainBackend.close()

	testBackend, err := newStandaloneBackend(test.addr)
	if err != nil {
		t.Fatalf("test upstream backend: %v", err)
	}
	defer testBackend.close()

	jobs := make(chan Job, 256)
	for i := 0; i < 8; i++ {
		go func() {
			for job := range jobs {
				job.Do()
			}
		}()
	}
	defer close(jobs)

	s := &server{job: jobs, mainBackend: mainBackend, testBackend: testBackend}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	proxyAddr := ln.Addr().String()
	srv := redcon.NewServer(proxyAddr, s.handle, accept, closed)
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	// Wait for the proxy to accept connections.
	if c, err := net.DialTimeout("tcp", proxyAddr, 1*time.Second); err == nil {
		_ = c.Close()
	} else {
		t.Fatalf("proxy did not come up: %v", err)
	}

	cli, err := dialResp(proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer cli.close()

	cases := []struct {
		name string
		args []string
		want []byte
	}{
		{"status", []string{"PING"}, []byte("+PONG\r\n")},
		{"integer", []string{"INCR", "counter"}, []byte(":42\r\n")},
		{"bulk", []string{"GET", "k"}, []byte("$5\r\nhello\r\n")},
		{"array", []string{"HGETALL", "h"}, []byte("*4\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$3\r\nbaz\r\n$3\r\nqux\r\n")},
		{"nil", []string{"DEL", "missing"}, []byte("$-1\r\n")},
		{"error", []string{"FAIL"}, []byte("-ERR boom\r\n")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cli.do(tc.args...)
			if err != nil {
				t.Fatalf("do %v: %v", tc.args, err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("reply mismatch\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
	}

	// Every command (including the error and nil cases) must have been mirrored to the
	// test upstream. The test path is async; poll until the count matches.
	wantCount := len(cases)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if test.count() >= wantCount {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := test.count(); got != wantCount {
		t.Fatalf("test upstream received %d commands, want %d", got, wantCount)
	}
}

// TestProxyClusterRouting verifies that with a cluster backend the proxy routes each
// keyed command to the node that owns its slot (per CLUSTER SLOTS) and still mirrors
// the command to the test upstream.
func TestProxyClusterRouting(t *testing.T) {
	// Two fake "master" nodes for the main cluster. nodeA owns slots 0..5460, nodeB the rest.
	nodeA, err := newFakeUpstream(func(cmd redcon.Command) []byte {
		return []byte("+OK\r\n")
	})
	if err != nil {
		t.Fatalf("start nodeA: %v", err)
	}
	defer nodeA.stop()

	nodeB, err := newFakeUpstream(func(cmd redcon.Command) []byte {
		return []byte("+OK\r\n")
	})
	if err != nil {
		t.Fatalf("start nodeB: %v", err)
	}
	defer nodeB.stop()

	// The seed node answers CLUSTER SLOTS, advertising nodeA/nodeB as the masters.
	seed, err := newFakeUpstream(func(cmd redcon.Command) []byte {
		if strings.EqualFold(string(cmd.Args[0]), "cluster") {
			return clusterSlotsReply(nodeA.addr, nodeB.addr)
		}
		return []byte("+OK\r\n")
	})
	if err != nil {
		t.Fatalf("start seed: %v", err)
	}
	defer seed.stop()

	// Test upstream: any single node; just records mirrored commands.
	test, err := newFakeUpstream(func(cmd redcon.Command) []byte {
		return []byte("+OK\r\n")
	})
	if err != nil {
		t.Fatalf("start test upstream: %v", err)
	}
	defer test.stop()

	// Build the cluster backend against the seed.
	cluster, err := newClusterBackend([]string{seed.addr})
	if err != nil {
		t.Fatalf("newClusterBackend: %v", err)
	}
	defer cluster.close()

	testBackend, err := newStandaloneBackend(test.addr)
	if err != nil {
		t.Fatalf("test backend: %v", err)
	}
	defer testBackend.close()

	jobs := make(chan Job, 256)
	for i := 0; i < 8; i++ {
		go func() {
			for job := range jobs {
				job.Do()
			}
		}()
	}
	defer close(jobs)

	s := &server{job: jobs, mainBackend: cluster, testBackend: testBackend}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	proxyAddr := ln.Addr().String()
	srv := redcon.NewServer(proxyAddr, s.handle, accept, closed)
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	cli, err := dialResp(proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer cli.close()

	// Pick keys that land on each half of the slot space.
	keyA := keyForSlot(t, func(slot int) bool { return slot <= 5460 })
	keyB := keyForSlot(t, func(slot int) bool { return slot > 5460 })

	if _, err := cli.do("SET", keyA, "1"); err != nil {
		t.Fatalf("SET %s: %v", keyA, err)
	}
	if _, err := cli.do("SET", keyB, "1"); err != nil {
		t.Fatalf("SET %s: %v", keyB, err)
	}

	// The slot-low key must have gone to nodeA, the slot-high key to nodeB.
	if got := nodeA.count(); got != 1 {
		t.Fatalf("nodeA received %d commands, want 1", got)
	}
	if got := nodeB.count(); got != 1 {
		t.Fatalf("nodeB received %d commands, want 1", got)
	}

	// Both commands must be mirrored to the test upstream asynchronously.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && test.count() < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := test.count(); got != 2 {
		t.Fatalf("test upstream received %d commands, want 2", got)
	}
}

// clusterSlotsReply builds a CLUSTER SLOTS reply mapping 0..5460 to addrA and the rest to addrB.
func clusterSlotsReply(addrA, addrB string) []byte {
	hostA, portA, _ := net.SplitHostPort(addrA)
	hostB, portB, _ := net.SplitHostPort(addrB)
	var b []byte
	b = append(b, []byte("*2\r\n")...)
	// range 1
	b = append(b, []byte("*3\r\n:0\r\n:5460\r\n")...)
	b = append(b, nodeInfo(hostA, portA)...)
	// range 2
	b = append(b, []byte("*3\r\n:5461\r\n:16383\r\n")...)
	b = append(b, nodeInfo(hostB, portB)...)
	return b
}

func nodeInfo(host, port string) []byte {
	var b []byte
	b = append(b, []byte("*3\r\n")...)
	b = append(b, []byte("$"+fmt.Sprint(len(host))+"\r\n"+host+"\r\n")...)
	b = append(b, []byte(":"+port+"\r\n")...)
	b = append(b, []byte("$9\r\nnode-0001\r\n")...)
	return b
}

// keyForSlot returns a key whose slot satisfies pred.
func keyForSlot(t *testing.T, pred func(int) bool) string {
	t.Helper()
	for i := 0; i < 100000; i++ {
		k := fmt.Sprintf("key-%d", i)
		if pred(rediscluster.KeySlot([]byte(k))) {
			return k
		}
	}
	t.Fatal("no key found for predicate")
	return ""
}