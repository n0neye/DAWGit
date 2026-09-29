package main

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"dawgit/internal/update"
	"dawgit/internal/version"
)

// UpdateInfo describes a newer release than the one running.
type UpdateInfo struct {
	Version     string `json:"version"`
	PageURL     string `json:"pageUrl"`     // what's new
	DownloadURL string `json:"downloadUrl"` // the installer ("" if none)
}

var updateCache struct {
	sync.Mutex
	checked time.Time
	info    *UpdateInfo
}

// CheckUpdate asks GitHub for a newer release (at most every 6 hours) and
// returns it, or nil when this is the newest. DAWGIT_NO_UPDATE_CHECK=1 turns
// it off; DAWGIT_DEV_VERSION pretends to be another version (testing).
func (a *App) CheckUpdate() (*UpdateInfo, error) {
	if os.Getenv("DAWGIT_NO_UPDATE_CHECK") != "" {
		return nil, nil
	}
	updateCache.Lock()
	defer updateCache.Unlock()
	if time.Since(updateCache.checked) < 6*time.Hour {
		return updateCache.info, nil
	}
	current := version.Version
	if v := os.Getenv("DAWGIT_DEV_VERSION"); v != "" {
		current = v
	}
	r, err := update.Newer(update.ReleasesAPI, current)
	if err != nil {
		return nil, err // offline, rate limited…: try again next time
	}
	updateCache.checked, updateCache.info = time.Now(), nil
	if r != nil {
		updateCache.info = &UpdateInfo{Version: r.Version, PageURL: r.PageURL, DownloadURL: r.DownloadURL}
	}
	return updateCache.info, nil
}

// OpenURL opens a DAWGit page (release notes, installer download) in the
// browser. Other addresses are refused.
func (a *App) OpenURL(url string) error {
	if !strings.HasPrefix(url, update.PageURLPrefix) {
		return errors.New("not a DAWGit link")
	}
	if a.openURL == nil {
		return errors.New("cannot open a browser here")
	}
	return a.openURL(url)
}
