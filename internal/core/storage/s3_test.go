package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3ConfigValidation covers the configuration contract.
func TestS3ConfigValidation(t *testing.T) {
	_, err := NewS3(S3Config{Bucket: "b"})
	assert.Error(t, err, "missing endpoint must be rejected")

	_, err = NewS3(S3Config{Endpoint: "e"})
	assert.Error(t, err, "missing bucket must be rejected")

	_, err = NewS3(S3Config{Endpoint: "e", Bucket: "b"})
	assert.NoError(t, err, "valid config should be accepted")
}

// fakeS3 serves a minimal S3 API surface over HTTP: PUT, GET, HEAD and
// DELETE on /<bucket>/<key>. It answers without validating the
// signature, which is enough to exercise the client path.
func fakeS3(t *testing.T, body string) (*httptest.Server, *S3) {
	t.Helper()
	var lastReq string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastReq = r.Method + " " + r.URL.Path + " host=" + r.Host
		defer func() { t.Log("fakeS3 request:", lastReq) }()
		switch r.Method {
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("ETag", `"faketag"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.URL.Query().Has("list-type") || r.URL.Path == "/bucket/" || r.URL.Path == "/bucket" {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>bucket</Name><Prefix></Prefix><KeyCount>0</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated></ListBucketResult>`))
				return
			}
			if strings.Contains(r.URL.Path, "missing.txt") {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`))
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("ETag", `"faketag"`)
			w.Header().Set("Last-Modified", "Mon, 10 Aug 2026 17:00:00 GMT")
			_, _ = w.Write([]byte(body))
		case http.MethodHead:
			if strings.Contains(r.URL.Path, "missing.txt") {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", "5")
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("ETag", `"faketag"`)
			w.Header().Set("Last-Modified", "Mon, 10 Aug 2026 17:00:00 GMT")
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	host := strings.TrimPrefix(server.URL, "http://")
	s, err := NewS3(S3Config{
		Endpoint: host, Bucket: "bucket", Region: "us-east-1", Secure: false, PathStyle: true,
	})
	require.NoError(t, err)
	return server, s
}

func TestS3PutGetStatDelete(t *testing.T) {
	_, s := fakeS3(t, "hello")
	ctx := context.Background()

	info, err := s.Put(ctx, "patients/p1/doc.txt", strings.NewReader("hello"), 5, "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "patients/p1/doc.txt", info.Key)
	assert.Equal(t, blake2b256Hex("hello"), info.Checksum)

	obj, err := s.Get(ctx, "patients/p1/doc.txt")
	require.NoError(t, err)
	defer obj.Data.Close()
	got, err := io.ReadAll(obj.Data)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))

	st, err := s.Stat(ctx, "patients/p1/doc.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(5), st.Size)

	err = s.Delete(ctx, "patients/p1/doc.txt")
	require.NoError(t, err)

	// Missing object
	_, err = s.Get(ctx, "patients/p1/missing.txt")
	assert.Error(t, err)
}

// TestS3RejectsTraversal ensures key validation happens before any
// network call.
func TestS3RejectsTraversal(t *testing.T) {
	_, s := fakeS3(t, "")
	ctx := context.Background()
	for _, key := range []string{"../escape", "/abs", `a\b`} {
		_, err := s.Put(ctx, key, strings.NewReader("x"), 1, "")
		assert.Error(t, err, "Put(%q) should fail", key)

		err = s.Delete(ctx, key)
		assert.Error(t, err, "Delete(%q) should fail", key)
	}
}

func TestS3VerifyAndList(t *testing.T) {
	_, s := fakeS3(t, "")
	ctx := context.Background()

	// Verify bucket
	require.NoError(t, s.Verify(ctx))

	// Valid List
	objs, err := s.List(ctx, "patients/p1")
	require.NoError(t, err)
	assert.Empty(t, objs)
}

var _ Store = (*S3)(nil)

