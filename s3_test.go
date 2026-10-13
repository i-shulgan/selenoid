//go:build s3
// +build s3

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aerokube/selenoid/event"
	"github.com/aerokube/selenoid/session"
	"github.com/aerokube/selenoid/upload"
	assert "github.com/stretchr/testify/require"
)

var (
	s3Srv *httptest.Server
)

func init() {
	s3Srv = httptest.NewServer(s3Mux())
}

type s3Request struct {
	method, path, storageClass, contentType, body string
}

var s3Requests = make(chan s3Request, 10)

func s3Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s3Requests <- s3Request{r.Method, r.URL.Path, r.Header.Get("X-Amz-Storage-Class"), r.Header.Get("Content-Type"), string(body)}
	})
	return mux
}

// testS3Session returns a new session: tests modify its capabilities.
func testS3Session() *session.Session {
	return &session.Session{
		Quota: "some-user",
		Caps: session.Caps{
			Name:     "internet explorer",
			Version:  "11",
			Platform: "WINDOWS",
		},
	}
}

func TestS3Uploader(t *testing.T) {
	uploader := &upload.S3Uploader{
		Endpoint:          s3Srv.URL,
		ForcePathStyle:    true,
		Region:            "us-west-1",
		AccessKey:         "some-access-key",
		SecretKey:         "some-secret-key",
		BucketName:        "test-bucket",
		KeyPattern:        "$fileName",
		ReducedRedundancy: true,
	}
	uploader.Init()
	f, _ := os.CreateTemp("", "some-file*.log")
	_, _ = f.WriteString("log contents")
	_ = f.Close()
	input := event.CreatedFile{
		Event: event.Event{
			RequestId: 4342,
			SessionId: "some-session-id",
			Session:   testS3Session(),
		},
		Name: f.Name(),
		Type: "log",
	}
	uploaded, err := uploader.Upload(input)
	assert.NoError(t, err)
	assert.True(t, uploaded)

	req := <-s3Requests
	assert.Equal(t, http.MethodPut, req.method)
	assert.Equal(t, "/test-bucket/"+filepath.Base(f.Name()), req.path)
	assert.Equal(t, "REDUCED_REDUNDANCY", req.storageClass)
	assert.Equal(t, "log contents", req.body)
	_, err = os.Stat(f.Name())
	assert.True(t, os.IsNotExist(err), "uploaded file must be removed unless -s3-keep-files is set")

	input.Name = "/does/not/exist.log"
	_, err = uploader.Upload(input)
	assert.Error(t, err)
}

func TestGetKey(t *testing.T) {
	const testPattern = "$quota/$sessionId_$browserName_$browserVersion_$platformName/$fileType$fileExtension"
	input := event.CreatedFile{
		Event: event.Event{
			SessionId: "some-Session-id",
			Session:   testS3Session(),
			RequestId: 12345,
		},

		Name: "/path/to/Some-File.txt",
		Type: "log",
	}

	key := upload.GetS3Key(testPattern, input)
	assert.Equal(t, key, "some-user/some-Session-id_internet-explorer_11_windows/log.txt")

	input.Session.Caps.Name = ""
	input.Session.Caps.DeviceName = "internet explorer"
	key = upload.GetS3Key(testPattern, input)
	assert.Equal(t, key, "some-user/some-Session-id_internet-explorer_11_windows/log.txt")

	input.Session.Caps.S3KeyPattern = "$quota/$fileType$fileExtension"
	key = upload.GetS3Key(testPattern, input)
	assert.Equal(t, key, "some-user/log.txt")

	input.Session.Caps.S3KeyPattern = "$fileName"
	key = upload.GetS3Key(testPattern, input)
	assert.Equal(t, key, "Some-File.txt")
}

func TestFileMatches(t *testing.T) {
	matches, err := upload.FileMatches("", "", "any-file-name")
	assert.NoError(t, err)
	assert.True(t, matches)

	matches, err = upload.FileMatches("[", "", "/path/to/file.mp4")
	assert.Error(t, err)
	assert.False(t, matches)

	matches, err = upload.FileMatches("", "[", "/path/to/file.mp4")
	assert.Error(t, err)
	assert.False(t, matches)

	matches, err = upload.FileMatches("*.mp4", "", "/path/to/file.mp4")
	assert.NoError(t, err)
	assert.True(t, matches)

	matches, err = upload.FileMatches("*.mp4", "", "/path/to/file.log")
	assert.NoError(t, err)
	assert.False(t, matches)

	matches, err = upload.FileMatches("*.mp4", "", "/path/to/file.log")
	assert.NoError(t, err)
	assert.False(t, matches)

	matches, err = upload.FileMatches("", "*.log", "/path/to/file.log")
	assert.NoError(t, err)
	assert.False(t, matches)
}
