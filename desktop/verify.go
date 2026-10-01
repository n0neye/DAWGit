package desktop

// VerifyResult is what checking a project's history found (VerifyProject).
type VerifyResult struct {
	Summary string `json:"summary"`
	// TeamChecked: the team's storage was asked about the files only it has;
	// InTeam: the project has a team (so it could have been).
	TeamChecked bool            `json:"teamChecked"`
	InTeam      bool            `json:"inTeam"`
	Problems    []VerifyProblem `json:"problems"`
}

type VerifyProblem struct {
	Kind   string `json:"kind"` // version | folder-list | file
	What   string `json:"what"` // a path, or a short id
	Detail string `json:"detail"`
	Fixed  bool   `json:"fixed"`
	How    string `json:"how"` // how it was repaired, or why it can't be
}

// VerifyProject checks the project's history (every version, folder list
// and stored file); with repair it brings back what it can.
func (a *App) VerifyProject(root string, repair bool) (*VerifyResult, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	rep, err := r.Verify(repair)
	if err != nil {
		return nil, err
	}
	out := &VerifyResult{Summary: rep.Summary(), TeamChecked: rep.TeamChecked, InTeam: r.Config.Remote != nil,
		Problems: []VerifyProblem{}}
	for _, p := range rep.Problems {
		what := p.Path
		if what == "" {
			what = p.ID[:min(10, len(p.ID))]
		}
		out.Problems = append(out.Problems, VerifyProblem{Kind: p.Kind, What: what, Detail: p.Detail, Fixed: p.Fixed, How: p.How})
	}
	return out, nil
}
