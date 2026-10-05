package remote

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// s3Bucket is a Bucket in S3-compatible storage (Cloudflare R2, Amazon S3,
// MinIO…), spoken to directly with signed path-style requests: a folder
// (prefix) of a bucket.
type s3Bucket struct {
	endpoint *url.URL // scheme + host
	bucket   string
	prefix   string // "" or "team/" (ends with a slash)
	sig      *signer
	http     *http.Client
}

var _ Bucket = (*s3Bucket)(nil)

func newS3Bucket(endpoint, bucket, prefix, region, accessKey, secretKey string) (*s3Bucket, error) {
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
	return &s3Bucket{
		endpoint: &url.URL{Scheme: u.Scheme, Host: u.Host},
		bucket:   bucket, prefix: prefix,
		sig:  &signer{accessKey: accessKey, secretKey: secretKey, region: region, now: time.Now},
		http: &http.Client{Timeout: 30 * time.Minute, Transport: keepAlive()},
	}, nil
}

// keepAlive is a transport that keeps connections for the many requests a
// transfer makes at once. Go's default keeps 2 per host: the others were
// closed after each file and set up again, TLS and all, which takes longer
// than sending a small file.
func keepAlive() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	// Storage that answers at all connects within a few seconds; without a
	// network the default (30s, for every try) kept the app waiting minutes.
	t.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.MaxIdleConns = 128
	t.MaxIdleConnsPerHost = 64
	return t
}

// --- requests ---

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

func (s *s3Bucket) keyURL(key string, q url.Values) *url.URL {
	u := *s.endpoint
	u.Path = "/" + s.bucket + "/" + s.prefix + key
	if key == "" {
		u.Path = "/" + s.bucket + "/"
	}
	u.RawQuery = q.Encode()
	return &u
}

// do sends a signed request. body may be nil; size < 0 means unknown.
// payloadHash is the hex SHA-256 of body (or unsignedPayload).
func (s *s3Bucket) do(method, key string, q url.Values, body io.Reader, size int64, payloadHash string,
	header http.Header) (*http.Response, error) {
	req, err := http.NewRequest(method, s.keyURL(key, q).String(), body)
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
	s.sig.sign(req, payloadHash)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach storage %s: %w", s.endpoint.Host, err)
	}
	return resp, nil
}

// call runs a request and reads the whole response.
// A storage error that may pass (5xx, too many requests) is tried again.
func (s *s3Bucket) call(method, key string, q url.Values, body []byte, header http.Header) (*s3Response, error) {
	hash := emptySHA256
	if body != nil {
		hash = sha256Hex(body)
	}
	var out *s3Response
	err := Retry(RetryAttempts, func() error {
		var r io.Reader
		if body != nil {
			r = bytes.NewReader(body)
		}
		resp, err := s.do(method, key, q, r, int64(len(body)), hash, header)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		out = &s3Response{status: resp.StatusCode, etag: resp.Header.Get("ETag"), body: data}
		if out.status >= 500 || out.status == http.StatusTooManyRequests {
			return s3Error(out) // transient: tried again, then returned as it is
		}
		return nil
	})
	var e *errS3
	if errors.As(err, &e) && out != nil {
		return out, nil // the caller reads the status
	}
	return out, err
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
	case http.StatusPreconditionFailed, http.StatusConflict:
		return ErrPrecondition
	}
	if msg == "" {
		msg = strings.TrimSpace(string(r.body))
	}
	return &errS3{r.status, msg}
}

func condHeader(cond string) http.Header {
	h := http.Header{}
	switch cond {
	case "":
	case "*":
		h.Set("If-None-Match", "*")
	default:
		h.Set("If-Match", cond)
	}
	return h
}

// --- Bucket ---

func (s *s3Bucket) Get(key string) ([]byte, string, error) {
	r, err := s.call("GET", key, nil, nil, nil)
	if err != nil {
		return nil, "", err
	}
	if r.status != http.StatusOK {
		return nil, "", s3Error(r)
	}
	return r.body, r.etag, nil
}

