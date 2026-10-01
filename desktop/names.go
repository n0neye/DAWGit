package desktop

import (
	"sync"
	"time"

	"dawgit/internal/project"
)

var namesCache struct {
	sync.Mutex
	byTeam map[string]cachedNames
}

type cachedNames struct {
	at    time.Time
	names map[string]string
}

// memberNames returns the team's member names (id -> name), asking the team
// at most once a minute.
func (a *App) memberNames(r *project.Repo) map[string]string {
	if r.Config.Remote == nil {
		return nil
	}
	key := r.Config.Remote.URL
	namesCache.Lock()
	if c, ok := namesCache.byTeam[key]; ok && time.Since(c.at) < time.Minute {
		namesCache.Unlock()
		return c.names
	}
	namesCache.Unlock()
	names := r.MemberNames()
	namesCache.Lock()
	if namesCache.byTeam == nil {
		namesCache.byTeam = map[string]cachedNames{}
	}
	namesCache.byTeam[key] = cachedNames{at: time.Now(), names: names}
	namesCache.Unlock()
	return names
}

// cachedMemberNames is memberNames without asking the team (any age).
func cachedMemberNames(r *project.Repo) map[string]string {
	if r.Config.Remote == nil {
		return nil
	}
	namesCache.Lock()
	defer namesCache.Unlock()
	return namesCache.byTeam[r.Config.Remote.URL].names
}

// forgetNames makes the next memberNames ask the team again (after a rename).
func forgetNames(teamURL string) {
	namesCache.Lock()
	delete(namesCache.byTeam, teamURL)
	namesCache.Unlock()
}
