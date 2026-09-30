// Package s3test is an in-memory S3-compatible server for tests: path-style
// buckets, GET/HEAD/PUT/DELETE, conditional writes (If-Match, If-None-Match),
// ETags, and paginated ListObjectsV2. It checks that requests are signed and
// that signed payload hashes match the body; it does not verify signatures.
package s3test

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type object struct {
	data []byte
	etag string
}

type Server struct {
	*httptest.Server
	mu      sync.Mutex
	buckets map[string]map[string]*object
	// PageSize limits list results per page (small values test pagination).
	PageSize int
	// Requests counts requests by method, e.g. to check polling cost.
	Requests map[string]int
	// IgnoreConditions acts like storage without conditional writes.
	IgnoreConditions bool
}

func New(buckets ...string) *Server {
	s := &Server{buckets: map[string]map[string]*object{}, PageSize: 1000, Requests: map[string]int{}}
	for _, b := range buckets {
		s.buckets[b] = map[string]*object{}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func xmlError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, "<Error><Code>%s</Code><Message>%s</Message></Error>", code, code)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=") {
		xmlError(w, http.StatusForbidden, "AccessDenied")
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests[r.Method]++
	bucket, ok := s.buckets[parts[0]]
	if !ok {
		xmlError(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}
	if key == "" && r.Method == "GET" && r.URL.Query().Get("list-type") == "2" {
		s.list(w, r, bucket)
		return
	}
	obj := bucket[key]
	switch r.Method {
	case "GET", "HEAD":
		if obj == nil {
			xmlError(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		w.Header().Set("ETag", obj.etag)
		w.Header().Set("Content-Length", strconv.Itoa(len(obj.data)))
		if r.Method == "GET" {
			w.Write(obj.data)
		}
	case "PUT":
		data, _ := io.ReadAll(r.Body)
		if h := r.Header.Get("x-amz-content-sha256"); h != "UNSIGNED-PAYLOAD" {
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != h {
				xmlError(w, http.StatusBadRequest, "XAmzContentSHA256Mismatch")
				return
			}
		}
		if !s.IgnoreConditions && !preconditions(w, r, obj) {
			return
		}
		sum := md5.Sum(data)
		o := &object{data: data, etag: `"` + hex.EncodeToString(sum[:]) + `"`}
		bucket[key] = o
		w.Header().Set("ETag", o.etag)
		w.WriteHeader(http.StatusOK)
	case "DELETE":
		if !s.IgnoreConditions && !preconditions(w, r, obj) {
			return
		}
		delete(bucket, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		xmlError(w, http.StatusMethodNotAllowed, "MethodNotAllowed")
	}
}

func preconditions(w http.ResponseWriter, r *http.Request, obj *object) bool {
	if r.Header.Get("If-None-Match") == "*" && obj != nil {
		xmlError(w, http.StatusPreconditionFailed, "PreconditionFailed")
		return false
	}
	if m := r.Header.Get("If-Match"); m != "" && (obj == nil || obj.etag != m) {
		xmlError(w, http.StatusPreconditionFailed, "PreconditionFailed")
		return false
	}
	return true
}

func (s *Server) list(w http.ResponseWriter, r *http.Request, bucket map[string]*object) {
	q := r.URL.Query()
	prefix, delim, token := q.Get("prefix"), q.Get("delimiter"), q.Get("continuation-token")
	var entries []string        // keys and common prefixes, sorted together
	common := map[string]bool{} // entries that are common prefixes
	seen := map[string]bool{}
	for k := range bucket {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		e := k
		if delim != "" {
			if i := strings.Index(k[len(prefix):], delim); i >= 0 {
				e = k[:len(prefix)+i+len(delim)]
				common[e] = true
			}
		}
		if !seen[e] {
			seen[e] = true
			entries = append(entries, e)
		}
	}
	sort.Strings(entries)
	start := 0
	if token != "" {
		start = sort.SearchStrings(entries, token)
	}
	end := min(len(entries), start+s.PageSize)
	type content struct{ Key string }
	type cp struct{ Prefix string }
	res := struct {
		XMLName               xml.Name  `xml:"ListBucketResult"`
		Contents              []content `xml:"Contents"`
		CommonPrefixes        []cp      `xml:"CommonPrefixes"`
		IsTruncated           bool
		NextContinuationToken string `xml:",omitempty"`
	}{IsTruncated: end < len(entries)}
	for _, e := range entries[start:end] {
		if common[e] {
			res.CommonPrefixes = append(res.CommonPrefixes, cp{e})
		} else {
			res.Contents = append(res.Contents, content{e})
		}
	}
	if res.IsTruncated {
		res.NextContinuationToken = entries[end]
	}
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(res)
}
