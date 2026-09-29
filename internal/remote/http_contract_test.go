package remote_test

import (
	"net/http/httptest"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/backendtest"
	"dawgit/internal/server"
)

func TestHTTPBackendContract(t *testing.T) {
	st, err := server.OpenStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Handler(st, "tok"))
	defer srv.Close()
	b, err := remote.Open(remote.Config{URL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	backendtest.Run(t, b)
}
