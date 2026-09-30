package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"
	"charm.land/lipgloss/v2/table"
	"charm.land/lipgloss/v2/tree"
	"golang.org/x/term"
)

var (
	purple = lipgloss.Color("#5A56E0")
	pink   = lipgloss.Color("#EE6FF8")
	gray   = lipgloss.Color("#808080")
	green  = lipgloss.Color("#04B575")
	yellow = lipgloss.Color("#FFD75F")
	red    = lipgloss.Color("#FF5F87")

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(pink)
	borderStyle = lipgloss.NewStyle().Foreground(purple)
	dimStyle    = lipgloss.NewStyle().Foreground(gray)
	okStyle     = lipgloss.NewStyle().Foreground(green)
	warnStyle   = lipgloss.NewStyle().Bold(true).Foreground(yellow)
	badStyle    = lipgloss.NewStyle().Foreground(red)
)

// A painter turns what pacman printed into charm components. It reports false
// when the text is not the shape it expects, and pacman's output is then
// printed exactly as it was written.
type painter func(out string) (string, bool)

// lines splits out into lines, without the newline that ends the last one.
func lines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// match applies re to every line of out and returns the groups it captured,
// or false as soon as one line does not match.
func match(out string, re *regexp.Regexp) ([][]string, bool) {
	var rows [][]string
	for _, l := range lines(out) {
		m := re.FindStringSubmatch(l)
		if m == nil {
			return nil, false
		}
		rows = append(rows, m[1:])
	}
	return rows, len(rows) > 0
}

func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

// columnStyle is how the cells under a given header are printed.
var columnStyle = map[string]lipgloss.Style{
	"Version":    dimStyle,
	"Repo":       dimStyle,
	"Groups":     dimStyle,
	"Installed":  okStyle,
	"Available":  okStyle,
	"Painted as": dimStyle,
}

// grid draws rows as a bordered table no wider than the terminal. Columns
// that are empty in every row are dropped.
func grid(headers []string, rows [][]string) string {
	var keep []int
	for c := range headers {
		for _, r := range rows {
			if r[c] != "" {
				keep = append(keep, c)
				break
			}
		}
	}
	pick := func(r []string) []string {
		out := make([]string, len(keep))
		for i, c := range keep {
			out[i] = r[c]
		}
		return out
	}
	headers = pick(headers)
	picked := make([][]string, len(rows))
	for i, r := range rows {
		picked[i] = pick(r)
	}

	t := newTable().Headers(headers...)
	return fit(t, len(headers)+1, headers, picked).StyleFunc(func(row, col int) lipgloss.Style {
		s := lipgloss.NewStyle().Padding(0, 1)
		if row == table.HeaderRow {
			return s.Inherit(titleStyle)
		}
		return s.Inherit(columnStyle[headers[col]])
	}).String()
}

func newTable() *table.Table {
	return table.New().Border(lipgloss.RoundedBorder()).BorderStyle(borderStyle)
}

// minWrap is the narrowest the last column of a table is wrapped to.
const minWrap = 20

// fit adds rows to t, keeping it no wider than the terminal. The last column
// holds the free text (a description, a list of files), so it is the one that
// is wrapped to make room; if that is not enough, every column gives some.
// borders is how many border columns t draws, headers may be nil.
func fit(t *table.Table, borders int, headers []string, rows [][]string) *table.Table {
	last := len(rows[0]) - 1
	widths := make([]int, last+1)
	for _, r := range append(rows, headers) {
		for c, cell := range r {
			widths[c] = max(widths[c], lipgloss.Width(cell))
		}
	}
	natural := borders
	for _, w := range widths {
		natural += w + 2 // the padding
	}

	if over := natural - termWidth(); over > 0 {
		if room := widths[last] - over; room >= minWrap {
			for _, r := range rows {
				r[last] = wrap(r[last], room)
			}
		} else {
			t.Width(termWidth())
		}
	}
	return t.Rows(rows...)
}

