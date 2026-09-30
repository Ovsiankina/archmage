package main

import (
	"slices"
	"strings"
)

// invocation is what we understood of a pacman command line: enough to tell
// which operation runs, whether it needs root, and what shape it will print.
type invocation struct {
	op      byte           // one of DFQRSTU, 0 if none was given
	flags   map[string]int // option -> times given, under its short name if it has one
	targets []string
	ok      bool // false if pacman would not read these arguments the way we did
}

const (
	operations  = "DFQRSTU"
	shortOpts   = operations + "Vbcdefghiklmnopqrstuvwxy"
	shortValued = "br" // -b <path>, -r <path>
)

// longOpts maps pacman's long options to the name we file them under.
var longOpts = map[string]string{
	"database": "D", "files": "F", "query": "Q", "remove": "R", "sync": "S",
	"deptest": "T", "upgrade": "U", "version": "V", "help": "h",

	"cascade": "c", "changelog": "c", "clean": "c",
	"deps": "d", "nodeps": "d",
	"explicit": "e",
	"groups":   "g",
	"info":     "i",
	"check":    "k",
	"list":     "l",
	"foreign":  "m",
	"native":   "n", "nosave": "n",
	"owns": "o",
	"file": "p", "print": "p",
	"quiet":     "q",
	"recursive": "s", "search": "s",
	"unrequired": "t",
	"sysupgrade": "u", "unneeded": "u", "upgrades": "u",
	"verbose":      "v",
	"downloadonly": "w",
	"regex":        "x",
	"refresh":      "y",

	"asdeps": "asdeps", "asexplicit": "asexplicit", "confirm": "confirm",
	"dbonly": "dbonly", "debug": "debug",
	"disable-download-timeout":   "disable-download-timeout",
	"disable-sandbox":            "disable-sandbox",
	"disable-sandbox-filesystem": "disable-sandbox-filesystem",
	"disable-sandbox-syscalls":   "disable-sandbox-syscalls",
	"machinereadable":            "machinereadable",
	"needed":                     "needed", "noconfirm": "noconfirm",
	"noprogressbar": "noprogressbar", "noscriptlet": "noscriptlet",
}

// longValued are the long options whose value is the next argument.
var longValued = []string{
	"arch", "ask", "assume-installed", "cachedir", "color", "config", "dbpath",
	"gpgdir", "hookdir", "ignore", "ignoregroup", "logfile", "overwrite",
	"print-format", "root", "sysroot",
}

// parseArgs reads args the way pacman's getopt does.
func parseArgs(args []string) invocation {
	in := invocation{flags: map[string]int{}, ok: true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			in.targets = append(in.targets, args[i+1:]...)
			return in
		case strings.HasPrefix(a, "--"):
			name, _, inline := strings.Cut(a[2:], "=")
			if slices.Contains(longValued, name) {
				in.flags[name]++
				if !inline {
					i++
				}
			} else if key, known := longOpts[name]; known {
				in.set(key)
			} else {
				in.ok = false
			}
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				c := a[j]
				if strings.IndexByte(shortOpts, c) < 0 {
					in.ok = false
					break
				}
				if strings.IndexByte(shortValued, c) >= 0 {
					if j == len(a)-1 {
						i++ // the value is the next argument, not the rest of this one
					}
					break
				}
				in.set(string(c))
			}
		default:
			in.targets = append(in.targets, a)
		}
	}
	return in
}

func (in *invocation) set(key string) {
	if len(key) == 1 && strings.Contains(operations, key) {
		if in.op != 0 && in.op != key[0] {
			in.ok = false // pacman: "only one operation may be used at a time"
		}
		in.op = key[0]
		return
	}
	in.flags[key]++
}

func (in invocation) has(flag string) bool { return in.flags[flag] > 0 }

// needsRoot mirrors needs_root() in pacman's own source. A command line we
// did not understand is never elevated: pacman gets to refuse it as we are.
func (in invocation) needsRoot() bool {
	if !in.ok || in.has("h") || in.has("V") {
		return false
	}
	if in.has("sysroot") {
		return true
	}
	switch in.op {
	case 'D':
		return !in.has("k")
	case 'R', 'U':
		return !in.has("p")
	case 'S':
		return in.has("c") || in.has("y") ||
			!(in.has("g") || in.has("i") || in.has("l") || in.has("s") || in.has("p"))
	case 'F':
		return in.has("y")
	}
	return false
}
