package cli

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dawgit/internal/livecheck"
	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/server"
	"dawgit/internal/teams"
	"dawgit/internal/version"
)

// parseArgs parses flags that may appear before or after positional args.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	data := fs.String("data", "dawgit-data", "folder for all server data (back this up)")
	addr := fs.String("addr", ":7331", "listen address")
	token := fs.String("token", os.Getenv("DAWGIT_TOKEN"), "team access token (default: generated and kept in <data>/token)")
	name := fs.String("name", "", "team name shown to members (kept in <data>/team.json)")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	st, err := server.OpenStorage(*data)
	if err != nil {
		return err
	}
	switch {
	case *name != "":
		if err := st.SetName(*name); err != nil {
			return err
		}
	case st.Name() == "":
		host, _ := os.Hostname()
		if err := st.SetName("Team on " + host); err != nil {
			return err
		}
	}
	if *token == "" {
		tokenFile := filepath.Join(*data, "token")
		if b, err := os.ReadFile(tokenFile); err == nil {
			*token = strings.TrimSpace(string(b))
		} else {
			*token = server.NewToken()
			if err := os.WriteFile(tokenFile, []byte(*token+"\n"), 0o600); err != nil {
				return err
			}
		}
	}
	host, port, _ := net.SplitHostPort(*addr)
	hosts := []string{host}
	if host == "" || host == "0.0.0.0" {
		hosts = lanIPs()
	}
	abs, _ := filepath.Abs(*data)
	fmt.Printf("%s server %s for team %q\n  data:  %s\n  token: %s\n\nteam members connect with:\n",
		version.Name(), version.Version, st.Name(), abs, *token)
	for _, h := range hosts {
		fmt.Printf("  dawgit remote http://%s:%s --token %s\n", h, port, *token)
	}
	return http.ListenAndServe(*addr, server.Handler(st, *token))
}

func lanIPs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
			out = append(out, n.IP.String())
		}
	}
	if len(out) == 0 {
		out = []string{"localhost"}
	}
	return out
}

func cmdRemote(args []string) error {
	fs := flag.NewFlagSet("remote", flag.ContinueOnError)
	token := fs.String("token", "", "team access token")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		t, err := r.Team()
		if err != nil {
			return err
		}
		fmt.Printf("team %q at %s\n", refreshTeamName(t.ID), t.Remote.Display())
		return nil
	}
	if err := r.SetRemote(pos[0], *token); err != nil {
		return err
	}
	t, _ := r.Team()
	fmt.Printf("connected to team %q (%s)\nnext: dawgit save -m \"message\" to share this project\n", t.Name, t.Remote.Display())
	return nil
}

// refreshTeamName picks up the name the team's server or storage gives now
// (unless the user renamed the team here) and returns the name to show.
func refreshTeamName(id string) string {
	store, err := teams.Load()
	if err != nil {
		return ""
	}
	t := store.Find(id)
	if t == nil {
		return ""
	}
	if b, err := remote.Open(t.Remote); err == nil {
		if info, err := b.Info(); err == nil && store.SyncName(id, info.Name) {
			store.Save()
		}
	}
	return t.Name
}

func cmdTeams(args []string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	if len(store.Teams) == 0 {
		fmt.Println("not connected to any team (use `dawgit remote` in a project, or `dawgit clone`)")
		return nil
	}
	for i := range store.Teams {
		store.Teams[i].Name = refreshTeamName(store.Teams[i].ID)
	}
	for _, t := range store.Teams {
		mark := "  "
		if t.ID == store.Current {
			mark = "* "
		}
		n := 0
		for k := range store.Projects {
			if strings.HasPrefix(k, t.ID+"/") {
				n++
			}
		}
		fmt.Printf("%s%-24s %s  (%d project(s) on this computer)\n", mark, t.Name, t.Remote.Display(), n)
	}
	return nil
}

func cmdClone(args []string) error {
	fs := flag.NewFlagSet("clone", flag.ContinueOnError)
	token := fs.String("token", os.Getenv("DAWGIT_TOKEN"), "team access token")
	author := fs.String("author", "", "your name on saved versions (default: OS user)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: dawgit clone <server-url> <project name or id> [folder] --token TOKEN")
	}
	dir := ""
	if len(pos) > 2 {
		dir = pos[2]
	}
	r, m, err := project.Clone(pos[0], *token, pos[1], dir, *author)
	if err != nil {
		return err
	}
	if release, err := r.Lock(0); err == nil {
		tidy(r)
		ensureRules(r)
		release()
	}
	fmt.Printf("downloaded %q into %s\n", r.Config.Name, r.Root)
	if m != nil {
		fmt.Printf("  latest version %s  %s (%s)\n", short(m.ID), m.Message, m.Author)
	}
	return nil
}

func strategyFlag(fs *flag.FlagSet) *string {
	return fs.String("strategy", "fail",
		"when you and others changed the same track or file: fail (ask), ours (keep yours), theirs (keep theirs), both (keep both)")
}

var openSet = livecheck.OpenSet

