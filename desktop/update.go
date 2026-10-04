package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dawgit/internal/teams"
	"dawgit/internal/update"
	"dawgit/internal/version"
)

// UpdateInfo describes a newer release than the one running.
type UpdateInfo struct {
	Version     string `json:"version"`
	PageURL     string `json:"pageUrl"`     // what's new
	DownloadURL string `json:"downloadUrl"` // the installer ("" if none)
	// Installable: signed, so DAWGit downloads and installs it itself (else
	// the installer is only offered as a download).
	Installable bool `json:"installable"`
	// Required: this version is too old to keep working (the release says
	// so, e.g. a new version format): it must be updated.
	Required bool `json:"required"`
}

// UpdateState is how getting an update is going.
type UpdateState struct {
	Stage string `json:"stage"` // "" | "downloading" | "ready" | "failed"
	Done  int64  `json:"done"`  // bytes
	Total int64  `json:"total"`
	Error string `json:"error"`
	Auto  bool   `json:"auto"` // installed on its own (see SetAutoUpdate)
}

var updateCache struct {
	sync.Mutex
	checked time.Time
	info    *UpdateInfo
	rel     *update.Release
	state   UpdateState
	path    string // the installer, when ready
}

// CheckUpdateNow asks for a newer release now (the settings' "Check for
// updates"), not waiting for the next check.
func (a *App) CheckUpdateNow() (*UpdateInfo, error) {
	updateCache.Lock()
	updateCache.checked = time.Time{}
	updateCache.Unlock()
	return a.CheckUpdate()
}

// CheckUpdate asks for a newer release (at most every 6 hours) and returns
// it, or nil when this is the newest. A signed one is downloaded right away,
// in the background. DAWGIT_NO_UPDATE_CHECK=1 turns it off;
// DAWGIT_DEV_VERSION pretends to be another version and DAWGIT_DEV_FEED
// checks another feed (testing).
func (a *App) CheckUpdate() (*UpdateInfo, error) {
	// A build with extensions is updated from its own feed (the public
	// release would replace it); without one it doesn't check.
	if os.Getenv("DAWGIT_NO_UPDATE_CHECK") != "" || (version.Edition != "" && update.Feed == "" && os.Getenv("DAWGIT_DEV_FEED") == "") {
		return nil, nil
	}
	updateCache.Lock()
	defer updateCache.Unlock()
	if time.Since(updateCache.checked) < 6*time.Hour {
		return updateCache.info, nil
	}
	current := currentVersion()
	feed, switching := update.Feed, false
	if feed == "" && chosenChannel() == "nightly" {
		// From Stable, the first Nightly may sort before this version
		// (0.13.0-nightly.x after 0.13.0): it is a switch, not an update.
		feed, switching = update.NightlyFeed, version.Channel != "nightly"
	}
	if f := os.Getenv("DAWGIT_DEV_FEED"); f != "" {
		feed = f // testing updates against a feed of one's own
	}
	var r *update.Release
	var err error
	switch {
	case feed != "" && switching:
		r, err = update.SwitchTo(feed, current)
	case feed != "":
		r, err = update.FromFeed(feed, current)
	default:
		// Stable. From a Nightly that waits for the next Stable release
		// (0.13.0 after 0.13.0-nightly.x): never back to an older version.
		r, err = update.Newer(update.ReleasesAPI, current)
	}
	if err != nil {
		return nil, err // offline, rate limited…: try again next time
	}
	updateCache.checked, updateCache.info, updateCache.rel = time.Now(), nil, r
	if r != nil {
		updateCache.info = &UpdateInfo{Version: r.Version, PageURL: r.PageURL, DownloadURL: r.DownloadURL,
			Installable: r.Installable(), Required: r.Requires(current)}
		if r.Installable() && updateCache.state.Stage == "" {
			go a.DownloadUpdate()
		}
	}
	return updateCache.info, nil
}

func currentVersion() string {
	if v := os.Getenv("DAWGIT_DEV_VERSION"); v != "" {
		return v
	}
	return version.Full()
}

// chosenChannel: the release line updates come from (Settings).
func chosenChannel() string {
	if store, err := teams.Load(); err == nil && store.Channel != "" {
		return store.Channel
	}
	return version.Channel
}

// ChannelInfo is the update channel, for Settings.
type ChannelInfo struct {
	Build  string `json:"build"`  // this build's: "stable" or "nightly"
	Chosen string `json:"chosen"` // where updates come from
}

// Channel says which release line this is and which one updates come from.
func (a *App) Channel() ChannelInfo {
	return ChannelInfo{Build: version.Channel, Chosen: chosenChannel()}
}

// SetChannel picks where updates come from, and checks there now. To
// Nightly, its latest build is offered right away; back to Stable, this
// Nightly stays until a Stable release is newer than it.
func (a *App) SetChannel(channel string) (*UpdateInfo, error) {
	if channel != "stable" && channel != "nightly" {
		return nil, errors.New("unknown channel")
	}
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	store.Channel = channel
	if err := store.Save(); err != nil {
		return nil, err
	}
	updateCache.Lock()
	if updateCache.state.Stage != "downloading" {
		updateCache.rel, updateCache.info, updateCache.state, updateCache.path = nil, nil, UpdateState{}, ""
	}
	updateCache.Unlock()
	a.emitUpdate()
	// Chosen either way; a check that fails (offline) is tried again later.
	u, _ := a.CheckUpdateNow()
	return u, nil
}

