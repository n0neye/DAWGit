package remote

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Storage is team storage (an S3-compatible bucket) as a person fills it in.
type Storage struct {
	// Endpoint: the S3 API address, e.g. https://<account>.r2.cloudflarestorage.com
	// (a Cloudflare account id alone works too; a trailing /<bucket> is taken
	// as the bucket).
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Folder    string `json:"folder"` // inside the bucket; "" means "dawgit"
	Region    string `json:"region"` // "" means "auto" (R2)
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

var accountID = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// Config turns the filled-in fields into a storage config.
func (s Storage) Config() (Config, error) {
	ep := strings.TrimRight(strings.TrimSpace(s.Endpoint), "/")
	bucket := strings.Trim(strings.TrimSpace(s.Bucket), "/")
	if accountID.MatchString(ep) {
		ep = "https://" + strings.ToLower(ep) + ".r2.cloudflarestorage.com"
	}
	if ep != "" && !strings.Contains(ep, "://") {
		ep = "https://" + ep
	}
	u, err := url.Parse(ep)
	if ep == "" || err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return Config{}, errors.New("the endpoint should look like https://<account id>.r2.cloudflarestorage.com")
	}
	// The dashboard shows the endpoint with the bucket after it.
	if p := strings.Trim(u.Path, "/"); p != "" {
		first, _, _ := strings.Cut(p, "/")
		if bucket == "" {
			bucket = first
		} else if first != bucket {
			return Config{}, fmt.Errorf("the endpoint ends in /%s but the bucket is %q", first, bucket)
		}
	}
	if bucket == "" {
		return Config{}, errors.New("enter the bucket's name")
	}
	folder := strings.Trim(strings.TrimSpace(s.Folder), "/")
	if folder == "" {
		folder = "dawgit"
	}
	region := strings.TrimSpace(s.Region)
	if region == "" {
		region = "auto"
	}
	access, secret := strings.TrimSpace(s.AccessKey), strings.TrimSpace(s.SecretKey)
	if access == "" || secret == "" {
		return Config{}, errors.New("enter the Access Key ID and the Secret Access Key")
	}
	// Cloudflare shows a "Token value" above the S3 credentials; it is not one.
	if strings.HasPrefix(access, "cfut_") || strings.HasPrefix(secret, "cfut_") {
		return Config{}, errors.New(`that is the token value: use the Access Key ID and Secret Access Key ` +
			`listed under "Use the following credentials for S3 clients"`)
	}
	return Config{URL: "s3+" + u.Scheme + "://" + u.Host + "/" + bucket + "/" + folder,
		AccessKey: access, SecretKey: secret, Region: region}, nil
}

// StorageOf splits a storage config back into its fields.
func StorageOf(c Config) (Storage, bool) {
	if !c.IsStorage() {
		return Storage{}, false
	}
	u, err := url.Parse(c.URL[len("s3+"):])
	if err != nil {
		return Storage{}, false
	}
	bucket, folder, _ := strings.Cut(strings.Trim(u.Path, "/"), "/")
	return Storage{Endpoint: u.Scheme + "://" + u.Host, Bucket: bucket, Folder: folder, Region: c.Region,
		AccessKey: c.AccessKey, SecretKey: c.SecretKey}, true
}

// Check makes sure a team can work with cfg: it can be reached, the
// credentials read and write, and (for storage) conditional writes are
// honoured, which keeps two people from overwriting each other's versions.
// Errors say what to fix in plain words.
func Check(cfg Config) error {
	b, err := Open(cfg)
	if err != nil {
		return err
	}
	if _, err := b.Projects(); err != nil {
		return explain(err)
	}
	if s, ok := b.(*S3Backend); ok {
		return explain(s.checkWrites())
	}
	return nil
}

// CheckBackup makes sure cfg can take a backup: it can be reached, and the
// keys list, write and delete (no conditional writes needed: one member
// writes there).
func CheckBackup(cfg Config) error {
	b, err := Open(cfg)
	if err != nil {
		return err
	}
	s, ok := b.(*S3Backend)
	if !ok {
		return errors.New("a backup goes to S3-compatible storage")
	}
	if _, err := s.list("check/", false); err != nil {
		return explain(err)
	}
	key := "check/" + newCheckID()
	r, err := s.put(key, []byte("check\n"), nil)
	if err == nil && r.status != http.StatusOK {
		err = s3Error(r)
	}
	if err != nil {
		return explain(err)
	}
	return explain(s.delete(key))
}

// checkWrites writes, conditionally rewrites and removes a scratch object.
func (b *S3Backend) checkWrites() error {
	key := "check/" + newCheckID()
	defer b.delete(key)
	create := http.Header{"If-None-Match": {"*"}}
	r, err := b.put(key, []byte("check\n"), create)
	if err != nil {
		return err
	}
	if r.status != http.StatusOK {
		return s3Error(r)
	}
	if r, err = b.put(key, []byte("again\n"), create); err != nil {
		return err
	}
	switch r.status {
	case http.StatusPreconditionFailed, http.StatusConflict:
	case http.StatusOK:
		return errNoConditional
	default:
		return s3Error(r)
	}
	if err := b.delete(key); err != nil {
		return fmt.Errorf("could not remove a test file (the key needs permission to delete): %w", err)
	}
	return nil
}

var errNoConditional = errors.New("this storage ignores conditional writes, so two people could overwrite each other's versions. " +
	"Use Cloudflare R2, Amazon S3 or another storage that supports If-None-Match")

// explain turns storage and network errors into what to check.
func explain(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	var dns *net.DNSError
	var opErr *net.OpError
	switch {
	case errors.As(err, &dns), errors.As(err, &opErr), strings.Contains(msg, "no such host"):
		return errors.New("can't reach the endpoint: check the address and your internet connection")
	case strings.Contains(msg, "bucket not found"):
		return errors.New("no bucket with that name at this endpoint: check the bucket name")
	case strings.Contains(msg, "rejected the credentials"):
		return fmt.Errorf("the keys were refused: check the Access Key ID and Secret Access Key, and that the token "+
			"may read and write this bucket (%v)", err)
	}
	return err
}

func newCheckID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
