package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Renaming a team on a server from before renaming existed says so plainly.
func TestSetInfoOnOldServer(t *testing.T) {
	for _, withInfo := range []bool{false, true} { // 0.1: no /info; early 0.2: GET only
		mux := http.NewServeMux()
		if withInfo {
			mux.HandleFunc("GET /api/v1/info", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) })
		}
		srv := httptest.NewServer(mux)
		err := New(srv.URL, "tok").SetInfo(TeamInfo{Name: "x"})
		srv.Close()
		if !errors.Is(err, ErrOldServer) {
			t.Errorf("withInfo=%v: err = %v, want ErrOldServer", withInfo, err)
		}
	}
}
