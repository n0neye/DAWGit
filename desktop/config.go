package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// appConfig is the desktop app's own settings: which project folders it shows.
type appConfig struct {
	Projects []string `json:"projects"`
}

func configPath() string {
	if dir := os.Getenv("DAWGIT_CONFIG_DIR"); dir != "" { // tests, demos
		return filepath.Join(dir, "desktop.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "DAWGit", "desktop.json")
}

func loadConfig() *appConfig {
	c := &appConfig{}
	if data, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(data, c)
	}
	return c
}

func (c *appConfig) save() error {
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(configPath(), data, 0o644)
}

func (c *appConfig) add(root string) {
	for _, p := range c.Projects {
		if p == root {
			return
		}
	}
	c.Projects = append(c.Projects, root)
	sort.Strings(c.Projects)
}

func (c *appConfig) remove(root string) {
	out := c.Projects[:0]
	for _, p := range c.Projects {
		if p != root {
			out = append(out, p)
		}
	}
	c.Projects = out
}