// liveOpenError explains that set ("?" if unknown) must be closed in Live.
func liveOpenError(set, why string) error {
	what := "a set of this project is open in Ableton Live"
	if set != "?" {
		what = fmt.Sprintf("%q is open in Ableton Live", set)
	}
	if why != "" {
		what = why + ", but " + what
	}
	return errors.New(what + ".\nSave and close it in Live, then run this again (or use --force if it is not open)")
}

// guardLive refuses to rewrite sets while one is open in Live, unless forced.
func guardLive(r *project.Repo, force bool) error {
	set := ""
	if !force {
		set = openSet(r.Root)
	}
	if set == "" {
		return nil
	}
	incoming, err := r.Incoming()
	if err != nil || !incoming {
		return err
	}
	return liveOpenError(set, "others saved new versions that must be merged into your files")
}

func printMerge(res *project.SyncResult) {
	for _, l := range res.MergeLog {
		fmt.Println("  merged " + l)
	}
	for _, n := range res.Relinked {
		fmt.Println("  relinked " + n)
	}
}

func explainConflict(err error) error {
	var c *project.MergeConflictError
	if errors.As(err, &c) {
		return fmt.Errorf("%w\nchoose how to resolve: --strategy ours | theirs | both", err)
	}
	return err
}

func cmdSave(args []string) error {
	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	msg := fs.String("m", "", "what changed")
	strategy := strategyFlag(fs)
	force := fs.Bool("force", false, "merge even while Ableton Live is running")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	if *msg == "" {
		return errors.New(`a message is required: dawgit save -m "what changed"`)
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	if r.Config.Remote != nil {
		if err := guardLive(r, *force); err != nil {
			return err
		}
	}
	defer tidy(r)
	m, res, err := r.Save(*msg, project.Strategy(*strategy))
	if errors.Is(err, project.ErrNoRemote) {
		if m != nil {
			fmt.Printf("saved version %s locally (not connected to a team; see `dawgit remote`)\n", short(m.ID))
			return nil
		}
		fmt.Println("nothing changed")
		return nil
	}
	if err != nil {
		return explainConflict(err)
	}
	if m != nil {
		fmt.Printf("saved version %s  %s\n", short(m.ID), m.Message)
	}
	printMerge(res)
	switch res.Action {
	case "published":
		fmt.Println("shared with the team")
		if len(res.MergeLog) > 0 {
			fmt.Println("others' changes were merged into your files: reopen the set in Live")
		}
	case "up-to-date":
		if m == nil {
			fmt.Println("nothing changed")
		}
	case "fast-forward":
		fmt.Println("you had nothing new; updated to the team's latest version")
	}
	return nil
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	preview := fs.Bool("preview", false, "show what the team changed, change nothing")
	strategy := strategyFlag(fs)
	force := fs.Bool("force", false, "update even while Ableton Live is running")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	if *preview {
		p, err := r.PreviewUpdate()
		if err != nil {
			return err
		}
		printPreview(p, "the team")
		return nil
	}
	if err := guardLive(r, *force); err != nil {
		return err
	}
	defer tidy(r)
	res, err := r.Update(project.Strategy(*strategy))
	if err != nil {
		return explainConflict(err)
	}
	switch res.Action {
	case "up-to-date":
		fmt.Println("already up to date")
	case "ahead":
		fmt.Println("you have versions the team does not have yet: dawgit save -m \"...\" to share them")
	case "fast-forward", "merged":
		fmt.Printf("updated to %s\n", short(res.To))
		printMerge(res)
		if res.Action == "merged" {
			fmt.Println("your versions and the team's were merged; run `dawgit save` to share the result")
		}
	}
	return nil
}

func cmdConnectionCode(args []string) error {
	fs := flag.NewFlagSet("connection-code", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "S3 endpoint, e.g. https://<account>.r2.cloudflarestorage.com")
	bucket := fs.String("bucket", "", "bucket name")
	prefix := fs.String("prefix", "dawgit", "folder inside the bucket")
	region := fs.String("region", "auto", "region")
	access := fs.String("access-key", "", "access key id (one per team member)")
	secret := fs.String("secret-key", "", "secret access key")
	name := fs.String("name", "", "team name shown to members (stored in the bucket)")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	cfg, err := remote.Storage{Endpoint: *endpoint, Bucket: *bucket, Folder: *prefix, Region: *region,
		AccessKey: *access, SecretKey: *secret}.Config()
	if err != nil {
		return err
	}
	if err := remote.Check(cfg); err != nil {
		return err
	}
	b, err := remote.Open(cfg)
	if err != nil {
		return err
	}
	if *name != "" {
		if err := b.SetInfo(remote.TeamInfo{Name: *name}); err != nil {
			return fmt.Errorf("could not save the team name: %w", err)
		}
	}
	info, _ := b.Info()
	if info.Name == "" {
		fmt.Println("tip: add --name \"Team name\" once so members see a name instead of the address")
	}
	fmt.Println("storage OK. Connection code (contains the key: share it privately):")
	fmt.Println()
	fmt.Println(remote.EncodeConnectionCode(cfg))
	return nil
}
