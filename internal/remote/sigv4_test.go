package remote

import (
	"net/http"
	"testing"
	"time"
)

// Example "GET Object" from the Amazon S3 documentation
// (Signature Version 4, authorization header, example calculations).
func TestSigV4MatchesAWSExample(t *testing.T) {
	s := &signer{
		accessKey: "AKIAIOSFODNN7EXAMPLE",
		secretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		region:    "us-east-1",
		now:       func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) },
	}
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	s.sign(req, emptySHA256)

	auth := req.Header.Get("Authorization")
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if auth != want {
		t.Fatalf("Authorization\n got: %s\nwant: %s", auth, want)
	}
}

func TestURIEncode(t *testing.T) {
	cases := map[string]string{
		"projects/ab cd/x.json": "projects/ab%20cd/x.json",
		"a+b=c":                 "a%2Bb%3Dc",
		"~safe-._":              "~safe-._",
	}
	for in, want := range cases {
		if got := uriEncode(in, true); got != want {
			t.Errorf("uriEncode(%q) = %q, want %q", in, got, want)
		}
	}
	if got := uriEncode("a/b", false); got != "a%2Fb" {
		t.Errorf("slash not encoded in query: %q", got)
	}
}
