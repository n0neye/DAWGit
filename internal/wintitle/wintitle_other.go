//go:build !windows

package wintitle

// Of returns nothing outside Windows yet.
func Of(exe string) []string { return nil }

// Matching returns nothing outside Windows yet.
func Matching(match func(exe string) bool) []string { return nil }
