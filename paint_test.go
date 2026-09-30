package main

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// Every painter must recognise what pacman prints for its command, and
// refuse anything else: a refusal is what keeps unknown output untouched.
func TestPainters(t *testing.T) {
	tests := []struct {
		name   string
		paint  painter
		out    string
		want   []string // all of these show up in the painting
		refuse string   // output of another shape
	}{
		{"packages", paintPackages,
			"bash 5.3.20-1\npacman 7.1.0-2\n",
			[]string{"Package", "bash", "5.3.20-1", "2 packages"},
			"bash\n"},
		{"upgrades", paintUpgrades,
			"discord 1:1.0.159-1 -> 1:1.0.160-1\nlinux 6.1-1 -> 6.2-1 [ignored]\n",
			[]string{"Available", "1:1.0.160-1", "[ignored]", "2 upgrades"},
			"discord 1:1.0.159-1\n"},
		{"repo", paintRepo,
			"core acl 2.4.0-1 [installed]\ncore bash 5.3-1 [installed: 5.2-1]\ncore amd-ucode 20260916-1\n",
			[]string{"Repo", "acl", "✓ 5.2-1", "3 packages"},
			"acl\n"},
		{"search", paintSearch,
			"core/pacman 7.1.0-2 (base-devel) [installed]\n    A package manager\nextra/pacoloco 1.9-1\n    Caching proxy\n",
			[]string{"Groups", "base-devel", "✓", "A package manager", "pacoloco"},
			"    no package above this line\n"},
		{"file search", paintFileSearch,
			"core/pacman 7.1.0-2 [installed]\n    usr/bin/pacman\n    usr/bin/pacman-conf\n",
			[]string{"Files", "usr/bin/pacman-conf"},
			"pacman usr/bin/pacman\n"},
		{"file owner", paintFileSearch,
			"usr/bin/pacman is owned by core/pacman 7.1.0-2\n",
			[]string{"File", "usr/bin/pacman", "core/pacman"},
			""},
		{"owner", paintOwner,
			"/usr/bin/ls is owned by coreutils 9.12-2\n",
			[]string{"/usr/bin/ls", "coreutils", "9.12-2"},
			"/usr/bin/ls est la propriété de coreutils 9.12-2\n"},
		{"groups", paintGroups,
			"kf6 attica\nkf6 baloo\n",
			[]string{"Group", "Package", "baloo", "2 packages"},
			"kf6 attica 1.0\n"},
		{"group names", paintGroups,
			"pro-audio\nkf6\n",
			[]string{"Group", "pro-audio", "2 groups"},
			""},
		{"check", paintCheck,
			"pacman: 426 total files, 0 missing files\nbash: 269 total files, 2 missing files\n",
			[]string{"Files", "Missing", "426", "2"},
			"pacman 426\n"},
		{"targets", paintTargets,
			"file:///var/cache/pacman/pkg/bash-5.3.20-1-x86_64.pkg.tar.zst\n",
			[]string{"Target", "bash-5.3.20-1", "1 target"},
			""},
		{"missing", paintMissing,
			"glibc>=99\n",
			[]string{"✗", "glibc>=99"},
			""},
		{"info", paintInfo,
			"Name            : bash\nURL             : https://www.gnu.org/\nOptional Deps   : a: for x\n                  b: for y\n\nName            : pacman\n",
			[]string{"Name", "bash", "https://www.gnu.org/", "a: for x", "b: for y", "pacman"},
			"    nothing but a continuation\n"},
		{"files", paintFiles,
			"which /usr/\nwhich /usr/bin/\nwhich /usr/bin/which\nwhich /usr/share/\ntree /usr/\n",
			[]string{"which", "usr/", "bin/", "share/", "tree", "1 file"},
			"which\n"},
		{"help", paintHelp,
			"usage:  pacman {-F --files} [options] [file(s)]\noptions:\n  -y, --refresh        download fresh package databases from the server\n                       (-yy to force a refresh even if up to date)\n      --disable-sandbox\n                       disables all sandbox features\n",
			[]string{"usage:", "-y, --refresh", "(-yy to force", "    --disable-sandbox", "disables all sandbox features"},
			"error: no operation specified\n"},
		{"operations", paintHelp,
			"usage:  pacman <operation> [...]\noperations:\n    pacman {-S --sync}     [options] [package(s)]\n\nuse 'pacman {-h --help}' with an operation for available options\n",
			[]string{"-S, --sync", "[options] [package(s)]", "with an operation"},
			""},
	}
	for _, tt := range tests {
		got, ok := tt.paint(tt.out)
		if !ok {
			t.Errorf("%s: refused pacman's output", tt.name)
			continue
		}
		got = ansi.Strip(got)
		for _, want := range tt.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %q is missing from\n%s", tt.name, want, got)
			}
		}
		if _, ok := tt.paint(tt.refuse); ok {
			t.Errorf("%s: painted %q", tt.name, tt.refuse)
		}
	}
}