// wrap breaks s at spaces so that no line is wider than width. lipgloss.Wrap
// also breaks at hyphens, which here are in the middle of package names, so
// it only gets the words that are too long to fit on a line of their own.
func wrap(s string, width int) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		cur := ""
		for _, word := range strings.Split(line, " ") {
			if cur != "" && lipgloss.Width(cur+" "+word) <= width {
				cur += " " + word
				continue
			}
			if cur != "" {
				out = append(out, strings.TrimRight(cur, " "))
			}
			pieces := strings.Split(lipgloss.Wrap(word, width, ""), "\n")
			out = append(out, pieces[:len(pieces)-1]...)
			cur = pieces[len(pieces)-1]
		}
		out = append(out, strings.TrimRight(cur, " "))
	}
	return strings.Join(out, "\n")
}

// count is the gray line under a table: " 3 packages".
func count(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return "\n" + dimStyle.Render(fmt.Sprintf(" %d %s", n, noun))
}

// installed turns what pacman puts in "[installed]" or "[installed: 1.0-1]"
// into a tick, followed by the installed version when it is not the listed one.
func installed(note string) string {
	if note == "" {
		return ""
	}
	if _, version, differs := strings.Cut(note, ": "); differs {
		return "✓ " + version
	}
	return "✓"
}

// -Q, -Qe, -Qm, ...: "name version"
var packageRE = regexp.MustCompile(`^(\S+) (\S+)$`)

func paintPackages(out string) (string, bool) {
	rows, ok := match(out, packageRE)
	if !ok {
		return "", false
	}
	return grid([]string{"Package", "Version"}, rows) + count(len(rows), "package"), true
}

// -Qu: "name 1.0-1 -> 1.1-1", then "[ignored]" for a held back package
var upgradeRE = regexp.MustCompile(`^(\S+) (\S+) -> (\S+)(?: (\[.*\]))?$`)

func paintUpgrades(out string) (string, bool) {
	rows, ok := match(out, upgradeRE)
	if !ok {
		return "", false
	}
	return grid([]string{"Package", "Version", "Available", "Note"}, rows) + count(len(rows), "upgrade"), true
}

// -Sl: "repo name version", then "[installed]" or "[installed: version]"
var repoRE = regexp.MustCompile(`^(\S+) (\S+) (\S+)(?: \[(.*)\])?$`)

func paintRepo(out string) (string, bool) {
	rows, ok := match(out, repoRE)
	if !ok {
		return "", false
	}
	for _, r := range rows {
		r[3] = installed(r[3])
	}
	return grid([]string{"Repo", "Package", "Version", "Installed"}, rows) + count(len(rows), "package"), true
}

// -Ss, -Qs, -F: "repo/name version (groups) [installed]", then indented lines
// that belong to it: its description, or the files that matched.
var hitRE = regexp.MustCompile(`^(\S+)/(\S+) (\S+)(?: \((.*?)\))?(?: \[(.*)\])?$`)

func paintHits(out, detail string) (string, bool) {
	var rows [][]string
	for _, l := range lines(out) {
		if indented := strings.TrimLeft(l, " "); indented != l {
			if len(rows) == 0 {
				return "", false
			}
			last := rows[len(rows)-1]
			last[5] = strings.TrimPrefix(last[5]+"\n"+indented, "\n")
			continue
		}
		m := hitRE.FindStringSubmatch(l)
		if m == nil {
			return "", false
		}
		rows = append(rows, []string{m[1], m[2], m[3], m[4], installed(m[5]), ""})
	}
	if len(rows) == 0 {
		return "", false
	}
	return grid([]string{"Repo", "Package", "Version", "Groups", "Installed", detail}, rows) + count(len(rows), "package"), true
}

func paintSearch(out string) (string, bool) { return paintHits(out, "Description") }

// -F <path> answers like -Qo; -F <name> and -Fx answer like a search.
func paintFileSearch(out string) (string, bool) {
	if s, ok := paintOwner(out); ok {
		return s, true
	}
	return paintHits(out, "Files")
}

// -Qo: "/usr/bin/ls is owned by coreutils 9.5-1" (pacman translates this one)
var ownerRE = regexp.MustCompile(`^(.+) is owned by (\S+) (\S+)$`)

func paintOwner(out string) (string, bool) {
	rows, ok := match(out, ownerRE)
	if !ok {
		return "", false
	}
	return grid([]string{"File", "Package", "Version"}, rows), true
}

// -Qg, -Sg <group>, -Sgg: "group member"; -Sg alone: "group"
var groupRE = regexp.MustCompile(`^(\S+)(?: (\S+))?$`)

