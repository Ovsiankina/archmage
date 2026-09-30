package main

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// backend is a package manager archmage can sit in front of.
type backend struct {
	name string
	sudo bool // we elevate root operations ourselves; AUR helpers do that on their own
	wip  bool // not painted yet: runs untouched
}

var backends = []backend{
	{name: "pacman", sudo: true},
	{name: "paru", wip: true}, // WIP
	{name: "yay", wip: true},  // WIP
}

// pickBackend returns the backend named by ARCHMAGE_BACKEND, pacman by default.
func pickBackend() (backend, error) {
	name := cmp.Or(os.Getenv("ARCHMAGE_BACKEND"), "pacman")
	for _, b := range backends {
		if b.name == name {
			return b, nil
		}
	}
	return backend{}, fmt.Errorf("unknown backend %q (ARCHMAGE_BACKEND is one of pacman, paru, yay)", name)
}

// command builds the real command line, with sudo only when pacman needs root.
func (b backend) command(args []string, root bool) *exec.Cmd {
	argv := []string{b.name}
	if root && b.sudo && os.Geteuid() != 0 {
		argv = []string{"sudo", b.name}
	}
	// ARCHMAGE_CMD lets you wrap something else for testing, e.g. "fakeroot pacman".
	if override := strings.Fields(os.Getenv("ARCHMAGE_CMD")); len(override) > 0 {
		argv = override
	}
	return exec.Command(argv[0], append(argv[1:], args...)...)
}
