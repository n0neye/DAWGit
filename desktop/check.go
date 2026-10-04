package desktop

import (
	"dawgit/internal/health"
	"dawgit/internal/liveenv"
	"dawgit/internal/project"
)

// ProjectCheck looks at a project and this computer: for Live Sets, the
// Live version they need, their samples, plugins and packs, and whether
// this computer has them; and what a first version uploads. Read only.
func (a *App) ProjectCheck(root string) (*health.Report, error) {
	r, err := project.Open(root) // reads files only: no lock
	if err != nil {
		return nil, err
	}
	inv, err := r.Inventory()
	if err != nil {
		return nil, err
	}
	rep := health.Check(inv, liveenv.Read())
	if rep.Live != nil && len(rep.Live.Samples.Missing) > 0 {
		if spots, err := r.SampleSpots(); err == nil {
			for _, sp := range spots {
				if sp.Restorable() {
					rep.Live.Samples.Restorable++
				}
			}
		}
	}
	// Teammates who share their setup: can they open it?
	if t, err := r.Team(); err == nil && t.Remote.IsStorage() {
		rep.TeamSetups, rep.ShareSetup = true, t.ShareSetup
		if setups, err := teamSetups(t); err == nil {
			health.CheckTeam(rep, inv, setups)
		}
	}
	return rep, nil
}