func paintGroups(out string) (string, bool) {
	rows, ok := match(out, groupRE)
	if !ok {
		return "", false
	}
	noun := "group"
	if rows[0][1] != "" {
		noun = "package"
	}
	return grid([]string{"Group", "Package"}, rows) + count(len(rows), noun), true
}

// -Qk: "bash: 269 total files, 0 missing files"; -Qkk counts altered files
var checkRE = regexp.MustCompile(`^(\S+): (\d+) [^,]+, (\d+) (\S+)`)

func paintCheck(out string) (string, bool) {
	rows, ok := match(out, checkRE)
	if !ok {
		return "", false
	}
	what := strings.ToUpper(rows[0][3][:1]) + rows[0][3][1:] // "Missing" or "Altered"
	for i, r := range rows {
		if r[2] == "0" {
			r[2] = okStyle.Render(r[2])
		} else {
			r[2] = badStyle.Render(r[2])
		}
		rows[i] = r[:3]
	}
	return grid([]string{"Package", "Files", what}, rows), true
}

// -Sp, -Rp, -Up: one target per line, in whatever --print-format asked for
func paintTargets(out string) (string, bool) {
	var rows [][]string
	for _, l := range lines(out) {
		rows = append(rows, []string{l})
	}
	if len(rows) == 0 {
		return "", false
	}
	return grid([]string{"Target"}, rows) + count(len(rows), "target"), true
}

// -T: the dependencies that are not satisfied, one per line
func paintMissing(out string) (string, bool) {
	deps := lines(out)
	if len(deps) == 0 {
		return "", false
	}
	l := list.New().
		Enumerator(func(list.Items, int) string { return "✗" }).
		EnumeratorStyle(badStyle.Padding(0, 1))
	for _, d := range deps {
		l.Item(d)
	}
	return l.String(), true
}

// -Dk: pacman only writes to stdout when it found nothing wrong
func paintOK(out string) (string, bool) {
	ls := lines(out)
	for i, l := range ls {
		ls[i] = okStyle.Render(" ✓ ") + l
	}
	return strings.Join(ls, "\n"), len(ls) > 0
}