func TestWrap(t *testing.T) {
	got := wrap("bash  coreutils  arch-install-scripts  curl", 22)
	want := "bash  coreutils\narch-install-scripts\ncurl"
	if got != want {
		t.Errorf("wrap = %q, want %q", got, want)
	}
}

func TestRestyle(t *testing.T) {
	// Without a terminal lipgloss drops every colour, and with them all that
	// tells a painted prefix from a plain one.
	profile := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.TrueColor
	defer func() { lipgloss.Writer.Profile = profile }()

	tests := []struct {
		in   string
		bol  bool
		want string
	}{
		{" vim [####----]  50%\r", true, " vim <bar>  50%\r"},
		// ILoveCandy, as pacman writes it: nothing eaten, half eaten, all eaten.
		{" vim [" + candyC + candy(" o  o  ") + "]   0%\r", true, " vim <bar>   0%\r"},
		{" vim [----" + candyc + candy("  o ") + "]  50%\r", true, " vim <bar>  50%\r"},
		{" vim [--------] 100%\r", true, " vim <bar> 100%\r"},
		{":: Proceed? [Y/n] ", true, "<::> Proceed? [Y/n] "},
		{"warning: a\r\nerror: b\r\n", true, "<warning:> a\r\n<error:> b\r\n"},
		{"\x1b[?25l:: Retrieving\r\n", true, "\x1b[?25l<::> Retrieving\r\n"},
		{":: not a line start\r\n:: a line start\r\n", false, ":: not a line start\r\n<::> a line start\r\n"},
		{"see foo:: bar, no error: here\r\n", true, "see foo:: bar, no error: here\r\n"},
		{"\x1b[1;34m::\x1b[0;1m already coloured\r\n", true, "\x1b[1;34m::\x1b[0;1m already coloured\r\n"},
	}
	for _, tt := range tests {
		got := string(restyle([]byte(tt.in), tt.bol))
		if ansi.StringWidth(got) != ansi.StringWidth(tt.in) {
			t.Errorf("restyle(%q) is %d cells wide, want %d", tt.in, ansi.StringWidth(got), ansi.StringWidth(tt.in))
		}
		// Collapse what was painted: a bar to "<bar>", a prefix to "<prefix>".
		got = barCells.ReplaceAllString(got, "<bar>")
		got = painted.ReplaceAllString(got, "<$1>")
		if got != tt.want {
			t.Errorf("restyle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The escape codes pacman wraps each cell of a candy bar in.
const (
	candyC = "\x1b[1;33mC\x1b[m"
	candyc = "\x1b[1;33mc\x1b[m"
)

func candy(pellets string) string {
	var b strings.Builder
	for _, c := range pellets {
		b.WriteString("\x1b[0;37m" + string(c) + "\x1b[m")
	}
	return b.String()
}

var (
	barCells = regexp.MustCompile(`(?:\x1b\[[0-9;]*m[█░▌]*\x1b\[m)+`) // however many colour runs, the empty one included
	painted  = regexp.MustCompile(`\x1b\[[0-9;]*m(::|error:|warning:)\x1b\[m`)
)
