package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"dawgit/internal/health"
	"dawgit/internal/liveenv"
	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// A member's setup (Live versions, plugins, packs: names and versions only)
// goes to their team's storage when they choose to share it, so a project
// check can tell who lacks what (see internal/health).

// setupEvery is how often a changed setup is shared again.
const setupEvery = 6 * time.Hour

var (
	sharedMu sync.Mutex
	shared   = map[string]health.Setup{} // team id: the setup last shared
)

// shareSetups shares this computer's setup with every team that asked, at
// start and then when it changes.
func (a *App) shareSetups(ctx context.Context) {
	for {
		if store, err := teams.Load(); err == nil {
			setup := health.SetupFrom(liveenv.Read(), time.Now())
			for _, t := range store.Teams {
				if t.ShareSetup && t.MemberID != "" {
					shareSetup(t, setup)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(setupEvery):
		}
	}
}

// shareSetup puts setup in team t's storage, unless it's what was shared.
func shareSetup(t teams.Team, setup health.Setup) error {
	sharedMu.Lock()
	last, ok := shared[t.ID]
	sharedMu.Unlock()
	if ok && last.Same(setup) {
		return nil
	}
	st, err := setupStore(t)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(setup)
	if err := st.PutSetup(t.MemberID, data); err != nil {
		return err
	}
	sharedMu.Lock()
	shared[t.ID] = setup
	sharedMu.Unlock()
	return nil
}

func setupStore(t teams.Team) (remote.SetupStore, error) {
	b, err := remote.Open(t.Remote)
	if err != nil {
		return nil, err
	}
	st, ok := b.(remote.SetupStore)
	if !ok {
		return nil, errors.New("this team's server doesn't keep setups")
	}
	return st, nil
}

// SetShareSetup turns sharing this computer's setup with a team on (it is
// shared now) or off (the shared one is deleted).
func (a *App) SetShareSetup(teamID string, on bool) error {
	saved, err := updateTeam(teamID, func(_ *teams.Store, t *teams.Team) error {
		t.ShareSetup, t.SetupAsked = on, true
		return nil
	})
	if err != nil {
		return err
	}
	t := &saved
	sharedMu.Lock()
	delete(shared, teamID)
	sharedMu.Unlock()
	if t.MemberID == "" {
		return nil // shared once you've chosen who you are
	}
	if on {
		return shareSetup(*t, health.SetupFrom(liveenv.Read(), time.Now()))
	}
	st, err := setupStore(*t)
	if err != nil {
		return err
	}
	return st.DeleteSetup(t.MemberID)
}

// teamSetups are the setups the team's other members share, by name.
func teamSetups(t *teams.Team) (map[string]health.Setup, error) {
	st, err := setupStore(*t)
	if err != nil {
		return nil, err
	}
	raw, err := st.Setups()
	if err != nil {
		return nil, err
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if ms, err := b.Members(); err == nil {
		for _, m := range ms {
			names[m.ID] = m.Name
		}
	}
	out := map[string]health.Setup{}
	for id, data := range raw {
		if id == t.MemberID {
			continue
		}
		var s health.Setup
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		name := names[id]
		if name == "" {
			name = "?"
		}
		out[name] = s
	}
	return out, nil
}
