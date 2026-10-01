package project

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"dawgit/internal/manifest"
	"dawgit/internal/remote"
	"dawgit/internal/store"
)

// StageVerifying: checking every stored file (Verify).
const StageVerifying = "verifying"

// Problem is something Verify found wrong.
type Problem struct {
	Kind   string // "version", "folder-list" (a version's tree) or "file"
	ID     string // the version id, tree hash or file content hash
	Path   string // for a file: where a version has it
	Detail string // what is wrong, for people
	Fixed  bool   // repaired (with repair)
	How    string // how it was repaired, or why it can't be
}

// VerifyReport is what Verify checked and found.
type VerifyReport struct {
	Versions int   // version records checked
	Folders  int   // trees checked
	Files    int   // stored files re-read
	Bytes    int64 // their size
	// TeamChecked: the team's storage was asked about the files only it
	// has (false without a team, or when it couldn't be reached).
	TeamChecked bool
	Repaired    bool // repair was asked for
	Problems    []Problem
}

// verifyWorkers re-read stored files in parallel.
const verifyWorkers = 4

// Verify checks the project's history: every version record and folder list
// against its hash, every stored file by reading it again, and that every
// file a version needs is here or in the team's storage. With repair it
// fixes what it can: a damaged or lost file comes back from the project
// folder (a file with the same content) or the team's storage, a version
// record or folder list from the team's storage.
func (r *Repo) Verify(repair bool) (*VerifyReport, error) {
	rep := &VerifyReport{Repaired: repair}
	var c remote.Backend
	if r.Config.Remote != nil {
		c, _ = r.Client() // offline or no keys: checked here only
	}

	// Version records.
	ids, err := r.storedVersions()
	if err != nil {
		return nil, err
	}
	var records []*Manifest
	for _, id := range ids {
		rep.Versions++
		data, err := os.ReadFile(r.snapshotPath(id))
		var m *Manifest
		if err == nil {
			m, err = manifest.Parse(id, data)
		}
		if err == nil {
			records = append(records, m)
			continue
		}
		p := Problem{Kind: "version", ID: id, Detail: "the version record is damaged"}
		if repair {
			if m, ok := r.refetchVersion(c, id); ok {
				records = append(records, m)
				p.Fixed, p.How = true, "downloaded again from the team"
			} else {
				p.How = "not in the team's storage either"
			}
		}
		rep.Problems = append(rep.Problems, p)
	}
	for _, id := range []string{r.Head(), r.Config.Tip} {
		if id != "" && !r.HasSnapshot(id) {
			rep.Problems = append(rep.Problems, Problem{Kind: "version", ID: id,
				Detail: "the version the project is on is missing"})
		}
	}

	// Folder lists (trees), read from disk (not the cache), and what files
	// every version needs, by content hash, with a path to name them.
	paths := map[string]string{}
	for _, m := range records {
		for _, f := range m.Files {
			paths[f.Hash] = f.Path
		}
		for _, f := range m.External {
			paths[f.Hash] = f.Path
		}
	}
	seen := map[string]bool{}
	var walk func(h, prefix string)
	walk = func(h, prefix string) {
		if seen[h] {
			return
		}
		seen[h] = true
		rep.Folders++
		entries, err := r.readTreeFromDisk(h)
		if err != nil {
			p := Problem{Kind: "folder-list", ID: h, Path: prefix, Detail: "a version's list of a folder is " + missingOrDamaged(err)}
			if repair {
				if c != nil && r.refetchTree(c, h) == nil {
					entries, err = r.readTreeFromDisk(h)
				}
				if err == nil {
					p.Fixed, p.How = true, "downloaded again from the team"
				} else {
					p.How = "not in the team's storage either"
				}
			}
			rep.Problems = append(rep.Problems, p)
			if err != nil {
				return
			}
		}
		trees.put(h, entries)
		for _, e := range entries {
			if e.Dir {
				walk(e.Hash, prefix+e.Name+"/")
			} else if _, ok := paths[e.Hash]; !ok {
				paths[e.Hash] = prefix + e.Name
			}
		}
	}
	for _, m := range records {
		if m.Tree != "" {
			walk(m.Tree, "")
		}
	}

	// Stored files, read again.
	stored, err := r.Store.List()
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(stored))
	for h, n := range stored {
		hashes = append(hashes, h)
		rep.Bytes += n
	}
	sort.Strings(hashes)
	var (
		mu      sync.Mutex
		damaged []string
		done    atomic.Int64
	)
	r.report(StageVerifying, 0, len(hashes))
	inParallelN(verifyWorkers, hashes, func(h string) error {
		got, _, err := store.HashFile(r.Store.Path(h))
		if err != nil || got != h {
			mu.Lock()
			damaged = append(damaged, h)
			mu.Unlock()
		}
		n := done.Add(1)
		if n%50 == 0 || int(n) == len(hashes) {
			mu.Lock()
			r.report(StageVerifying, int(n), len(hashes))
			mu.Unlock()
		}
		return nil
	})
	rep.Files = len(hashes)
	bad := map[string]bool{}
	for _, h := range damaged {
		bad[h] = true
	}

	// Files versions need that aren't here (or are damaged).
	remoteOnly := r.remoteOnly()
	var onTeam []string // here only as "in the team's storage"
	var lost []string   // damaged, or neither here nor in the team's storage
	for h := range paths {
		switch {
		case bad[h]:
			lost = append(lost, h)
		case stored[h] != 0 || r.Store.Has(h):
		case remoteOnly[h]:
			onTeam = append(onTeam, h)
		case r.localCopy(h) != "":
			// in the project folder only (e.g. a sample used as it is)
		default:
			lost = append(lost, h)
		}
	}
	teamLacks := map[string]bool{}
	if c != nil && len(onTeam)+len(lost) > 0 {
		missing, err := c.MissingObjects(append(append([]string(nil), onTeam...), lost...))
		if err == nil {
			rep.TeamChecked = true
			for _, h := range missing {
				teamLacks[h] = true
			}
		}
	}
	if c != nil && len(onTeam)+len(lost) == 0 {
		rep.TeamChecked = true
	}
	for _, h := range onTeam {
		if !teamLacks[h] {
			continue
		}
		p := Problem{Kind: "file", ID: h, Path: paths[h], Detail: "only in the team's storage, which doesn't have it"}
		if repair {
			r.repairFile(c, h, &p, true)
		}
		rep.Problems = append(rep.Problems, p)
	}
	for _, h := range lost {
		detail := "missing"
		if bad[h] {
			detail = "damaged"
		}
		p := Problem{Kind: "file", ID: h, Path: paths[h], Detail: "the stored copy is " + detail}
		if repair {
			r.repairFile(c, h, &p, !rep.TeamChecked || teamLacks[h])
		}
		rep.Problems = append(rep.Problems, p)
	}
	// Damaged files no version needs any more: just removed.
	for _, h := range damaged {
		if _, needed := paths[h]; !needed && repair {
			store.Remove(r.Store.Path(h))
		}
	}
	if repair && r.remote != nil {
		if err := r.saveRemoteOnly(); err != nil {
			return rep, err
		}
	}
	sort.Slice(rep.Problems, func(i, j int) bool {
		a, b := rep.Problems[i], rep.Problems[j]
		if a.Kind != b.Kind {
			return a.Kind > b.Kind // versions, then folder lists, then files
		}
		return a.Path < b.Path
	})
	return rep, nil
}

