package protect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
)

func ok(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func message(t *testing.T, rec *httptest.ResponseRecorder) string {
	var reply struct {
		Value struct {
			Message string `json:"message"`
		} `json:"value"`
	}
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&reply))
	return reply.Value.Message
}

func TestTryNoWaitWhenFull(t *testing.T) {
	q := New(1, false)
	q.limit <- struct{}{}
	r := httptest.NewRequest(http.MethodPost, "/session", nil)
	r.Header.Set("X-Selenoid-No-Wait", "")
	rec := httptest.NewRecorder()
	q.Try(ok)(rec, r)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "Too Many Requests", message(t, rec), "Ggr recognizes busy hosts by this message")

	rec = httptest.NewRecorder()
	q.Try(ok)(rec, httptest.NewRequest(http.MethodPost, "/session", nil))
	assert.Equal(t, http.StatusOK, rec.Code, "without the header the request goes on to the queue")
}

func TestCheckDisabledQueue(t *testing.T) {
	q := New(1, true)
	q.limit <- struct{}{}
	rec := httptest.NewRecorder()
	q.Check(ok)(rec, httptest.NewRequest(http.MethodPost, "/session", nil))
	assert.Equal(t, "queue is full", message(t, rec))
}

func TestProtectWaitsForFreeSlot(t *testing.T) {
	q := New(1, false)
	q.Protect(ok)(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/session", nil))
	q.Create()
	assert.Equal(t, 1, q.Used())

	done := make(chan int)
	go func() {
		rec := httptest.NewRecorder()
		q.Protect(ok)(rec, httptest.NewRequest(http.MethodPost, "/session", nil))
		done <- rec.Code
	}()
	assert.Eventually(t, func() bool { return q.Queued() == 1 }, time.Second, 5*time.Millisecond)
	select {
	case <-done:
		t.Fatal("the request must wait while the limit is reached")
	case <-time.After(50 * time.Millisecond):
	}
	q.Release()
	assert.Equal(t, http.StatusOK, <-done)
	assert.Equal(t, 1, q.Pending())
	q.Drop()
	assert.Equal(t, 0, q.Pending())
	assert.Equal(t, 0, q.Queued())
}

func TestProtectClientGoesAway(t *testing.T) {
	q := New(1, false)
	q.limit <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	called := false
	go func() {
		q.Protect(func(http.ResponseWriter, *http.Request) { called = true })(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/session", nil).WithContext(ctx))
		close(done)
	}()
	assert.Eventually(t, func() bool { return q.Queued() == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	<-done
	assert.False(t, called)
	assert.Equal(t, 0, q.Queued())
	assert.Equal(t, 0, q.Pending())
}
