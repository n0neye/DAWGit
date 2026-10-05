package remote

import (
	"strings"
	"testing"
	"time"

	"dawgit/internal/remote/s3test"
)

func TestBranchLog(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	b, _ := NewS3(fake.URL, "band", "team", "auto", "k", "s")
	b.SetActor(strings.Repeat("a", 32))
	clock := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	defer func(f func() time.Time) { branchLogNow = f }(branchLogNow)
	branchLogNow = func() time.Time { clock = clock.Add(time.Second); return clock }
	pid := strings.Repeat("1", 32)
	v1, v2 := strings.Repeat("c", 64), strings.Repeat("d", 64)
	if err := b.UpdateBranch(pid, "main", "", v1); err != nil {
		t.Fatal(err)
	}
	b.UpdateBranch(pid, "main", v1, v2)
	b.UpdateBranch(pid, "idea", "", v1)
	b.UpdateBranch(pid, "idea", v1, "")
	if err := b.UpdateBranch(pid, "main", v1, v2); err == nil {
		t.Fatal("a move from the wrong version went through")
	}
	all, err := b.BranchLog(pid, "")
	if err != nil || len(all) != 4 {
		t.Fatalf("log: %+v %v", all, err)
	}
	main, _ := b.BranchLog(pid, "main")
	if len(main) != 2 || main[0].From != "" || main[0].To != v1 || main[1].From != v1 || main[1].To != v2 ||
		main[1].By != strings.Repeat("a", 32) || !main[0].Time.Before(main[1].Time) {
		t.Fatalf("main: %+v", main)
	}
	if idea, _ := b.BranchLog(pid, "idea"); len(idea) != 2 || idea[1].To != "" {
		t.Fatalf("idea: %+v", idea)
	}
}