// DownloadUpdate fetches the newer release's installer and checks its
// signature; "update" events say how it goes.
func (a *App) DownloadUpdate() error {
	updateCache.Lock()
	r := updateCache.rel
	if r == nil || !r.Installable() {
		updateCache.Unlock()
		return errors.New("no update to install")
	}
	if updateCache.state.Stage == "downloading" || updateCache.state.Stage == "ready" {
		updateCache.Unlock()
		return nil
	}
	updateCache.state = UpdateState{Stage: "downloading", Auto: autoUpdate()}
	updateCache.Unlock()
	a.emitUpdate()

	var last time.Time
	path, err := update.Download(r, filepath.Join(os.TempDir(), "DAWGit-update"), func(done, total int64) {
		if time.Since(last) < 200*time.Millisecond {
			return
		}
		last = time.Now()
		updateCache.Lock()
		updateCache.state.Done, updateCache.state.Total = done, total
		updateCache.Unlock()
		a.emitUpdate()
	})
	updateCache.Lock()
	if err != nil {
		updateCache.state = UpdateState{Stage: "failed", Error: err.Error(), Auto: autoUpdate()}
	} else {
		updateCache.state.Stage, updateCache.path = "ready", path
	}
	updateCache.Unlock()
	a.emitUpdate()
	return err
}

// UpdateStatus says how getting the update is going.
func (a *App) UpdateStatus() UpdateState {
	updateCache.Lock()
	defer updateCache.Unlock()
	s := updateCache.state
	s.Auto = autoUpdate()
	return s
}

func (a *App) emitUpdate() {
	if a.emit != nil {
		a.emit("update", a.UpdateStatus())
	}
}

// Busy: a project is in the middle of something (a commit, an upload, a
// download): no update is installed now.
func (a *App) Busy() bool {
	return a.working.Load() > 0 || time.Since(time.Unix(0, a.lastProgress.Load())) < 30*time.Second
}

// InstallUpdate installs the downloaded update: its installer runs on its
// own, replaces DAWGit and opens it again; DAWGit quits.
func (a *App) InstallUpdate() error {
	if a.Busy() {
		return errors.New("DAWGit is still working on a project: try again when it's done")
	}
	return a.runInstaller("/relaunch", true)
}

// runInstaller starts the downloaded installer silently (relaunch: how
// DAWGit opens again after it, "" for not) and, with quit, quits.
func (a *App) runInstaller(relaunch string, quit bool) error {
	updateCache.Lock()
	path := updateCache.path
	ready := updateCache.state.Stage == "ready"
	updateCache.Unlock()
	if !ready || path == "" {
		return errors.New("the update isn't downloaded yet")
	}
	args := []string{"/S"}
	if relaunch != "" {
		args = append(args, relaunch)
	}
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	if quit && a.quit != nil {
		go func() {
			time.Sleep(300 * time.Millisecond) // let the call return
			a.quit()
		}()
	}
	return nil
}

// AutoUpdate: updates install on their own (when DAWGit is in the tray with
// nothing to do, or quits); on by default.
func (a *App) AutoUpdate() bool { return autoUpdate() }

func autoUpdate() bool {
	store, err := teams.Load()
	return err != nil || !store.ManualUpdates
}

func (a *App) SetAutoUpdate(on bool) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	store.ManualUpdates = !on
	if err := store.Save(); err != nil {
		return err
	}
	a.emitUpdate()
	return nil
}

// updateInBackground installs a downloaded update by itself while DAWGit is
// in the tray and idle, and opens again in the tray: checked every few
// minutes from the start.
func (a *App) updateInBackground() {
	for range time.Tick(5 * time.Minute) {
		a.CheckUpdate() // keeps checking while DAWGit stays in the tray for days
		updateCache.Lock()
		ready := updateCache.state.Stage == "ready"
		updateCache.Unlock()
		if ready && autoUpdate() && a.hidden.Load() && !a.Busy() {
			if err := a.runInstaller("/relaunch-background", true); err == nil {
				return
			}
		}
	}
}

// beforeQuit installs a downloaded update as DAWGit quits (automatic
// updates on): DAWGit isn't opened again.
func (a *App) beforeQuit() {
	updateCache.Lock()
	ready := updateCache.state.Stage == "ready"
	updateCache.Unlock()
	if ready && autoUpdate() && !a.Busy() {
		a.runInstaller("", false)
	}
}

// OpenURL opens a page the app links to in the browser: DAWGit's (release
// notes, installer, guides), the update feed's site, Cloudflare's dashboard
// and docs (team setup). Other addresses are refused.
func (a *App) OpenURL(url string) error {
	allowed := []string{update.PageURLPrefix, "https://dash.cloudflare.com/", "https://developers.cloudflare.com/"}
	if update.Feed != "" {
		allowed = append(allowed, update.FeedSite(update.Feed))
	}
	ok := false
	for _, p := range allowed {
		ok = ok || strings.HasPrefix(url, p)
	}
	if !ok {
		return errors.New("not a link DAWGit opens")
	}
	if a.openURL == nil {
		return errors.New("cannot open a browser here")
	}
	return a.openURL(url)
}
