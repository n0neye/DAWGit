package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Stages reported while saving or downloading a project.
const (
	StageScanning    = "scanning"    // looking for changed files
	StageStoring     = "storing"     // copying changed files into the history
	StageUploading   = "uploading"   // sending files to the team
	StageDownloading = "downloading" // fetching files from the team
	StageExporting   = "exporting"   // writing a version to another folder
)

// Progress describes a long-running step: Done of Total items (Total is 0
// when unknown).
type Progress struct {
	Stage string
	Done  int
	Total int
}

// report calls r.OnProgress when set.
func (r *Repo) report(stage string, done, total int) {
	if r.OnProgress != nil {
		r.OnProgress(Progress{Stage: stage, Done: done, Total: total})
	}
}

// SetsSignature changes whenever a set in the project root is saved or the
// workspace moves to another version. It only stats files, so it is cheap
// enough to poll every second.
func (r *Repo) SetsSignature() string {
	sets, _ := filepath.Glob(filepath.Join(r.Root, "*.als"))
	sort.Strings(sets)
	var b strings.Builder
	b.WriteString(r.Head())
	for _, s := range sets {
		if fi, err := os.Stat(s); err == nil {
			fmt.Fprintf(&b, "|%s:%d:%d", filepath.Base(s), fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
}
