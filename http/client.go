package main

import (
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/snapp-incubator/proksi/internal/config"
)

// newUpstreamClient builds an HTTP client for an upstream using its configured timeouts.
// A zero timeout disables the corresponding limit.
func newUpstreamClient(u config.HTTPUpstream) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   u.DialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = u.TLSHandshakeTimeout
	transport.ResponseHeaderTimeout = u.ResponseHeaderTimeout

	return &http.Client{
		Timeout:   u.Timeout,
		Transport: transport,
	}
}

// hopByHopHeaders are connection-scoped headers that must not be forwarded by a proxy (RFC 9110 section 7.6.1).
var hopByHopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// stripHopByHop removes hop-by-hop headers, including those named by the Connection header, from h in place.
func stripHopByHop(h http.Header) http.Header {
	for _, v := range h.Values("Connection") {
		for _, name := range strings.Split(v, ",") {
			if name = strings.TrimSpace(name); name != "" {
				h.Del(textproto.CanonicalMIMEHeaderKey(name))
			}
		}
	}
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}

	return h
}
