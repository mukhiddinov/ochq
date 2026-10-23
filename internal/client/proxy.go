package client

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// RequestLog describes one completed request, for the terminal log.
type RequestLog struct {
	Method   string
	Path     string
	Status   int
	Duration time.Duration
}

// newLocalProxy forwards tunnel requests to the local service unchanged:
// same Host header and the X-Forwarded-* headers ochqd set.
func newLocalProxy(localAddr string) http.Handler {
	target := &url.URL{Scheme: "http", Host: localAddr}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host
			// Rewrite strips inbound X-Forwarded-*; these came from ochqd,
			// which we trust, and carry the visitor's real IP.
			for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
				if v := pr.In.Header[h]; v != nil {
					pr.Out.Header[h] = v
				}
			}
		},
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: localDialTimeout}).DialContext,
			MaxIdleConnsPerHost: 32,
			IdleConnTimeout:     90 * time.Second,
			// Pass bodies through byte for byte instead of letting the
			// transport negotiate and undo gzip.
			DisableCompression: true,
		},
		// Flush every write: server-sent events and long polling must not
		// sit in a buffer.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, fmt.Sprintf("ochq: could not reach local service at %s: %v", localAddr, err), http.StatusBadGateway)
		},
	}
}

// logRequests reports every request to onRequest after it completes.
func logRequests(next http.Handler, onRequest func(RequestLog)) http.Handler {
	if onRequest == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		onRequest(RequestLog{
			Method:   r.Method,
			Path:     r.URL.RequestURI(),
			Status:   rec.status,
			Duration: time.Since(start),
		})
	})
}

// statusRecorder captures the response status. Unwrap lets
// http.ResponseController reach Flush on the real writer.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Hijack is how ReverseProxy takes over the connection for WebSocket and
// other upgrades; it writes the 101 itself, so record it here.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.status = http.StatusSwitchingProtocols
	return http.NewResponseController(r.ResponseWriter).Hijack()
}