// -Qc: free text
func paintBox(out string) (string, bool) {
	out = strings.Trim(out, "\n")
	if out == "" {
		return "", false
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(purple).Padding(0, 1)
	if w := termWidth(); lipgloss.Width(out)+4 > w {
		box = box.Width(w)
	}
	return box.Render(out), true
}

// -V: pacman's banner, its ASCII art on the left of the text
func paintVersion(out string) (string, bool) {
	const art = 23 // columns the art takes
	ls := lines(strings.Trim(out, "\n"))
	for i, l := range ls {
		cut := min(art, len(l))
		text := l[cut:]
		if i == 0 {
			text = lipgloss.NewStyle().Bold(true).Render(text)
		}
		ls[i] = lipgloss.NewStyle().Foreground(yellow).Render(l[:cut]) + text
	}
	return paintBox(strings.Join(ls, "\n"))
}

// -Qi, -Si: "Key   : value" lines, the value running on over indented lines,
// one blank-line separated block per package.
var fieldRE = regexp.MustCompile(`^(\S[^:]*?) *: ?(.*)$`)

func paintInfo(out string) (string, bool) {
	var cards []string
	for _, block := range strings.Split(strings.Trim(out, "\n"), "\n\n") {
		var rows [][]string
		for _, l := range lines(block) {
			if m := fieldRE.FindStringSubmatch(l); m != nil {
				rows = append(rows, m[1:])
			} else if len(rows) > 0 {
				last := rows[len(rows)-1]
				last[1] = strings.TrimPrefix(last[1]+"\n"+strings.TrimSpace(l), "\n")
			} else {
				return "", false
			}
		}
		if len(rows) == 0 {
			return "", false
		}
		cards = append(cards, fit(newTable().BorderColumn(false), 2, nil, rows).
			StyleFunc(func(row, col int) lipgloss.Style {
				s := lipgloss.NewStyle().Padding(0, 1)
				switch {
				case col == 0:
					return s.Inherit(titleStyle)
				case rows[row][1] == "None":
					return s.Inherit(dimStyle)
				}
				return s
			}).String())
	}
	return strings.Join(cards, "\n"), true
}

// -Ql, -Fl: "package /path/to/file", directories ending in "/"
func paintFiles(out string) (string, bool) {
	ls := lines(out)
	if len(ls) == 0 {
		return "", false
	}
	var roots []*node
	files := 0
	for _, l := range ls {
		pkg, path, ok := strings.Cut(l, " ")
		if !ok {
			return "", false
		}
		if len(roots) == 0 || roots[len(roots)-1].name != pkg {
			roots = append(roots, &node{name: pkg})
		}
		if !strings.HasSuffix(path, "/") {
			files++
		}
		roots[len(roots)-1].add(strings.SplitAfter(strings.TrimPrefix(path, "/"), "/"))
	}

	var trees []string
	for _, r := range roots {
		trees = append(trees, r.tree(titleStyle).
			Enumerator(tree.RoundedEnumerator).
			EnumeratorStyle(dimStyle).
			String())
	}
	return strings.Join(trees, "\n") + count(files, "file"), true
}

// node is a file or directory in a package's file list. The name of a
// directory ends in "/".
type node struct {
	name string
	kids []*node
}

// add hangs path under n, reusing the directories that are already there.
// pacman lists a directory before what is in it, so only the last kid can match.
func (n *node) add(path []string) {
	if len(path) == 0 || path[0] == "" {
		return
	}
	if len(n.kids) == 0 || n.kids[len(n.kids)-1].name != path[0] {
		n.kids = append(n.kids, &node{name: path[0]})
	}
	n.kids[len(n.kids)-1].add(path[1:])
}

func (n *node) tree(style lipgloss.Style) *tree.Tree {
	t := tree.Root(style.Render(n.name))
	for _, k := range n.kids {
		if strings.HasSuffix(k.name, "/") {
			t.Child(k.tree(borderStyle))
		} else {
			t.Child(k.name)
		}
	}
	return t
}

// opHelp says what each operation is for; pacman's own help does not.
var opHelp = map[string]string{
	"-h": "show this help, or an operation's options",
	"-V": "show pacman's version",
	"-D": "change why a package is recorded as installed, check the database",
	"-F": "search the files of repository packages",
	"-Q": "ask about what is installed",
	"-R": "remove packages",
	"-S": "install, upgrade and search repository packages",
	"-T": "check whether dependencies are satisfied",
	"-U": "install package files",
}

var (
	// "    pacman {-S --sync}     [options] [package(s)]"
	helpOpRE = regexp.MustCompile(`^\s+\S+ \{(-\w) (--[\w-]+)\}\s*(.*)$`)
	// "  -o, --owns <file>    query the package that owns <file>"
	helpOptRE = regexp.MustCompile(`^\s+((?:-\w, )?--[\w-]+(?: <[^>]+>)?)(?:\s+(\S.*))?$`)
)

// -h: a usage line, then either the operations or one operation's options,
// the description of an option sometimes running on over the next line.
func paintHelp(out string) (string, bool) {
	var usage, footer []string
	var ops, opts [][]string
	for _, l := range lines(out) {
		if op := helpOpRE.FindStringSubmatch(l); op != nil {
			ops = append(ops, []string{op[1] + ", " + op[2], op[3], opHelp[op[1]]})
		} else if opt := helpOptRE.FindStringSubmatch(l); opt != nil {
			if strings.HasPrefix(opt[1], "--") {
				opt[1] = "    " + opt[1] // line the long options up: "-y, --refresh"
			}
			opts = append(opts, opt[1:])
		} else if indented := strings.TrimSpace(l); indented != l && len(opts) > 0 {
			last := opts[len(opts)-1]
			last[1] = strings.TrimSpace(last[1] + " " + indented)
		} else if strings.HasSuffix(l, ":") || l == "" {
			// a section title ("options:"): the table header says as much
		} else if len(ops)+len(opts) == 0 {
			usage = append(usage, lipgloss.NewStyle().Bold(true).Render(" "+l))
		} else {
			footer = append(footer, dimStyle.Render(" "+l))
		}
	}

	var body string
	switch {
	case len(ops) > 0:
		body = grid([]string{"Operation", "Arguments", "Does"}, ops)
	case len(opts) > 0:
		body = grid([]string{"Option", "Does"}, opts)
	default:
		return "", false
	}
	return strings.Join(append(append(usage, body), footer...), "\n"), true
}
