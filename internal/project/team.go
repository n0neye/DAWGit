package project

import (
	"errors"
	"sync"

	"dawgit/internal/remote"
)

// TeamView is what the team has for this project: its branch heads (with
// their versions downloaded) and the teammates' workspaces with unsaved
// edits. BranchesFrom and IncomingFrom read it without the network.
type TeamView struct {
	Heads map[string]string
	Mates []WorkspaceState
}

// FetchTeam asks the team for its branches and teammates at once. It only
// adds version files (atomically), so it runs without the project lock.
func (r *Repo) FetchTeam() (*TeamView, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	v := &TeamView{}
	var wg sync.WaitGroup
	var headsErr, matesErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		v.Heads, headsErr = c.Branches(r.Config.ProjectID)
		if errors.Is(headsErr, remote.ErrNotFound) {
			v.Heads, headsErr = map[string]string{}, nil
		}
		if headsErr != nil {
			return
		}
		var fw sync.WaitGroup
		for _, head := range v.Heads {
			fw.Add(1)
			go func(h string) {
				defer fw.Done()
				r.fetchSnapshots(c, h)
			}(head)
		}
		fw.Wait()
	}()
	go func() {
		defer wg.Done()
		v.Mates, matesErr = r.Teammates()
	}()
	wg.Wait()
	if headsErr != nil {
		return nil, headsErr
	}
	if matesErr != nil {
		return nil, matesErr
	}
	return v, nil
}
