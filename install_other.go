//go:build !windows

package main

// macOS is installed by the .pkg into /usr/local/bin; nothing to move.
func installSelf() bool { return false }
