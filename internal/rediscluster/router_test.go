package rediscluster

import (
	"testing"
)

func TestKeySlotKnownVectors(t *testing.T) {
	cases := []struct {
		key  string
		want int
	}{
		// crc16-xmodem("123456789") = 0x31C3 = 12739; 12739 % 16384 = 12739.
		{"123456789", 12739},
		// Empty key hashes to 0.
		{"", 0},
	}
	for _, tc := range cases {
		if got := KeySlot([]byte(tc.key)); got != tc.want {
			t.Errorf("KeySlot(%q) = %d, want %d", tc.key, got, tc.want)
		}
	}
}

func TestKeySlotHashTag(t *testing.T) {
	// Keys sharing a hash tag must land on the same slot, and that slot must equal
	// the slot of the tag itself.
	a := KeySlot([]byte("foo{bar}1"))
	b := KeySlot([]byte("foo{bar}2"))
	tag := KeySlot([]byte("bar"))
	if a != b || a != tag {
		t.Errorf("hash tag co-location failed: foo{bar}1=%d foo{bar}2=%d bar=%d", a, b, tag)
	}

	// An empty tag ({}) is ignored and the whole key is hashed.
	whole := KeySlot([]byte("foo{}bar"))
	if whole == tag {
		t.Errorf("empty hash tag should be ignored; got tag slot %d", whole)
	}
}

// buildSlotsReply builds a CLUSTER SLOTS RESP2 reply for the given ranges.
// Each range is (start, end, masterIP, masterPort).
func buildSlotsReply(ranges ...[4]interface{}) []byte {
	out := []byte("*" + itoa(len(ranges)) + "\r\n")
	for _, r := range ranges {
		out = append(out, []byte("*4\r\n")...)
		out = append(out, []byte(":"+itoa(r[0].(int))+"\r\n")...) // start
		out = append(out, []byte(":"+itoa(r[1].(int))+"\r\n")...) // end
		// master node: array of (ip, port, nodeid)
		out = append(out, []byte("*3\r\n")...)
		out = appendBulk(out, r[2].(string))
		out = append(out, []byte(":"+itoa(r[3].(int))+"\r\n")...)
		out = appendBulk(out, "nodeid-"+r[2].(string))
		// one replica (ignored by parser): array of (ip, port, nodeid)
		out = append(out, []byte("*3\r\n")...)
		out = appendBulk(out, "replica-"+r[2].(string))
		out = append(out, []byte(":7001\r\n")...)
		out = appendBulk(out, "replica-id")
	}
	return out
}

func appendBulk(b []byte, s string) []byte {
	b = append(b, []byte("$"+itoa(len(s))+"\r\n")...)
	b = append(b, s...)
	b = append(b, []byte("\r\n")...)
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}

func TestParseClusterSlots(t *testing.T) {
	reply := buildSlotsReply(
		[4]interface{}{0, 5460, "10.0.0.1", 6379},
		[4]interface{}{5461, 16383, "10.0.0.2", 6379},
	)
	masters, err := ParseClusterSlots(reply)
	if err != nil {
		t.Fatalf("ParseClusterSlots: %v", err)
	}
	if got := masters[0]; got != "10.0.0.1:6379" {
		t.Errorf("slot 0 owner = %q, want 10.0.0.1:6379", got)
	}
	if got := masters[5460]; got != "10.0.0.1:6379" {
		t.Errorf("slot 5460 owner = %q, want 10.0.0.1:6379", got)
	}
	if got := masters[5461]; got != "10.0.0.2:6379" {
		t.Errorf("slot 5461 owner = %q, want 10.0.0.2:6379", got)
	}
	if got := masters[16383]; got != "10.0.0.2:6379" {
		t.Errorf("slot 16383 owner = %q, want 10.0.0.2:6379", got)
	}
}

func TestParseClusterSlotsMalformed(t *testing.T) {
	if _, err := ParseClusterSlots([]byte("+OK\r\n")); err == nil {
		t.Error("expected error for non-array reply, got nil")
	}
	if _, err := ParseClusterSlots([]byte("*2\r\n:1\r\n")); err == nil {
		t.Error("expected error for truncated reply, got nil")
	}
}

func TestRouterRouteWithMap(t *testing.T) {
	r := &Router{
		masters:  make([]string, NumSlots),
		fallback: "seed:6379",
	}
	// Manually populate a couple of slots.
	for s := 0; s <= 5460; s++ {
		r.masters[s] = "nodeA:6379"
	}
	for s := 5461; s < NumSlots; s++ {
		r.masters[s] = "nodeB:6379"
	}

	// Find keys that land in each half.
	var keyA, keyB []byte
	for i := 0; i < 100000 && (keyA == nil || keyB == nil); i++ {
		k := []byte("key-" + itoa(i))
		if KeySlot(k) <= 5460 && keyA == nil {
			keyA = k
		}
		if KeySlot(k) > 5460 && keyB == nil {
			keyB = k
		}
	}
	if got := r.Route(keyA); got != "nodeA:6379" {
		t.Errorf("Route(%q) = %q, want nodeA:6379", keyA, got)
	}
	if got := r.Route(keyB); got != "nodeB:6379" {
		t.Errorf("Route(%q) = %q, want nodeB:6379", keyB, got)
	}
}

func TestRouterFallbackWhenUnmapped(t *testing.T) {
	r := &Router{masters: make([]string, NumSlots), fallback: "seed:6379"}
	if got := r.Route([]byte("anything")); got != "seed:6379" {
		t.Errorf("Route on empty topology = %q, want fallback seed:6379", got)
	}
	if got := r.Fallback(); got != "seed:6379" {
		t.Errorf("Fallback() = %q, want seed:6379", got)
	}
}
