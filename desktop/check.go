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
	return health.Check(inv, liveenv.Read()), nil
}
