package main

import (
	"bytes"
	"encoding/json"
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

func createWithDriverReply(t *testing.T, status int, body string) (*http.Response, map[string]interface{}) {
	manager = &HTTPTest{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})}
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	defer resp.Body.Close()
	var reply map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&reply)
	return resp, reply
}

func errorMessage(reply map[string]interface{}) string {
	value, _ := reply["value"].(map[string]interface{})
	msg, _ := value["message"].(string)
	return msg
}

func TestCreateNonStringSessionID(t *testing.T) {
	resp, reply := createWithDriverReply(t, http.StatusOK, `{"value":{"sessionId":42,"capabilities":{}}}`)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Contains(t, errorMessage(reply), "no session ID")
	assert.Equal(t, 0, queue.Pending(), "a failed request must free its queue slot")
	assert.Equal(t, 0, queue.Used())
}

func TestCreateW3CWithoutCapabilities(t *testing.T) {
	resp, reply := createWithDriverReply(t, http.StatusOK, `{"value":{"sessionId":"no-caps"}}`)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	value, _ := reply["value"].(map[string]interface{})
	assert.Equal(t, "no-caps", value["sessionId"])
	_, ok := sessions.Get("no-caps")
	assert.True(t, ok, "the session the client got must be registered")
	sessions.Remove("no-caps")
	queue.Release()
}

func TestCreateNonJSONResponse(t *testing.T) {
	resp, reply := createWithDriverReply(t, http.StatusBadGateway, `<html>502 Bad Gateway</html>`)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode, "the status of the driver is kept")
	assert.Equal(t, "session not created", reply["value"].(map[string]interface{})["error"])
	assert.Contains(t, errorMessage(reply), "502 Bad Gateway")
	assert.Equal(t, 0, queue.Pending())
}

func TestCreateEmptySuccessfulResponse(t *testing.T) {
	resp, reply := createWithDriverReply(t, http.StatusOK, ``)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Contains(t, errorMessage(reply), "unexpected response")
	assert.Equal(t, 0, queue.Pending())
}

func TestCreateDriverErrorIsForwarded(t *testing.T) {
	resp, reply := createWithDriverReply(t, http.StatusInternalServerError, `{"value":{"error":"session not created","message":"Chrome failed to start"}}`)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "Chrome failed to start", errorMessage(reply))
	assert.Equal(t, 0, queue.Pending())
}
