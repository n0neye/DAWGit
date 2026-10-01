package remote

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// S3Backend stores a team's data directly in an S3-compatible bucket, with
// no DAWGit server (docs/design/storage-backends.md). Layout under prefix:
//
//	objects/<ab>/<cdef…>                  file contents
//	projects/<pid>/project.json
//	projects/<pid>/snapshots/<id>.json
//	projects/<pid>/branches/<name>        body = version id; updated with If-Match
//	projects/<pid>/workspaces/<wsid>.json
type S3Backend struct {
	endpoint *url.URL // scheme + host
	bucket   string
	prefix   string // "" or "team/" (ends with a slash)
	sig      *signer
	http     *http.Client
}

var _ Backend = (*S3Backend)(nil)

// NewS3 connects to bucket at endpoint (e.g. https://<account>.r2.cloudflarestorage.com)
// using path-style requests.
func NewS3(endpoint, bucket, prefix, region, accessKey, secretKey string) (*S3Backend, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid storage endpoint %q", endpoint)
	}
	if bucket == "" {
		return nil, errors.New("storage bucket is required")
	}
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	if region == "" {
		region = "auto"
	}
	return &S3Backend{
		endpoint: &url.URL{Scheme: u.Scheme, Host: u.Host},
		bucket:   bucket, prefix: prefix,
		sig:  &signer{accessKey: accessKey, secretKey: secretKey, region: region, now: time.Now},
		http: &http.Client{Timeout: 30 * time.Minute, Transport: keepAlive()},
	}, nil
}

// --- low level ---

type s3Response struct {
	status int
	etag   string
	body   []byte
}

// errS3 is returned for unexpected responses.
type errS3 struct {
	status int
	msg    string
}

func (e *errS3) Error() string { return fmt.Sprintf("storage error %d: %s", e.status, e.msg) }

func (b *S3Backend) keyURL(key string, q url.Values) *url.URL {
	u := *b.endpoint
	u.Path = "/" + b.bucket + "/" + b.prefix + key
	if key == "" {
		u.Path = "/" + b.bucket + "/"
	}
	u.RawQuery = q.Encode()
	return &u
}

// do sends a signed request. body may be nil; size < 0 means unknown.
// payloadHash is the hex SHA-256 of body (or unsignedPayload).
func (b *S3Backend) do(method, key string, q url.Values, body io.Reader, size int64, payloadHash string,
	header http.Header) (*http.Response, error) {
	req, err := http.NewRequest(method, b.keyURL(key, q).String(), body)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if body != nil {
		req.ContentLength = size
	}
	if payloadHash == "" {
		payloadHash = emptySHA256
	}
	b.sig.sign(req, payloadHash)
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach storage %s: %w", b.endpoint.Host, err)
	}
	return resp, nil
}

