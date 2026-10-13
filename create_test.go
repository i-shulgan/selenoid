package main

import (
	"bytes"
	"net/http"
	"sync"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
)

func TestRetryKeepsSessionPath(t *testing.T) {
	setGlobal(t, &retryCount, 3)
	setGlobal(t, &newSessionAttemptTimeout, 300*time.Millisecond)
	var (
		mu    sync.Mutex
		paths []string
	)
	manager = &HTTPTest{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		n := len(paths)
		mu.Unlock()
		switch {
		case r.URL.Path != "/wd/hub/session":
			// Selenium server style driver: only /wd/hub is served.
			w.WriteHeader(http.StatusNotFound)
		case n == 2:
			// The first attempt on the right path times out.
			<-time.After(time.Second)
		default:
			_, _ = w.Write([]byte(`{"value":{"sessionId":"123","capabilities":{}}}`))
		}
	})}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	mu.Lock()
	assert.Equal(t, []string{"/session", "/wd/hub/session", "/wd/hub/session"}, paths)
	mu.Unlock()
	sessions.Remove("123")
	queue.Release()
}
