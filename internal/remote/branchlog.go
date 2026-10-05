package remote

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// The branch log: every time a branch moves (a share, a merge, a branch
// made or deleted), a record of it is added, never changed: who, when, from
// which version to which. A branch moved by mistake can be put back, and
// what happened can be read afterwards — storage keeps only where branches
// are now.
//
// One key per move (projects/<pid>/branchlog/<time>-<random>.json), so
// moves never contend for a record. Written after the move succeeded, as
// well as can be: a move isn't undone because its record didn't go up.
// DAWGits from before the log don't write it: their moves are missing.

// BranchMove is a branch moving.
type BranchMove struct {
	Branch string    `json:"branch"`
	From   string    `json:"from,omitempty"` // "" when it was made
	To     string    `json:"to,omitempty"`   // "" when it was deleted
	By     string    `json:"by,omitempty"`   // the member id ("" unknown)
	Time   time.Time `json:"time"`
}

// BranchLogger is a backend that keeps the branch log.
type BranchLogger interface {
	// BranchLog lists a project's branch moves (of branch, or of all with
	// ""), oldest first.
	BranchLog(pid, branch string) ([]BranchMove, error)
}

var _ BranchLogger = (*BucketBackend)(nil)

func branchLogDir(pid string) string { return projectDir(pid) + "branchlog/" }

// SetActor names who this backend's branch moves are by (a member id).
func (s *BucketBackend) SetActor(memberID string) { s.actor = memberID }

func (s *BucketBackend) logMove(pid, branch, from, to string) {
	now := branchLogNow().UTC()
	id := make([]byte, 4)
	rand.Read(id)
	m := BranchMove{Branch: branch, From: from, To: to, By: s.actor, Time: now}
	data, _ := json.Marshal(m)
	// The name sorts by time: 20261005T120000.123456789Z.
	key := branchLogDir(pid) + now.Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(id) + ".json"
	s.put(key, data)
}

var branchLogNow = time.Now // tests set the clock

func (s *BucketBackend) BranchLog(pid, branch string) ([]BranchMove, error) {
	if !validHex(pid, 32) {
		return nil, errors.New("invalid project id")
	}
	var out []BranchMove
	err := s.readAll(branchLogDir(pid), func(_ string, data []byte) {
		var m BranchMove
		if json.Unmarshal(data, &m) == nil && (branch == "" || strings.EqualFold(m.Branch, branch)) {
			out = append(out, m)
		}
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, err
}
