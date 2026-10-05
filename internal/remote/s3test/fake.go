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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type object struct {
	data     []byte
	etag     string
	modified time.Time
}

type Server struct {
	*httptest.Server
	mu      sync.Mutex
	buckets map[string]map[string]*object
	// PageSize limits list results per page (small values test pagination).
	PageSize int
	// Requests counts requests by method, e.g. to check polling cost;
	// "LIST" counts listings (also counted as GET).
	Requests map[string]int
	// PutBytes and GetBytes count the bytes of objects written and read
	// (what a share uploads, a download downloads).
	PutBytes, GetBytes int64
	// IgnoreConditions acts like storage without conditional writes.
	IgnoreConditions bool
	// FailPart makes uploading that part number of a multipart upload fail.
	FailPart int
	// Flaky: every Flaky-th request fails with 500 InternalError, as storage
	// does now and then (0: never).
	Flaky      int
	flaky      int
	lastFailed string
	// CutAfter: after this many writes (PUT, POST, DELETE) every request
	// fails, for good (as if the computer lost its connection or DAWGit
	// stopped mid-way); 0: never. Writes counts them.
	CutAfter, Writes int
	// WriteLog lists the writes in order ("PUT key"), for tests to read.
	WriteLog []string
	// Clock is when objects are written (time.Now when nil): tests set it
	// to make objects old.
	Clock func() time.Time

	uploads map[string]*upload // multipart uploads in progress
	nextID  int
}

func New(buckets ...string) *Server {
	s := &Server{buckets: map[string]map[string]*object{}, PageSize: 1000, Requests: map[string]int{}}
	for _, b := range buckets {
		s.buckets[b] = map[string]*object{}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *Server) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
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
	if r.Method == "PUT" || r.Method == "POST" || r.Method == "DELETE" {
		s.Writes++
		s.WriteLog = append(s.WriteLog, r.Method+" "+r.URL.Path)
	}
	if s.CutAfter > 0 && s.Writes > s.CutAfter {
		io.Copy(io.Discard, r.Body)
		xmlError(w, http.StatusBadRequest, "ConnectionCut")
		return
	}
	if s.Flaky > 0 {
		// Never the same request twice running: requests retried in step
		// would otherwise keep landing on the failing turn.
		again := r.Method + " " + r.URL.String()
		if s.flaky++; s.flaky%s.Flaky == 0 && s.lastFailed != again {
			s.lastFailed = again
			io.Copy(io.Discard, r.Body)
			xmlError(w, http.StatusInternalServerError, "InternalError")
			return
		}
	}
	if r.Method == "GET" && r.URL.Query().Get("list-type") != "" {
		s.Requests["LIST"]++ // billed apart from reads
	}
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
	q := r.URL.Query()
	switch {
	case r.Method == "POST" && q.Has("uploads"):
		s.startUpload(w, parts[0], key)
		return
	case r.Method == "PUT" && q.Get("uploadId") != "":
		s.putPart(w, r, q)
		return
	case r.Method == "POST" && q.Get("uploadId") != "":
		s.completeUpload(w, r, bucket, key, q.Get("uploadId"))
		return
	case r.Method == "DELETE" && q.Get("uploadId") != "":
		delete(s.uploads, q.Get("uploadId"))
		w.WriteHeader(http.StatusNoContent)
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
			s.GetBytes += int64(len(obj.data))
			w.Write(obj.data)
		}
	case "PUT":
		data, _ := io.ReadAll(r.Body)
		// Copying within storage: the body is the source object's.
		if src := r.Header.Get("x-amz-copy-source"); src != "" {
			p, _ := url.PathUnescape(strings.TrimPrefix(src, "/"))
			from := strings.SplitN(p, "/", 2)
			var so *object
			if len(from) == 2 {
				so = s.buckets[from[0]][from[1]]
			}
			if so == nil {
				xmlError(w, http.StatusNotFound, "NoSuchKey")
				return
			}
			o := &object{data: append([]byte(nil), so.data...), etag: so.etag, modified: s.now()}
			bucket[key] = o
			w.Header().Set("ETag", o.etag)
			fmt.Fprintf(w, "<CopyObjectResult><ETag>%s</ETag></CopyObjectResult>", o.etag)
			return
		}
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
		s.PutBytes += int64(len(data))
		sum := md5.Sum(data)
		o := &object{data: data, etag: `"` + hex.EncodeToString(sum[:]) + `"`, modified: s.now()}
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

type upload struct {
	bucket, key string
	parts       map[int][]byte
}

// Uploads is how many multipart uploads are started and not finished or
// aborted.
func (s *Server) Uploads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.uploads)
}

