package profile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// A project's .gitignore files, read as git does: each applies to its
// folder; a deeper one wins, and within one the last matching line; "!"
// takes a file back (but not out of a folder left out: DAWGit doesn't look
// inside it, like git).

type giRule struct {
	pattern string
	negate  bool
	line    int
	raw     string
}

type gitignores struct {
	mu    sync.Mutex
	byDir map[string][]giRule // nil entry: no .gitignore there
}

func (p *Profile) gitignoreOf(dir string) []giRule {
	p.gi.mu.Lock()
	defer p.gi.mu.Unlock()
	if p.gi.byDir == nil {
		p.gi.byDir = map[string][]giRule{}
	}
	if rules, ok := p.gi.byDir[dir]; ok {
		return rules
	}
	rules := readGitignore(filepath.Join(p.root, filepath.FromSlash(dir), ".gitignore"))
	p.gi.byDir[dir] = rules
	return rules
}

func readGitignore(path string) []giRule {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []giRule
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		raw := strings.TrimSuffix(sc.Text(), "\r")
		line := raw
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Trailing spaces go, unless escaped.
		for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, `\ `) {
			line = line[:len(line)-1]
		}
		line = strings.ReplaceAll(line, `\ `, " ")
		r := giRule{line: n, raw: raw}
		switch {
		case strings.HasPrefix(line, "!"):
			r.negate, line = true, line[1:]
		case strings.HasPrefix(line, `\!`), strings.HasPrefix(line, `\#`):
			line = line[1:]
		}
		if line == "" || line == "/" || checkPattern(line) != nil {
			continue
		}
		r.pattern = line
		out = append(out, r)
	}
	return out
}

// gitignored decides rel by the .gitignore files of its folders (ok false
// when none says anything).
func (p *Profile) gitignored(rel string, isDir bool) (Decision, bool) {
	segs := strings.Split(rel, "/")
	for n := len(segs) - 1; n >= 0; n-- {
		dir := strings.Join(segs[:n], "/")
		rules := p.gitignoreOf(dir)
		sub := strings.Join(segs[n:], "/")
		for i := len(rules) - 1; i >= 0; i-- {
			r := rules[i]
			if !matchPath(r.pattern, sub, isDir) {
				continue
			}
			file := ".gitignore"
			if dir != "" {
				file = dir + "/.gitignore"
			}
			return Decision{Ignored: !r.negate, By: fmt.Sprintf("%s line %d: %s", file, r.line, strings.TrimSpace(r.raw))}, true
		}
	}
	return Decision{}, false
}
