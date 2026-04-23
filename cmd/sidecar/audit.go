package main

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// auditEntry is the single-line JSON payload after each handled request.
type auditEntry struct {
	Time    string `json:"time"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Host    string `json:"host,omitempty"` // for proxy requests
	Status  int    `json:"status"`
	Bytes   int    `json:"bytes"`
	Latency int    `json:"latency_ms"`
}

func newAuditMiddleware(w io.Writer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: rw, status: 200}
			next.ServeHTTP(recorder, r)
			entry := auditEntry{
				Time:    start.UTC().Format(time.RFC3339),
				Method:  r.Method,
				Path:    r.URL.Path,
				Host:    r.URL.Host, // empty for non-proxy requests
				Status:  recorder.status,
				Bytes:   recorder.bytes,
				Latency: int(time.Since(start) / time.Millisecond),
			}
			b, _ := json.Marshal(entry)
			io.WriteString(w, "AUDIT ")
			w.Write(b)
			io.WriteString(w, "\n")
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(c int) {
	s.status = c
	s.ResponseWriter.WriteHeader(c)
}
func (s *statusRecorder) Write(b []byte) (int, error) {
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}
