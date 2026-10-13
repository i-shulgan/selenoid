package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
)

func TestConcurrentRequestsResetTimeoutSafely(t *testing.T) {
	// Tests use a 2s idle timeout; 90 requests under -race on a slow runner
	// can take longer, and the session must not time out in the middle.
	setGlobal(t, &timeout, time.Minute)
	id := newTestSession(t)
	urls := []string{
		With(srv.URL).Path(fmt.Sprintf("/clipboard/%s", id)),
		With(srv.URL).Path(fmt.Sprintf("/devtools/%s/json/version", id)),
		With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url", id)),
	}
	var wg sync.WaitGroup
	errs := make(chan error, 90)
	for i := 0; i < 90; i++ {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			resp, err := http.Get(u)
			if err != nil {
				errs <- err
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}(urls[i%len(urls)])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("request failed (handler panicked?): %v", err)
	}
	sess, ok := sessions.Get(id)
	assert.True(t, ok)
	sess.Lock.Lock()
	ch := sess.TimeoutCh
	sess.Lock.Unlock()
	select {
	case <-ch:
		t.Fatal("the idle timer of an active session must be armed")
	default:
	}
}