func (s *s3Bucket) Open(key string) (io.ReadCloser, error) {
	resp, err := s.do("GET", key, nil, nil, 0, "", nil)
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

func (s *s3Bucket) Exists(key string) (bool, error) {
	r, err := s.call("HEAD", key, nil, nil, nil)
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

// smallPut: a body up to this size is sent from memory, signed, and tried
// again when storage has a passing problem.
const smallPut = 8 << 20

func (s *s3Bucket) Put(key string, r io.Reader, size int64, sum, cond string) error {
	if size > multipartThreshold {
		if cond != "" {
			return errors.New("storage: a conditional write of a big object")
		}
		return s.putMultipart(key, r, size)
	}
	header := condHeader(cond)
	if br, ok := r.(*bytes.Reader); ok && size <= smallPut {
		data := make([]byte, br.Len())
		br.Read(data)
		if sum != "" && sha256Hex(data) != sum {
			return fmt.Errorf("storage: %s: the bytes don't match their SHA-256", key)
		}
		res, err := s.call("PUT", key, nil, data, header) // signed with the bytes' SHA-256
		if err != nil {
			return err
		}
		if res.status != http.StatusOK {
			return s3Error(res)
		}
		return nil
	}
	hash := sum
	if hash == "" {
		hash = unsignedPayload // the caller checks sizes afterwards
	}
	resp, err := s.do("PUT", key, nil, r, size, hash, header)
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

func (s *s3Bucket) Copy(src, dst string) error {
	source := "/" + s.bucket + "/" + s.prefix + src
	h := http.Header{"x-amz-copy-source": {(&url.URL{Path: source}).EscapedPath()}}
	r, err := s.call("PUT", dst, nil, nil, h)
	if err != nil {
		return err
	}
	// Copying can fail with 200 and an error in the body.
	if r.status != http.StatusOK || bytes.Contains(r.body, []byte("<Error>")) {
		return s3Error(r)
	}
	return nil
}

func (s *s3Bucket) Delete(key, cond string) error {
	r, err := s.call("DELETE", key, nil, nil, condHeader(cond))
	if err != nil {
		return err
	}
	switch r.status {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		if cond == "" {
			return nil
		}
	}
	return s3Error(r)
}

func (s *s3Bucket) List(prefix, startAfter string, page func([]Item) bool) error {
	token := ""
	for {
		p, err := s.listPage(prefix, false, startAfter, token)
		if err != nil {
			return err
		}
		if len(p.items) > 0 && !page(p.items) {
			return nil
		}
		if p.next == "" {
			return nil
		}
		token = p.next
	}
}

func (s *s3Bucket) Folders(dir string) ([]string, error) {
	var out []string
	token := ""
	for {
		p, err := s.listPage(dir, true, "", token)
		if err != nil {
			return nil, err
		}
		out = append(out, p.folders...)
		if p.next == "" {
			return out, nil
		}
		token = p.next
	}
}

type listing struct {
	items   []Item   // keys (relative to prefix), without delim
	folders []string // sub-"folders", with delim
	next    string   // continuation token; "" on the last page
}

// listPage returns one page (up to 1000 keys) under dir, after the key
// startAfter (relative to prefix; "" for the start) or from a continuation
// token.
func (s *s3Bucket) listPage(dir string, delim bool, startAfter, token string) (listing, error) {
	q := url.Values{"list-type": {"2"}, "prefix": {s.prefix + dir}}
	if delim {
		q.Set("delimiter", "/")
	}
	if token != "" {
		q.Set("continuation-token", token)
	} else if startAfter != "" {
		q.Set("start-after", s.prefix+startAfter)
	}
	r, err := s.call("GET", "", q, nil, nil)
	if err != nil {
		return listing{}, err
	}
	if r.status != http.StatusOK {
		return listing{}, s3Error(r)
	}
	var res struct {
		Contents []struct {
			Key          string
			Size         int64
			LastModified string
		}
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
			out.folders = append(out.folders, strings.TrimPrefix(p.Prefix, s.prefix))
		}
	} else {
		for _, c := range res.Contents {
			t, _ := time.Parse(time.RFC3339Nano, c.LastModified)
			out.items = append(out.items, Item{Key: strings.TrimPrefix(c.Key, s.prefix), Size: c.Size, Modified: t})
		}
	}
	if res.IsTruncated {
		out.next = res.NextContinuationToken
	}
	return out, nil
}

// Objects bigger than multipartThreshold go up in parts of partSize: storage
// takes at most 5 GB per request. Parts are streamed, not held in memory.
var (
	multipartThreshold int64 = 512 << 20
	partSize           int64 = 64 << 20
)

// putMultipart uploads a big object in parts; on any failure the upload is
// aborted, so no parts are left behind (and billed).
func (s *s3Bucket) putMultipart(key string, r io.Reader, size int64) error {
	start, err := s.call("POST", key, url.Values{"uploads": {""}}, nil, nil)
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
		s.call("DELETE", key, url.Values{"uploadId": {id}}, nil, nil)
		return err
	}
	var done bytes.Buffer
	done.WriteString("<CompleteMultipartUpload>")
	for n, off := 1, int64(0); off < size; n, off = n+1, off+partSize {
		part := min(partSize, size-off)
		q := url.Values{"partNumber": {strconv.Itoa(n)}, "uploadId": {id}}
		// The object's hash covers the whole file; parts go unsigned, and the
		// download checks the hash.
		resp, err := s.do("PUT", key, q, io.LimitReader(r, part), part, unsignedPayload, nil)
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
	end, err := s.call("POST", key, url.Values{"uploadId": {id}}, done.Bytes(), nil)
	if err != nil {
		return abort(err)
	}
	// Completing can fail with 200 and an error in the body.
	if end.status != http.StatusOK || bytes.Contains(end.body, []byte("<Error>")) {
		return abort(s3Error(end))
	}
	return nil
}
