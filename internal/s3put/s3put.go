// Package s3put uploads a file to an S3-compatible bucket (Cloudflare R2),
// signed with AWS Signature Version 4. Just enough to publish releases.
package s3put

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Target is a bucket and the key that may write to it.
type Target struct {
	Endpoint  string // e.g. https://<account>.r2.cloudflarestorage.com
	Bucket    string
	Region    string // "auto" for R2
	AccessKey string
	SecretKey string
}

// Put uploads data as key with a content type.
func (t Target) Put(key string, data []byte, contentType string) error {
	u, err := url.Parse(strings.TrimRight(t.Endpoint, "/") + "/" + t.Bucket + "/" + escapeKey(key))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", u.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", contentType)
	t.sign(req, data, time.Now().UTC())
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("upload %s: %s %s", key, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(p), "+", "%2B")
	}
	return strings.Join(parts, "/")
}

func (t Target) sign(req *http.Request, body []byte, now time.Time) {
	region := t.Region
	if region == "" {
		region = "auto"
	}
	payload := sha256Hex(body)
	date, stamp := now.Format("20060102"), now.Format("20060102T150405Z")
	req.Header.Set("x-amz-date", stamp)
	req.Header.Set("x-amz-content-sha256", payload)
	host := req.URL.Host
	signed := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonical := strings.Join([]string{
		req.Method, req.URL.EscapedPath(), "",
		"content-type:" + req.Header.Get("Content-Type") + "\nhost:" + host + "\nx-amz-content-sha256:" + payload +
			"\nx-amz-date:" + stamp + "\n",
		signed, payload,
	}, "\n")
	scope := date + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	key := hmacSHA([]byte("AWS4"+t.SecretKey), date)
	key = hmacSHA(key, region)
	key = hmacSHA(key, "s3")
	key = hmacSHA(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA(key, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+t.AccessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+sig)
}

func sha256Hex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func hmacSHA(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}