func (s *Server) startUpload(w http.ResponseWriter, bucket, key string) {
	if s.uploads == nil {
		s.uploads = map[string]*upload{}
	}
	s.nextID++
	id := fmt.Sprintf("up%d", s.nextID)
	s.uploads[id] = &upload{bucket: bucket, key: key, parts: map[int][]byte{}}
	s.Requests["MULTIPART"]++
	fmt.Fprintf(w, "<InitiateMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>",
		bucket, key, id)
}

func (s *Server) putPart(w http.ResponseWriter, r *http.Request, q map[string][]string) {
	u := s.uploads[first(q["uploadId"])]
	n, err := strconv.Atoi(first(q["partNumber"]))
	if u == nil || err != nil || n < 1 {
		xmlError(w, http.StatusNotFound, "NoSuchUpload")
		return
	}
	if n == s.FailPart {
		xmlError(w, http.StatusInternalServerError, "InternalError")
		return
	}
	data, _ := io.ReadAll(r.Body)
	u.parts[n] = data
	sum := md5.Sum(data)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request, bucket map[string]*object, key, id string) {
	u := s.uploads[id]
	if u == nil || u.key != key {
		xmlError(w, http.StatusNotFound, "NoSuchUpload")
		return
	}
	var req struct {
		Parts []struct {
			PartNumber int
			ETag       string
		} `xml:"Part"`
	}
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Parts) == 0 {
		xmlError(w, http.StatusBadRequest, "MalformedXML")
		return
	}
	var data []byte
	for i, p := range req.Parts {
		part, ok := u.parts[p.PartNumber]
		sum := md5.Sum(part)
		if !ok || p.PartNumber != i+1 || p.ETag != `"`+hex.EncodeToString(sum[:])+`"` {
			xmlError(w, http.StatusBadRequest, "InvalidPart")
			return
		}
		data = append(data, part...)
	}
	sum := md5.Sum(data)
	bucket[key] = &object{data: data, etag: `"` + hex.EncodeToString(sum[:]) + `-multipart"`, modified: s.now()}
	delete(s.uploads, id)
	fmt.Fprintf(w, "<CompleteMultipartUploadResult><Key>%s</Key></CompleteMultipartUploadResult>", key)
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
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
	} else if after := q.Get("start-after"); after != "" {
		start = sort.SearchStrings(entries, after)
		if start < len(entries) && entries[start] == after {
			start++
		}
	}
	end := min(len(entries), start+s.PageSize)
	type content struct {
		Key          string
		Size         int
		LastModified string
	}
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
			o := bucket[e]
			res.Contents = append(res.Contents, content{e, len(o.data), o.modified.UTC().Format(time.RFC3339Nano)})
		}
	}
	if res.IsTruncated {
		res.NextContinuationToken = entries[end]
	}
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(res)
}

// Object is what is stored at key in bucket (tests look at the bytes).
func (s *Server) Object(bucket, key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.buckets[bucket][key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), o.data...), true
}

// Put writes key in bucket directly (tests set up records as another
// DAWGit would have written them).
func (s *Server) Put(bucket, key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sum := md5.Sum(data)
	s.buckets[bucket][key] = &object{data: append([]byte(nil), data...), etag: `"` + hex.EncodeToString(sum[:]) + `"`, modified: s.now()}
}

// Delete removes key from bucket (tests lose objects on purpose).
func (s *Server) Delete(bucket, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buckets[bucket], key)
}

// Keys lists the keys in bucket under prefix.
func (s *Server) Keys(bucket, prefix string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.buckets[bucket] {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
