package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestWaitDoesNotLeakOnHangingService(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	release := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))

	start := time.Now()
	err := wait(hanging.URL, 200*time.Millisecond)
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)

	// Only the server goroutines may remain: release them and check that
	// wait() left nothing behind.
	close(release)
	hanging.Close()
	goleak.VerifyNone(t, ignore)
}

func TestWaitSucceedsWhenServiceStarts(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	u := "http://" + s.Listener.Addr().String()
	go func() {
		time.Sleep(150 * time.Millisecond)
		s.Start()
	}()
	assert.NoError(t, wait(u, 5*time.Second))
	s.Close()
}

func TestWaitTimesOutWhenNothingListens(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	s := httptest.NewServer(http.NotFoundHandler())
	u := s.URL
	s.Close()
	assert.Error(t, wait(u, 200*time.Millisecond))
}
