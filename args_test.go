package main

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args  string
		op    byte
		root  bool
		paint string // flags of the command that paints it, "" if none does
	}{
		{"-Q", 'Q', false, "-Q"},
		{"-Qe", 'Q', false, "-Q"},
		{"--query --explicit", 'Q', false, "-Q"},
		{"-Qi pacman", 'Q', false, "-Qi"},
		{"-Q --info pacman", 'Q', false, "-Qi"},
		{"-Qip pkg.tar.zst", 'Q', false, "-Qi"},
		{"-Qil pacman", 'Q', false, ""},
		{"-Qq", 'Q', false, ""},
		{"-Qdtq", 'Q', false, ""},
		{"-Qkk", 'Q', false, "-Qk"},
		{"-Qo /usr/bin/ls", 'Q', false, "-Qo"},
		{"-b /tmp/db -Q", 'Q', false, "-Q"},
		{"-Qb /tmp/db", 'Q', false, "-Q"},
		{"-Qbi", 'Q', false, "-Q"}, // "i" is the value of -b
		{"--dbpath /tmp/-S -Q", 'Q', false, "-Q"},
		{"--dbpath=/tmp/db -Q", 'Q', false, "-Q"},

		{"-S vim", 'S', true, ""},
		{"vim -S", 'S', true, ""},
		{"-Syu", 'S', true, ""},
		{"-Sc", 'S', true, ""},
		{"-Sw vim", 'S', true, ""},
		{"-Sys vim", 'S', true, ""},
		{"-Ss vim", 'S', false, "-Ss"},
		{"-Si vim", 'S', false, "-Si"},
		{"-Sl core", 'S', false, "-Sl"},
		{"-Sgg", 'S', false, "-Sg"},
		{"-Sp vim", 'S', false, "-Sp"},
		{"-Ssq vim", 'S', false, ""},
		{"-S --print-format %n --print vim", 'S', false, "-Sp"},

		{"-R vim", 'R', true, ""},
		{"-Rns vim", 'R', true, ""},
		{"-Rsp vim", 'R', false, "-Rp"},
		{"-U pkg.tar.zst", 'U', true, ""},
		{"-Up pkg.tar.zst", 'U', false, "-Up"},

		{"-F pacman.conf", 'F', false, "-F"},
		{"-Fx pacman", 'F', false, "-Fx"},
		{"-Fl pacman", 'F', false, "-Fl"},
		{"-Fl --machinereadable pacman", 'F', false, ""},
		{"-Fy", 'F', true, ""},

		{"-D --asdeps vim", 'D', true, ""},
		{"-Dk", 'D', false, "-Dk"},
		{"-T bash", 'T', false, "-T"},
		{"-Q --sysroot /mnt", 'Q', true, ""},

		{"-Sh", 'S', false, ""},
		{"-S --help", 'S', false, ""},
		{"-V", 0, false, ""},
		{"-Q -- -S", 'Q', false, "-Q"},
	}
	for _, tt := range tests {
		in := parseArgs(strings.Fields(tt.args))
		if !in.ok {
			t.Errorf("%q: not understood", tt.args)
		}
		if in.op != tt.op {
			t.Errorf("%q: op = %q, want %q", tt.args, in.op, tt.op)
		}
		if got := in.needsRoot(); got != tt.root {
			t.Errorf("%q: needsRoot = %v, want %v", tt.args, got, tt.root)
		}
		paint := ""
		if c := route(in); c != nil && !in.needsRoot() {
			paint = c.flags
		}
		if paint != tt.paint {
			t.Errorf("%q: painted by %q, want %q", tt.args, paint, tt.paint)
		}
	}
}

func TestParseArgsGivesUp(t *testing.T) {
	for _, args := range []string{"-QS", "-Q --sync", "-Qz", "--nonsense", "-S --quer"} {
		in := parseArgs(strings.Fields(args))
		if in.ok {
			t.Errorf("%q: understood, but pacman reads it differently or not at all", args)
		}
		if in.needsRoot() {
			t.Errorf("%q: would run under sudo without being understood", args)
		}
	}
}