// repairFile brings back the content h: from a file in the project folder
// with that content, or (unless teamLacks) by relying on the team's copy.
// A project-folder copy also goes up to the team when the team lacks it.
func (r *Repo) repairFile(c remote.Backend, h string, p *Problem, teamLacks bool) {
	if src := r.sourcesByHash()[h]; src != "" {
		if got, _, err := store.HashFile(src); err == nil && got == h {
			store.Remove(r.Store.Path(h)) // a damaged copy
			if _, _, err := r.Store.PutFile(src); err == nil {
				delete(r.remoteOnly(), h)
				p.Fixed, p.How = true, "copied again from the project folder"
				if c != nil && teamLacks {
					if f, err := os.Open(src); err == nil {
						if c.PutObject(h, f) == nil {
							p.How += ", and uploaded to the team"
						}
						f.Close()
					}
				}
				return
			}
		}
	}
	if c != nil && !teamLacks {
		store.Remove(r.Store.Path(h))
		r.remoteOnly()[h] = true
		p.Fixed, p.How = true, "the team's copy is used (downloaded when needed)"
		return
	}
	p.How = "no copy left: in the project folder, here or in the team's storage"
}

func missingOrDamaged(err error) string {
	if os.IsNotExist(err) {
		return "missing"
	}
	return "damaged"
}

// readTreeFromDisk reads and checks a stored tree, bypassing the cache.
func (r *Repo) readTreeFromDisk(h string) ([]manifest.TreeEntry, error) {
	data, err := os.ReadFile(r.treePath(h))
	if err != nil {
		return nil, err
	}
	return manifest.ParseTree(h, data)
}

func (r *Repo) refetchTree(c remote.Backend, h string) error {
	body, err := c.GetObject(h)
	if err != nil {
		return err
	}
	defer body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(body); err != nil {
		return err
	}
	if _, err := manifest.ParseTree(h, buf.Bytes()); err != nil {
		return err
	}
	return store.WriteAtomic(r.treePath(h), bytes.NewReader(buf.Bytes()))
}

func (r *Repo) refetchVersion(c remote.Backend, id string) (*Manifest, bool) {
	if c == nil || r.Config.ProjectID == "" {
		return nil, false
	}
	data, err := c.GetSnapshot(r.Config.ProjectID, id)
	if err != nil {
		return nil, false
	}
	m, err := manifest.Parse(id, data)
	if err != nil || r.storeSnapshot(id, data) != nil {
		return nil, false
	}
	return m, true
}

// Summary is one line about a report, for people.
func (rep *VerifyReport) Summary() string {
	open := 0
	for _, p := range rep.Problems {
		if !p.Fixed {
			open++
		}
	}
	checked := fmt.Sprintf("%d versions, %d folder lists and %d stored files checked", rep.Versions, rep.Folders, rep.Files)
	switch {
	case len(rep.Problems) == 0:
		return "No problems: " + checked + "."
	case !rep.Repaired:
		return fmt.Sprintf("%d problem(s) found: %s.", len(rep.Problems), checked)
	case open == 0:
		return fmt.Sprintf("%d problem(s), all repaired: %s.", len(rep.Problems), checked)
	}
	return fmt.Sprintf("%d problem(s), %d not repaired: %s.", len(rep.Problems), open, checked)
}
