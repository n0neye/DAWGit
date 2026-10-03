//go:build !windows

package liveenv

func fileVersion(string) string { return "" }

func extraInstalls() []string { return nil }