// call runs a request and reads the whole response.
func (b *S3Backend) call(method, key string, q url.Values, body []byte, header http.Header) (*s3Response, error) {
	var r io.Reader
	hash := emptySHA256
	if body != nil {
		r = bytes.NewReader(body)
		hash = sha256Hex(body)
	}
	resp, err := b.do(method, key, q, r, int64(len(body)), hash, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &s3Response{status: resp.StatusCode, etag: resp.Header.Get("ETag"), body: data}, nil
}

func s3Error(r *s3Response) error {
	var e struct {
		Code    string
		Message string
	}
	xml.Unmarshal(r.body, &e)
	msg := strings.TrimSpace(e.Code + " " + e.Message)
	switch r.status {
	case http.StatusForbidden, http.StatusUnauthorized:
		return errors.New("storage rejected the credentials (" + msg + ")")
	case http.StatusNotFound:
		if e.Code == "NoSuchBucket" {
			return errors.New("storage bucket not found")
		}
		return ErrNotFound
	}
	if msg == "" {
		msg = strings.TrimSpace(string(r.body))
	}
	return &errS3{r.status, msg}
}

func (b *S3Backend) get(key string) (*s3Response, error) {
	r, err := b.call("GET", key, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.status != http.StatusOK {
		return r, s3Error(r)
	}
	return r, nil
}

func (b *S3Backend) exists(key string) (bool, error) {
	r, err := b.call("HEAD", key, nil, nil, nil)
	if err != nil {
		return false, err
	}
	switch r.status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, s3Error(r)
}

// put writes key; ifMatch "*"-style preconditions go in header.
func (b *S3Backend) put(key string, body []byte, header http.Header) (*s3Response, error) {
	if header == nil {
		header = http.Header{}
	}
	return b.call("PUT", key, nil, body, header)
}

// list returns keys (relative to prefix) under dir; with delim it returns the
// immediate sub-"folders" instead.
func (b *S3Backend) list(dir string, delim bool) ([]string, error) {
	var out []string
	token := ""
	for {
		page, err := b.listPage(dir, delim, "", token)
		if err != nil {
			return nil, err
		}
		out = append(out, page.keys...)
		if page.next == "" {
			return out, nil
		}
		token = page.next
	}
}

type listing struct {
	keys []string // relative to prefix: keys, or sub-"folders" with delim
	next string   // continuation token; "" on the last page
}

// listPage returns one page (up to 1000 keys) under dir, after the key
// startAfter (relative to prefix; "" for the start) or from a continuation
// token.
func (b *S3Backend) listPage(dir string, delim bool, startAfter, token string) (listing, error) {
	q := url.Values{"list-type": {"2"}, "prefix": {b.prefix + dir}}
	if delim {
		q.Set("delimiter", "/")
	}
	if token != "" {
		q.Set("continuation-token", token)
	} else if startAfter != "" {
		q.Set("start-after", b.prefix+startAfter)
	}
	r, err := b.call("GET", "", q, nil, nil)
	if err != nil {
		return listing{}, err
	}
	if r.status != http.StatusOK {
		return listing{}, s3Error(r)
	}
	var res struct {
		Contents []struct{ Key string }
		Prefixes []struct {
			Prefix string
		} `xml:"CommonPrefixes"`
		IsTruncated           bool
		NextContinuationToken string
	}
	if err := xml.Unmarshal(r.body, &res); err != nil {
		return listing{}, fmt.Errorf("storage list: %w", err)
	}
	var out listing
	if delim {
		for _, p := range res.Prefixes {
			out.keys = append(out.keys, strings.TrimPrefix(p.Prefix, b.prefix))
		}
	} else {
		for _, c := range res.Contents {
			out.keys = append(out.keys, strings.TrimPrefix(c.Key, b.prefix))
		}
	}
	if res.IsTruncated {
		out.next = res.NextContinuationToken
	}
	return out, nil
}

// parallel runs fn over items with bounded concurrency, collecting errors.
func parallel(items []string, fn func(string) error) error { return parallelN(8, items, fn) }

// checks is how many existence checks (small requests) go at once.
const checks = 32

func parallelN(n int, items []string, fn func(string) error) error {
	sem := make(chan struct{}, n)
	var mu sync.Mutex
	var first error
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it string) {
			defer func() { <-sem; wg.Done() }()
			if err := fn(it); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}(it)
	}
	wg.Wait()
	return first
}

// --- keys ---

func objectKey(hash string) string       { return "objects/" + hash[:2] + "/" + hash[2:] }
func projectDir(pid string) string       { return "projects/" + pid + "/" }
func snapshotKey(pid, id string) string  { return projectDir(pid) + "snapshots/" + id + ".json" }
func branchKey(pid, name string) string  { return projectDir(pid) + "branches/" + name }
func workspaceKey(pid, ws string) string { return projectDir(pid) + "workspaces/" + ws + ".json" }

