package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	assert "github.com/stretchr/testify/require"
)

func zipOf(t *testing.T, name, content string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	assert.NoError(t, err)
	_, err = f.Write([]byte(content))
	assert.NoError(t, err)
	assert.NoError(t, zw.Close())
	body, err := json.Marshal(map[string][]byte{"file": buf.Bytes()})
	assert.NoError(t, err)
	return body
}

func newTestSession(t *testing.T) string {
	manager = &HTTPTest{Handler: Selenium()}
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))
	id := sess["sessionId"]
	t.Cleanup(func() {
		// The session may be gone already (deleted or timed out); releasing
		// its queue slot twice would block forever.
		if _, ok := sessions.Get(id); ok {
			sessions.Remove(id)
			queue.Release()
		}
		_ = os.RemoveAll(filepath.Join(os.TempDir(), id))
	})
	return id
}

func uploadFile(t *testing.T, id string, body []byte) (int, string) {
	resp, err := http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", id)), "", bytes.NewReader(body))
	assert.NoError(t, err)
	defer resp.Body.Close()
	var reply map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&reply)
	path, _ := reply["value"].(string)
	return resp.StatusCode, path
}

func TestFileUploadZipSlip(t *testing.T) {
	id := newTestSession(t)
	sessionDir := filepath.Join(os.TempDir(), id)
	name := "selenoid-zip-slip-" + id + ".txt"
	escaped := filepath.Join(os.TempDir(), name)
	t.Cleanup(func() { _ = os.Remove(escaped) })

	code, path := uploadFile(t, id, zipOf(t, "../"+name, "pwned"))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, filepath.Join(sessionDir, name), path)
	_, err := os.Stat(escaped)
	assert.True(t, os.IsNotExist(err), "zip entry escaped the session directory")

	code, _ = uploadFile(t, id, zipOf(t, "dir/..", "x"))
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestFileUploadOverwritesShorterFile(t *testing.T) {
	id := newTestSession(t)
	_, _ = uploadFile(t, id, zipOf(t, "a.txt", "a much longer content"))
	code, path := uploadFile(t, id, zipOf(t, "a.txt", "short"))
	assert.Equal(t, http.StatusOK, code)
	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "short", string(data))
}

func TestFileEndpointRejectsForeignDirectory(t *testing.T) {
	target := t.TempDir()
	for _, dir := range []string{target, filepath.Join(os.TempDir(), "no-such-session"), ""} {
		req, _ := http.NewRequest(http.MethodPost, With(srv.URL).Path("/file"), bytes.NewReader(zipOf(t, "evil.txt", "pwned")))
		req.Header.Set("X-Selenoid-File", dir)
		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, dir)
	}
	_, err := os.Stat(filepath.Join(target, "evil.txt"))
	assert.True(t, os.IsNotExist(err), "/file must not write outside session directories")
}
