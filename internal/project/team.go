package project

import (
	"errors"
	"sync"

	"dawgit/internal/remote"
)

// TeamView is what the team has for this project: its branch heads, with
// their versions downloaded. BranchesFrom and IncomingFrom read it without
// the network.
type TeamView struct {
	Heads map[string]string
}

// FetchTeam asks the team for its branches. It only adds version files
// (atomically), so it runs without the project lock.
func (r *Repo) FetchTeam() (*TeamView, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	heads, err := c.Branches(r.Config.ProjectID)
	if errors.Is(err, remote.ErrNotFound) {
		heads, err = map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for _, head := range heads {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			r.fetchSnapshots(c, h)
		}(head)
	}
	wg.Wait()
	return &TeamView{Heads: heads}, nil
}
