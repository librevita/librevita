package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestS3ConfigValidation covers the configuration contract.
func TestS3ConfigValidation(t *testing.T) {
	if _, err := NewS3(S3Config{Bucket: "b"}); err == nil {
		t.Error("missing endpoint must be rejected")
	}
	if _, err := NewS3(S3Config{Endpoint: "e"}); err == nil {
		t.Error("missing bucket must be rejected")
	}
	if _, err := NewS3(S3Config{Endpoint: "e", Bucket: "b"}); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
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
			io.Copy(io.Discard, r.Body)
			w.Header().Set("ETag", `"faketag"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("ETag", `"faketag"`)
			w.Header().Set("Last-Modified", "Mon, 10 Aug 2026 17:00:00 GMT")
			w.Write([]byte(body))
		case http.MethodHead:
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
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	return server, s
}

func TestS3PutGetStatDelete(t *testing.T) {
	_, s := fakeS3(t, "hello")
	ctx := context.Background()

	info, err := s.Put(ctx, "patients/p1/doc.txt", strings.NewReader("hello"), 5, "text/plain")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Key != "patients/p1/doc.txt" {
		t.Errorf("Put info = %+v", info)
	}
	if info.Checksum != blake2b256Hex("hello") {
		t.Errorf("checksum = %q, want the canonical digest of hello", info.Checksum)
	}

	obj, err := s.Get(ctx, "patients/p1/doc.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer obj.Data.Close()
	got, _ := io.ReadAll(obj.Data)
	if string(got) != "hello" {
		t.Errorf("Get body = %q, want hello", got)
	}

	st, err := s.Stat(ctx, "patients/p1/doc.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if st.Size != 5 {
		t.Errorf("Stat size = %d, want 5", st.Size)
	}

	if err := s.Delete(ctx, "patients/p1/doc.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// TestS3RejectsTraversal ensures key validation happens before any
// network call.
func TestS3RejectsTraversal(t *testing.T) {
	_, s := fakeS3(t, "")
	ctx := context.Background()
	for _, key := range []string{"../escape", "/abs", `a\b`} {
		if _, err := s.Put(ctx, key, strings.NewReader("x"), 1, ""); err == nil {
			t.Errorf("Put(%q) = nil, want error", key)
		}
		if err := s.Delete(ctx, key); err == nil {
			t.Errorf("Delete(%q) = nil, want error", key)
		}
	}
}

var _ Store = (*S3)(nil)
