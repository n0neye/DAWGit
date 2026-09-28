// Package remote is the HTTP client for a DAWGit server.
package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	URL   string // e.g. http://nas.local:7331
	Token string
	HTTP  *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{URL: strings.TrimRight(baseURL, "/"), Token: token,
		HTTP: &http.Client{Timeout: 30 * time.Minute}}
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ErrConflict means the branch moved since it was read.
type ErrConflict struct{ Current string }

func (e *ErrConflict) Error() string { return "branch was updated by someone else" }

// ErrNotFound is returned for unknown projects, snapshots or objects.
var ErrNotFound = errors.New("not found on server")

func (c *Client) do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.URL+"/api/v1"+path, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach server %s: %w", c.URL, err)
	}
	return resp, nil
}

func apiError(resp *http.Response) error {
	var body struct{ Error, Current string }
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	json.Unmarshal(data, &body)
	switch resp.StatusCode {
	case http.StatusConflict:
		return &ErrConflict{Current: body.Current}
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnauthorized:
		return errors.New("server rejected the access token (check `dawgit remote`)")
	}
	if body.Error == "" {
		body.Error = strings.TrimSpace(string(data))
	}
	return fmt.Errorf("server error %d: %s", resp.StatusCode, body.Error)
}

// call sends a JSON request and decodes a JSON response into out (if non-nil).
func (c *Client) call(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	resp, err := c.do(method, path, body, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return apiError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) Projects() ([]Project, error) {
	var out []Project
	return out, c.call("GET", "/projects", nil, &out)
}

func (c *Client) PutProject(p Project) error {
	return c.call("PUT", "/projects/"+p.ID, map[string]string{"name": p.Name}, nil)
}

func (c *Client) Branches(pid string) (map[string]string, error) {
	out := map[string]string{}
	return out, c.call("GET", "/projects/"+pid+"/branches", nil, &out)
}

// UpdateBranch moves a branch from old ("" = create) to new.
func (c *Client) UpdateBranch(pid, name, old, new string) error {
	return c.call("POST", "/projects/"+pid+"/branches/"+url.PathEscape(name),
		map[string]string{"old": old, "new": new}, nil)
}

func (c *Client) MissingSnapshots(pid string, ids []string) ([]string, error) {
	var out struct{ Missing []string }
	return out.Missing, c.call("POST", "/projects/"+pid+"/snapshots/missing", map[string][]string{"ids": ids}, &out)
}

func (c *Client) PutSnapshot(pid, id string, data []byte) error {
	resp, err := c.do("PUT", "/projects/"+pid+"/snapshots/"+id, bytes.NewReader(data), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return apiError(resp)
	}
	return nil
}

func (c *Client) GetSnapshot(pid, id string) ([]byte, error) {
	resp, err := c.do("GET", "/projects/"+pid+"/snapshots/"+id, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, apiError(resp)
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) MissingObjects(hashes []string) ([]string, error) {
	var out struct{ Missing []string }
	return out.Missing, c.call("POST", "/objects/missing", map[string][]string{"hashes": hashes}, &out)
}

func (c *Client) PutObject(hash string, r io.Reader) error {
	resp, err := c.do("PUT", "/objects/"+hash, r, "application/octet-stream")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return apiError(resp)
	}
	return nil
}

// GetObject returns the blob's body; the caller closes it.
func (c *Client) GetObject(hash string) (io.ReadCloser, error) {
	resp, err := c.do("GET", "/objects/"+hash, nil, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, apiError(resp)
	}
	return resp.Body, nil
}

// PutWorkspace stores this workspace's state (any JSON-encodable value).
func (c *Client) PutWorkspace(pid, wsid string, state any) error {
	return c.call("PUT", "/projects/"+pid+"/workspaces/"+wsid, state, nil)
}

// Workspaces decodes all workspace states of a project into out (a pointer
// to a slice).
func (c *Client) Workspaces(pid string, out any) error {
	return c.call("GET", "/projects/"+pid+"/workspaces", nil, out)
}
