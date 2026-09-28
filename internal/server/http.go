package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// API (all JSON, Bearer token auth):
//
//	GET  /api/v1/projects                         -> [{id,name}]
//	PUT  /api/v1/projects/{pid}                   {name}
//	GET  /api/v1/projects/{pid}/branches          -> {name: snapshot}
//	POST /api/v1/projects/{pid}/branches/{name}   {old,new} -> 200 | 409 {current}
//	POST /api/v1/projects/{pid}/snapshots/missing {ids} -> {missing}
//	PUT  /api/v1/projects/{pid}/snapshots/{id}    manifest bytes
//	GET  /api/v1/projects/{pid}/snapshots/{id}
//	POST /api/v1/objects/missing                  {hashes} -> {missing}
//	PUT  /api/v1/objects/{hash}                   blob bytes
//	GET  /api/v1/objects/{hash}
func Handler(s *Storage, token string) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{s: s}
	mux.HandleFunc("GET /api/v1/projects", h.listProjects)
	mux.HandleFunc("PUT /api/v1/projects/{pid}", h.putProject)
	mux.HandleFunc("GET /api/v1/projects/{pid}/branches", h.project(h.getBranches))
	mux.HandleFunc("POST /api/v1/projects/{pid}/branches/{name}", h.project(h.updateBranch))
	mux.HandleFunc("POST /api/v1/projects/{pid}/snapshots/missing", h.project(h.missingSnapshots))
	mux.HandleFunc("PUT /api/v1/projects/{pid}/snapshots/{id}", h.project(h.putSnapshot))
	mux.HandleFunc("GET /api/v1/projects/{pid}/snapshots/{id}", h.project(h.getSnapshot))
	mux.HandleFunc("POST /api/v1/objects/missing", h.missingObjects)
	mux.HandleFunc("PUT /api/v1/objects/{hash}", h.putObject)
	mux.HandleFunc("GET /api/v1/objects/{hash}", h.getObject)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	return auth(token, mux)
}

func auth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.URL.Path != "/healthz" {
			got, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				httpError(w, http.StatusUnauthorized, "invalid or missing token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type handlers struct{ s *Storage }

func writeJSONResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, msg string) {
	writeJSONResponse(w, status, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(v); err != nil {
		httpError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return false
	}
	return true
}

// project wraps handlers that require an existing project.
func (h *handlers) project(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.s.HasProject(r.PathValue("pid")) {
			httpError(w, http.StatusNotFound, "unknown project")
			return
		}
		next(w, r)
	}
}

func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := h.s.Projects()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ps == nil {
		ps = []Project{}
	}
	writeJSONResponse(w, http.StatusOK, ps)
}

func (h *handlers) putProject(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name string }
	if !decode(w, r, &body) {
		return
	}
	if err := h.s.PutProject(Project{ID: r.PathValue("pid"), Name: body.Name}); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getBranches(w http.ResponseWriter, r *http.Request) {
	b, err := h.s.Branches(r.PathValue("pid"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, b)
}

func (h *handlers) updateBranch(w http.ResponseWriter, r *http.Request) {
	var body struct{ Old, New string }
	if !decode(w, r, &body) {
		return
	}
	err := h.s.UpdateBranch(r.PathValue("pid"), r.PathValue("name"), body.Old, body.New)
	var conflict *ErrConflict
	switch {
	case errors.As(err, &conflict):
		writeJSONResponse(w, http.StatusConflict, map[string]string{"error": err.Error(), "current": conflict.Current})
	case err != nil:
		httpError(w, http.StatusBadRequest, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handlers) missingSnapshots(w http.ResponseWriter, r *http.Request) {
	var body struct{ IDs []string }
	if !decode(w, r, &body) {
		return
	}
	missing := []string{}
	for _, id := range body.IDs {
		if !h.s.HasSnapshot(r.PathValue("pid"), id) {
			missing = append(missing, id)
		}
	}
	writeJSONResponse(w, http.StatusOK, map[string][]string{"missing": missing})
}

func (h *handlers) putSnapshot(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.s.PutSnapshot(r.PathValue("pid"), r.PathValue("id"), data); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getSnapshot(w http.ResponseWriter, r *http.Request) {
	data, err := h.s.GetSnapshot(r.PathValue("pid"), r.PathValue("id"))
	if errors.Is(err, os.ErrNotExist) {
		httpError(w, http.StatusNotFound, "unknown snapshot")
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (h *handlers) missingObjects(w http.ResponseWriter, r *http.Request) {
	var body struct{ Hashes []string }
	if !decode(w, r, &body) {
		return
	}
	missing := []string{}
	for _, hash := range body.Hashes {
		if !h.s.Objects.Has(hash) {
			missing = append(missing, hash)
		}
	}
	writeJSONResponse(w, http.StatusOK, map[string][]string{"missing": missing})
}

func (h *handlers) putObject(w http.ResponseWriter, r *http.Request) {
	want := r.PathValue("hash")
	if !idRE.MatchString(want) {
		httpError(w, http.StatusBadRequest, "invalid hash")
		return
	}
	got, _, err := h.s.Objects.Put(r.Body)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if got != want {
		// The blob is stored under its real hash; harmless, but reject the upload.
		httpError(w, http.StatusBadRequest, "content hash mismatch")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getObject(w http.ResponseWriter, r *http.Request) {
	rc, err := h.s.Objects.Open(r.PathValue("hash"))
	if err != nil {
		httpError(w, http.StatusNotFound, "unknown object")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := io.Copy(w, rc); err != nil {
		log.Printf("get object: %v", err)
	}
}

// NewToken returns a random access token.
func NewToken() string {
	b := make([]byte, 20)
	rand.Read(b)
	return hex.EncodeToString(b)
}
