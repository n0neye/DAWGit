//go:build !windows

package wintitle

// Of returns nothing outside Windows yet.
func Of(exe string) []string { return nil }
