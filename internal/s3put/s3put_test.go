package s3put

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPut(t *testing.T) {
	var got *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	tg := Target{Endpoint: srv.URL, Bucket: "releases", AccessKey: "AK", SecretKey: "SK"}
	if err := tg.Put("releases/0.7.1/DAWGit Pro+x.exe", []byte("installer"), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if got.Method != "PUT" || got.URL.EscapedPath() != "/releases/releases/0.7.1/DAWGit%20Pro%2Bx.exe" || body != "installer" {
		t.Fatalf("request: %s %s %q", got.Method, got.URL.EscapedPath(), body)
	}
	auth := got.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AK/") || !strings.Contains(auth, "/auto/s3/aws4_request") ||
		got.Header.Get("x-amz-content-sha256") == "" {
		t.Fatalf("signature headers: %q", auth)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<Error><Code>AccessDenied</Code></Error>", http.StatusForbidden)
	}))
	defer failing.Close()
	tg.Endpoint = failing.URL
	if err := tg.Put("x", []byte("y"), "text/plain"); err == nil || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("refused upload: %v", err)
	}
}
