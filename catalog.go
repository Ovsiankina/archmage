package main

import (
	"os/exec"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
)

// command is one pacman command and how archmage paints it.
type command struct {
	flags string  // "-Qi": the operation, then the mode flags that select it, sorted
	args  string  // what it takes, for display only
	desc  string  // what pacman does
	as    string  // what it is painted as, for display only
	paint painter // nil: runs live in a pty, where only bars and prefixes are repainted
}

// live is what every command that changes the system is painted as.
const live = "progress bars"

// commands is every pacman command. `archmage` with no arguments prints it.
var commands = []command{
	{"-Q", "[pkg…]", "list installed packages (-e explicit, -d deps, -m foreign, -n native, -t unrequired)", "table", paintPackages},
	{"-Qi", "[pkg…]", "show details of installed packages (-p: of a package file)", "cards", paintInfo},
	{"-Ql", "[pkg…]", "list the files a package owns", "tree", paintFiles},
	{"-Qs", "[regex…]", "search installed packages", "table", paintSearch},
	{"-Qo", "<file…>", "find the package that owns a file", "table", paintOwner},
	{"-Qg", "[group…]", "list the installed members of a group", "table", paintGroups},
	{"-Qk", "[pkg…]", "check that a package's files are present (-kk: unaltered)", "table", paintCheck},
	{"-Qu", "", "list outdated packages", "table", paintUpgrades},
	{"-Qc", "<pkg…>", "show a package's changelog", "box", paintBox},

	{"-S", "<pkg…>", "install packages (-w: download only)", live, nil},
	{"-Sy", "", "refresh the package databases", live, nil},
	{"-Syu", "", "upgrade the whole system", live, nil},
	{"-Sc", "", "clean the package cache (-cc: all of it)", live, nil},
	{"-Ss", "[regex…]", "search the repositories", "table", paintSearch},
	{"-Si", "[pkg…]", "show details of repository packages", "cards", paintInfo},
	{"-Sl", "[repo…]", "list the packages in a repository", "table", paintRepo},
	{"-Sg", "[group…]", "list package groups and their members", "table", paintGroups},
	{"-Sp", "<pkg…>", "print what would be installed", "table", paintTargets},

	{"-R", "<pkg…>", "remove packages (-s unneeded deps, -n config files, -c dependants)", live, nil},
	{"-Rp", "<pkg…>", "print what would be removed", "table", paintTargets},

	{"-U", "<file…>", "install package files or URLs", live, nil},
	{"-Up", "<file…>", "print what would be installed", "table", paintTargets},

	{"-F", "<file…>", "find the repository package that owns a file", "table", paintFileSearch},
	{"-Fx", "<regex…>", "the same, matching with regular expressions", "table", paintFileSearch},
	{"-Fl", "<pkg…>", "list the files of a repository package", "tree", paintFiles},
	{"-Fy", "", "refresh the file databases", live, nil},

	{"-D", "--asdeps <pkg…>", "mark packages as installed as dependencies", live, nil},
	{"-D", "--asexplicit <pkg…>", "mark packages as explicitly installed", live, nil},
	{"-Dk", "", "check the package database for errors (-kk: sync databases too)", "checklist", paintOK},

	{"-T", "[dep…]", "print the dependencies that are not satisfied", "list", paintMissing},
	{"-V", "", "show pacman's version", "box", paintVersion},
	{"-h", "", "show help (after an operation: its options)", "table", paintHelp},
}

// shapes are, per operation, the flags that change the shape of what is
// printed. Any combination without a command of its own is left unpainted.
var shapes = map[byte]string{
	'D': "k",
	'F': "lqx",
	'Q': "cgikloqsu",
	'R': "p",
	'S': "gilpqs",
	'U': "p",
}

// route finds the command that paints in, or nil if its output is left alone.
func route(in invocation) *command {
	if in.has("machinereadable") {
		return nil
	}
	flags := "-" + string(in.op)
	for _, f := range shapes[in.op] {
		if in.has(string(f)) {
			flags += string(f)
		}
	}
	i := slices.IndexFunc(commands, func(c command) bool { return c.flags == flags && c.paint != nil })
	if i < 0 {
		return nil
	}
	return &commands[i]
}

// overview is what `archmage` prints when it is given nothing to do.
func overview() string {
	var status [][]string
	for _, b := range backends {
		state := okStyle.Render("ready")
		if b.wip {
			state = warnStyle.Render("WIP")
		}
		path, err := exec.LookPath(b.name)
		if err != nil {
			path = "not installed"
		}
		status = append(status, []string{b.name, state, path})
	}

	var rows [][]string
	for _, c := range commands {
		rows = append(rows, []string{strings.TrimSpace(c.flags + " " + c.args), c.desc, c.as})
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("archmage")+dimStyle.Render("  pacman, painted with charm"),
		grid([]string{"Backend", "Status", "Path"}, status),
		grid([]string{"Command", "Does", "Painted as"}, rows),
		dimStyle.Render(" usage: archmage <operation> [...]   every argument goes to pacman untouched"),
	)
}

// wipNotice warns that b is not painted yet.
func wipNotice(b backend) string {
	return warnStyle.Render("WIP") + dimStyle.Render(" "+b.name+" is not painted yet, running it as is")
}
