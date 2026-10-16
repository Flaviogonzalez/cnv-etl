package logger

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Setup configures the global logger. When debug is true it writes to both
// stdout and a timestamped file under logsDir. Returns a close func.
func Setup(debug bool, logsDir string) (func(), error) {
	if !debug {
		return func() {}, nil
	}

	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir logs: %w", err)
	}

	name := filepath.Join(logsDir, time.Now().Format("20060102_150405")+".log")
	f, err := os.Create(name)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.Printf("debug log → %s", name)

	return func() { f.Close() }, nil
}

// Transport wraps an http.RoundTripper and logs every request/response
// when debug mode is active.
type Transport struct {
	Base  http.RoundTripper
	Debug bool
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.Debug {
		if t.Base != nil {
			return t.Base.RoundTrip(req)
		}
		return http.DefaultTransport.RoundTrip(req)
	}

	start := time.Now()
	log.Printf("→ %s %s", req.Method, req.URL)

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		log.Printf("← ERR %s (%s): %v", req.Method, req.URL.Host, err)
		return nil, err
	}

	log.Printf("← %d %s %s [%s]", resp.StatusCode, req.Method, req.URL.Path, time.Since(start).Round(time.Millisecond))
	return resp, nil
}