// memberKey: one object per member, so members never overwrite each other.
func memberKey(id string) string { return "members/" + id + ".json" }
func validHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func validBranch(name string) bool {
	if name == "" || len(name) > 64 || strings.HasPrefix(name, ".") {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// --- Backend ---

func (b *S3Backend) Projects() ([]Project, error) {
	dirs, err := b.list("projects/", true)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	out := []Project{}
	err = parallel(dirs, func(dir string) error {
		r, err := b.get(dir + "project.json")
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var p Project
		if json.Unmarshal(r.body, &p) == nil && p.ID != "" {
			mu.Lock()
			out = append(out, p)
			mu.Unlock()
		}
		return nil
	})
	return out, err
}

func (b *S3Backend) PutProject(p Project) error {
	if !validHex(p.ID, 32) {
		return errors.New("invalid project id")
	}
	data, _ := json.Marshal(p)
	r, err := b.put(projectDir(p.ID)+"project.json", data, nil)
	if err != nil {
		return err
	}
	if r.status != http.StatusOK {
		return s3Error(r)
	}
	return nil
}

// DeleteProject removes project.json first, so the project leaves the list
// even if deleting the rest is interrupted.
func (b *S3Backend) DeleteProject(pid string) error {
	if !validHex(pid, 32) {
		return errors.New("invalid project id")
	}
	if err := b.delete(projectDir(pid) + "project.json"); err != nil {
		return err
	}
	keys, err := b.list(projectDir(pid), false)
	if err != nil {
		return err
	}
	return parallel(keys, b.delete)
}

func (b *S3Backend) delete(key string) error {
	r, err := b.call("DELETE", key, nil, nil, nil)
	if err != nil {
		return err
	}
	if r.status != http.StatusNoContent && r.status != http.StatusOK && r.status != http.StatusNotFound {
		return s3Error(r)
	}
	return nil
}

func (b *S3Backend) branch(pid, name string) (id, etag string, err error) {
	r, err := b.get(branchKey(pid, name))
	if errors.Is(err, ErrNotFound) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(r.body)), r.etag, nil
}

// BranchHead reads one branch's head ("" if it doesn't exist): a single GET,
// which storage bills far less than the listing Branches needs.
func (b *S3Backend) BranchHead(pid, name string) (string, error) {
	if !validBranch(name) {
		return "", fmt.Errorf("invalid branch name %q", name)
	}
	id, _, err := b.branch(pid, name)
	return id, err
}

func (b *S3Backend) Branches(pid string) (map[string]string, error) {
	keys, err := b.list(projectDir(pid)+"branches/", false)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	out := map[string]string{}
	err = parallel(keys, func(key string) error {
		name := path.Base(key)
		id, _, err := b.branch(pid, name)
		if err == nil && id != "" {
			mu.Lock()
			out[name] = id
			mu.Unlock()
		}
		return err
	})
	return out, err
}

// UpdateBranch uses conditional writes: If-None-Match: * to create,
// If-Match: <etag> to move or delete.
func (b *S3Backend) UpdateBranch(pid, name, old, new string) error {
	if !validBranch(name) {
		return fmt.Errorf("invalid branch name %q", name)
	}
	current, etag, err := b.branch(pid, name)
	if err != nil {
		return err
	}
	if current != old {
		return &ErrConflict{Current: current}
	}
	conflict := func() error {
		cur, _, err := b.branch(pid, name)
		if err != nil {
			return err
		}
		return &ErrConflict{Current: cur}
	}
	h := http.Header{}
	if old == "" {
		h.Set("If-None-Match", "*")
	} else {
		h.Set("If-Match", etag)
	}
	var r *s3Response
	if new == "" {
		r, err = b.call("DELETE", branchKey(pid, name), nil, nil, h)
	} else {
		r, err = b.put(branchKey(pid, name), []byte(new+"\n"), h)
	}
	if err != nil {
		return err
	}
	switch r.status {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusPreconditionFailed, http.StatusConflict:
		return conflict()
	}
	return s3Error(r)
}

func (b *S3Backend) MissingSnapshots(pid string, ids []string) ([]string, error) {
	return b.missing(ids, func(id string) string { return snapshotKey(pid, id) })
}

func (b *S3Backend) missing(names []string, key func(string) string) ([]string, error) {
	var mu sync.Mutex
	missing := []string{}
	err := parallelN(checks, names, func(n string) error {
		if !validHex(n, 64) {
			return fmt.Errorf("invalid id %q", n)
		}
		ok, err := b.exists(key(n))
		if err == nil && !ok {
			mu.Lock()
			missing = append(missing, n)
			mu.Unlock()
		}
		return err
	})
	return missing, err
}

func (b *S3Backend) PutSnapshot(pid, id string, data []byte) error {
	if !validHex(id, 64) || sha256Hex(data) != id {
		return errors.New("version content does not match its id")
	}
	r, err := b.put(snapshotKey(pid, id), data, nil)
	if err != nil {
		return err
	}
	if r.status != http.StatusOK {
		return s3Error(r)
	}
	return nil
}

func (b *S3Backend) GetSnapshot(pid, id string) ([]byte, error) {
	if !validHex(id, 64) {
		return nil, ErrNotFound
	}
	r, err := b.get(snapshotKey(pid, id))
	if err != nil {
		return nil, err
	}
	if sha256Hex(r.body) != id {
		return nil, fmt.Errorf("version %s is corrupt in storage", id[:10])
	}
	return r.body, nil
}

// MissingObjects asks storage about each object, or, where many share a
// folder (objects/<ab>/), lists the folder: a first share asks about
// thousands of files, and one listing answers for up to 1000 of them.
func (b *S3Backend) MissingObjects(hashes []string) ([]string, error) {
	shards := map[string][]string{}
	for _, h := range hashes {
		if !validHex(h, 64) {
			return nil, fmt.Errorf("invalid id %q", h)
		}
		shards[h[:2]] = append(shards[h[:2]], h)
	}
	var mu sync.Mutex
	present := map[string]bool{}
	var ask []string // to ask about one by one
	var listed []string
	for shard, hs := range shards {
		if len(hs) >= listFrom {
			listed = append(listed, shard)
		} else {
			ask = append(ask, hs...)
		}
	}
	err := parallelN(checks, listed, func(shard string) error {
		want := slices.Clone(shards[shard])
		slices.Sort(want)
		dir := "objects/" + shard + "/"
		// From just before the first wanted object.
		first := objectKey(want[0])
		after, token := first[:len(first)-1], ""
		for {
			page, err := b.listPage(dir, false, after, token)
			if err != nil {
				return err
			}
			mu.Lock()
			for _, k := range page.keys {
				present[k] = true
			}
			mu.Unlock()
			if page.next == "" || len(page.keys) == 0 {
				return nil
			}
			// Wanted objects past this page: a few are quicker asked about.
			last := page.keys[len(page.keys)-1]
			var rest []string
			for _, h := range want {
				if objectKey(h) > last {
					rest = append(rest, h)
				}
			}
			if len(rest) == 0 {
				return nil
			}
			if len(rest) < listFrom {
				mu.Lock()
				ask = append(ask, rest...)
				mu.Unlock()
				return nil
			}
			token = page.next
		}
	})
	if err != nil {
		return nil, err
	}
	asked, err := b.missing(ask, objectKey)
	if err != nil {
		return nil, err
	}
	notAsked := map[string]bool{}
	for _, h := range ask {
		notAsked[h] = true
	}
	missing := asked
	for _, h := range dedupeHashes(hashes) {
		if !notAsked[h] && !present[objectKey(h)] {
			missing = append(missing, h)
		}
	}
	return missing, nil
}

// listFrom: a folder holding this many of the wanted objects is listed.
const listFrom = 4

func dedupeHashes(hs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range hs {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// PutObject uploads a blob. Its name is the SHA-256 of its contents, which
// is also the signed payload hash, so storage verifies the upload.
func (b *S3Backend) PutObject(hash string, r io.Reader) error {
	if !validHex(hash, 64) {
		return fmt.Errorf("invalid object hash %q", hash)
	}
	size := int64(-1)
	switch v := r.(type) {
	case *os.File:
		if fi, err := v.Stat(); err == nil {
			size = fi.Size()
		}
	case interface{ Len() int }:
		size = int64(v.Len())
	case interface{ Size() int64 }: // e.g. a reader reporting progress; -1 if unknown
		size = v.Size()
	}
	if size < 0 {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		r, size = bytes.NewReader(data), int64(len(data))
	}
	if size > multipartThreshold {
		return b.putMultipart(objectKey(hash), r, size)
	}
	resp, err := b.do("PUT", objectKey(hash), nil, r, size, hash, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return s3Error(&s3Response{status: resp.StatusCode, body: data})
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// Objects bigger than multipartThreshold go up in parts of partSize: storage
// takes at most 5 GB per request. Parts are streamed, not held in memory.
var (
	multipartThreshold int64 = 512 << 20
	partSize           int64 = 64 << 20
)

// putMultipart uploads a big object in parts; on any failure the upload is
// aborted, so no parts are left behind (and billed).
func (b *S3Backend) putMultipart(key string, r io.Reader, size int64) error {
	start, err := b.call("POST", key, url.Values{"uploads": {""}}, nil, nil)
	if err != nil {
		return err
	}
	if start.status != http.StatusOK {
		return s3Error(start)
	}
	var started struct{ UploadId string }
	if err := xml.Unmarshal(start.body, &started); err != nil || started.UploadId == "" {
		return fmt.Errorf("storage did not start an upload of %s", key)
	}
	id := started.UploadId
	abort := func(err error) error {
		b.call("DELETE", key, url.Values{"uploadId": {id}}, nil, nil)
		return err
	}
	var done bytes.Buffer
	done.WriteString("<CompleteMultipartUpload>")
	for n, off := 1, int64(0); off < size; n, off = n+1, off+partSize {
		part := min(partSize, size-off)
		q := url.Values{"partNumber": {strconv.Itoa(n)}, "uploadId": {id}}
		// The object's hash covers the whole file; parts go unsigned, and the
		// download checks the hash.
		resp, err := b.do("PUT", key, q, io.LimitReader(r, part), part, unsignedPayload, nil)
		if err != nil {
			return abort(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return abort(s3Error(&s3Response{status: resp.StatusCode, body: body}))
		}
		fmt.Fprintf(&done, "<Part><PartNumber>%d</PartNumber><ETag>%s</ETag></Part>", n, html.EscapeString(resp.Header.Get("ETag")))
	}
	done.WriteString("</CompleteMultipartUpload>")
	end, err := b.call("POST", key, url.Values{"uploadId": {id}}, done.Bytes(), nil)
	if err != nil {
		return abort(err)
	}
	// Completing can fail with 200 and an error in the body.
	if end.status != http.StatusOK || bytes.Contains(end.body, []byte("<Error>")) {
		return abort(s3Error(end))
	}
	return nil
}

func (b *S3Backend) GetObject(hash string) (io.ReadCloser, error) {
	if !validHex(hash, 64) {
		return nil, ErrNotFound
	}
	resp, err := b.do("GET", objectKey(hash), nil, nil, 0, "", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, s3Error(&s3Response{status: resp.StatusCode, body: data})
	}
	return resp.Body, nil
}

func (b *S3Backend) PutWorkspace(pid, wsid string, state any) error {
	if !validHex(wsid, 32) {
		return errors.New("invalid workspace id")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	r, err := b.put(workspaceKey(pid, wsid), data, nil)
	if err != nil {
		return err
	}
	if r.status != http.StatusOK {
		return s3Error(r)
	}
	return nil
}

func (b *S3Backend) Members() ([]Member, error) {
	keys, err := b.list("members/", false)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	out := []Member{}
	err = parallel(keys, func(key string) error {
		r, err := b.get(key)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var m Member
		if json.Unmarshal(r.body, &m) == nil && ValidMemberID(m.ID) {
			mu.Lock()
			out = append(out, m)
			mu.Unlock()
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}

func (b *S3Backend) PutMember(m Member) error {
	if !ValidMemberID(m.ID) {
		return errors.New("invalid member id")
	}
	data, _ := json.Marshal(m)
	r, err := b.put(memberKey(m.ID), data, nil)
	if err != nil {
		return err
	}
	if r.status != http.StatusOK {
		return s3Error(r)
	}
	return nil
}

func (b *S3Backend) Workspaces(pid string, out any) error {
	keys, err := b.list(projectDir(pid)+"workspaces/", false)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var docs []json.RawMessage
	err = parallel(keys, func(key string) error {
		r, err := b.get(key)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if json.Valid(r.body) {
			mu.Lock()
			docs = append(docs, r.body)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if docs == nil {
		docs = []json.RawMessage{}
	}
	data, _ := json.Marshal(docs)
	return json.Unmarshal(data, out)
}

// Info reads the team name from team.json in the storage folder.
func (b *S3Backend) Info() (TeamInfo, error) {
	var info TeamInfo
	r, err := b.get("team.json")
	if errors.Is(err, ErrNotFound) {
		return info, nil
	}
	if err != nil {
		return info, err
	}
	json.Unmarshal(r.body, &info)
	return info, nil
}

// SetInfo names the team (written by whoever sets up the storage).
func (b *S3Backend) SetInfo(info TeamInfo) error {
	data, _ := json.Marshal(info)
	r, err := b.put("team.json", data, nil)
	if err != nil {
		return err
	}
	if r.status != http.StatusOK {
		return s3Error(r)
	}
	return nil
}

// keepAlive is a transport that keeps connections for the many requests a
// transfer makes at once. Go's default keeps 2 per host: the others were
// closed after each file and set up again, TLS and all, which takes longer
// than sending a small file.
func keepAlive() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 128
	t.MaxIdleConnsPerHost = 64
	return t
}
