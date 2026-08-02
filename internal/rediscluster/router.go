package rediscluster

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/tidwall/redcon"
)

// DialFunc dials a TCP address. It is abstracted so tests can stub connections.
type DialFunc func(addr string) (net.Conn, error)

// Router holds the slot->master-node topology of one Redis Cluster and routes a
// key to the address of the node that owns its slot.
type Router struct {
	dial DialFunc

	mu      sync.RWMutex
	masters []string // masters[slot] = "host:port" of the master owning that slot
	fallback string  // address used for keyless commands and as a dial seed
}

// NewRouter loads the cluster topology by issuing CLUSTER SLOTS against one of
// the seed addresses and returns a Router. It fails if no seed yields a valid
// topology.
func NewRouter(seeds []string, dial DialFunc) (*Router, error) {
	r := &Router{dial: dial}
	if len(seeds) > 0 {
		r.fallback = seeds[0]
	}
	if err := r.Refresh(seeds); err != nil {
		return nil, err
	}
	return r, nil
}

// Route returns the master address owning key's slot. If no topology is loaded
// (or the slot is unmapped), it returns the fallback seed address.
func (r *Router) Route(key []byte) string {
	slot := KeySlot(key)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if addr := r.masters[slot]; addr != "" {
		return addr
	}
	return r.fallback
}

// Fallback returns the address used for keyless commands.
func (r *Router) Fallback() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fallback
}

// Refresh re-issues CLUSTER SLOTS against the given seeds (falling back to the
// current fallback) and atomically swaps the topology on success.
func (r *Router) Refresh(seeds []string) error {
	if len(seeds) == 0 {
		r.mu.RLock()
		seeds = []string{r.fallback}
		r.mu.RUnlock()
	}

	var lastErr error
	for _, seed := range seeds {
		masters, err := fetchTopology(seed, r.dial)
		if err != nil {
			lastErr = err
			continue
		}
		r.mu.Lock()
		r.masters = masters
		r.mu.Unlock()
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("no seed addresses configured")
	}
	return lastErr
}

// fetchTopology dials seed, issues CLUSTER SLOTS, and builds the slot->master map.
func fetchTopology(seed string, dial DialFunc) ([]string, error) {
	conn, err := dial(seed)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	cmd := redcon.AppendArray(nil, 2)
	cmd = redcon.AppendBulkString(cmd, "CLUSTER")
	cmd = redcon.AppendBulkString(cmd, "SLOTS")
	if _, err := conn.Write(cmd); err != nil {
		return nil, err
	}

	raw, err := readOneRESP(conn)
	if err != nil {
		return nil, err
	}
	return ParseClusterSlots(raw)
}

// readOneRESP reads from conn until one complete RESP value is available and
// returns its raw bytes.
func readOneRESP(conn net.Conn) ([]byte, error) {
	var buf []byte
	scratch := make([]byte, 4096)
	for {
		if n, _ := redcon.ReadNextRESP(buf); n > 0 {
			out := make([]byte, n)
			copy(out, buf[:n])
			return out, nil
		}
		nn, err := conn.Read(scratch)
		if nn > 0 {
			buf = append(buf, scratch[:nn]...)
			continue
		}
		if err != nil {
			return nil, err
		}
	}
}

// ParseClusterSlots decodes a raw CLUSTER SLOTS reply into a slot->master map.
// Reply shape (RESP2):
//
//	*<ranges>
//	  *<range>
//	    :<startSlot> :<endSlot>
//	    *<master>  $<ip> :<port> [$<nodeid> ...]
//	    *<replica> ...   (ignored)
func ParseClusterSlots(raw []byte) ([]string, error) {
	v, rest, err := parseRESP(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing CLUSTER SLOTS reply: %w", err)
	}
	_ = rest
	if len(raw) == 0 || raw[0] != '*' {
		return nil, fmt.Errorf("unexpected CLUSTER SLOTS reply type %q", firstByte(raw))
	}

	masters := make([]string, NumSlots)
	for _, rng := range v.arr {
		if len(rng.arr) < 3 {
			continue
		}
		start := rng.arr[0].num
		end := rng.arr[1].num
		master := rng.arr[2]
		if len(master.arr) < 2 {
			continue
		}
		addr := net.JoinHostPort(master.arr[0].str, fmt.Sprintf("%d", master.arr[1].num))
		if start < 0 {
			start = 0
		}
		if end >= NumSlots {
			end = NumSlots - 1
		}
		for s := start; s <= end; s++ {
			masters[s] = addr
		}
	}
	return masters, nil
}

func firstByte(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return string(b[0])
}

// respValue is a minimally-decoded RESP value: enough to walk CLUSTER SLOTS.
type respValue struct {
	str string      // bulk/simple string payload
	num int         // integer payload
	arr []respValue // array elements
}

// parseRESP decodes one RESP value from b, returning the value and the bytes
// following it.
func parseRESP(b []byte) (respValue, []byte, error) {
	if len(b) == 0 {
		return respValue{}, nil, errors.New("empty RESP")
	}
	switch b[0] {
	case '+', '-': // simple string / error
		line, rest, err := readLine(b[1:])
		return respValue{str: string(line)}, rest, err
	case ':': // integer
		line, rest, err := readLine(b[1:])
		var n int
		fmt.Sscanf(string(line), "%d", &n)
		return respValue{num: n}, rest, err
	case '$': // bulk string
		line, rest, err := readLine(b[1:])
		if err != nil {
			return respValue{}, nil, err
		}
		var n int
		fmt.Sscanf(string(line), "%d", &n)
		if n < 0 {
			return respValue{}, rest, nil
		}
		if len(rest) < n+2 {
			return respValue{}, nil, errors.New("truncated bulk string")
		}
		return respValue{str: string(rest[:n])}, rest[n+2:], nil
	case '*': // array
		line, rest, err := readLine(b[1:])
		if err != nil {
			return respValue{}, nil, err
		}
		var n int
		fmt.Sscanf(string(line), "%d", &n)
		if n <= 0 {
			return respValue{}, rest, nil
		}
		arr := make([]respValue, 0, n)
		cur := rest
		for i := 0; i < n; i++ {
			v, r, err := parseRESP(cur)
			if err != nil {
				return respValue{}, nil, err
			}
			arr = append(arr, v)
			cur = r
		}
		return respValue{arr: arr}, cur, nil
	default:
		return respValue{}, nil, fmt.Errorf("unknown RESP type %q", b[0])
	}
}

// readLine returns the bytes up to the first CRLF and the remainder after it.
func readLine(b []byte) ([]byte, []byte, error) {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '\r' && b[i+1] == '\n' {
			return b[:i], b[i+2:], nil
		}
	}
	return nil, nil, errors.New("unterminated RESP line")
}
