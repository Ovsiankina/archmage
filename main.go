// sp-tmp is a purely cosmetic wrapper around `sudo pacman`.
//
// It runs the real, unmodified pacman and only repaints what it prints:
//   - progress bars ("[####----]") become bubbles progress bars
//   - plain "name version" listings (pacman -Q) become a lipgloss table
//
// Everything else (prompts, errors, exit code, keystrokes) passes through as is.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/creack/pty"
	"golang.org/x/term"
)

var (
	purple = lipgloss.Color("#5A56E0")
	pink   = lipgloss.Color("#EE6FF8")
	gray   = lipgloss.Color("#808080")
)

func main() {
	args := os.Args[1:]

	// SP_CMD lets you wrap something else for testing, e.g. "fakeroot pacman".
	base := strings.Fields(os.Getenv("SP_CMD"))
	if len(base) == 0 {
		base = []string{"sudo", "pacman"}
	}
	cmd := exec.Command(base[0], append(base[1:], args...)...)

	var err error
	switch {
	case !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())):
		// Piped or redirected: stay out of the way entirely.
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err = cmd.Run()
	case isQuery(args):
		err = runQuery(cmd)
	default:
		err = runWithBars(cmd)
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sp-tmp:", err)
		os.Exit(1)
	}
}

// isQuery reports whether pacman was asked for -Q / --query.
func isQuery(args []string) bool {
	for _, a := range args {
		if a == "--query" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "Q")) {
			return true
		}
	}
	return false
}

// runQuery prints "name version" listings as a table. Any other shape of
// output (-Qi, -Ql, -Qs, ...) is printed exactly as pacman wrote it.
func runQuery(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	out, err := cmd.Output()

	var rows [][]string
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) != 2 {
			os.Stdout.Write(out)
			return err
		}
		rows = append(rows, f)
	}
	if len(rows) == 0 {
		return err
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(purple)).
		Headers("Package", "Version").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			switch {
			case row == table.HeaderRow:
				return s.Bold(true).Foreground(pink)
			case col == 1:
				return s.Foreground(gray)
			}
			return s
		})
	lipgloss.Println(t)
	lipgloss.Println(lipgloss.NewStyle().Foreground(gray).Render(fmt.Sprintf(" %d packages", len(rows))))
	return err
}

// runWithBars runs pacman inside a pseudo-terminal, so it still believes it is
// talking to a terminal and draws its progress bars, then repaints those bars
// on their way to the real terminal.
func runWithBars(cmd *exec.Cmd) error {
	size, _ := pty.GetsizeFull(os.Stdout)
	ptmx, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return err
	}
	defer ptmx.Close()

	// Keep the pty the same size as the real terminal, so pacman's own layout
	// (column widths, cursor movement) stays valid.
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			pty.InheritSize(os.Stdout, ptmx)
		}
	}()

	// Raw mode: every keystroke (Y/n, sudo password, ^C) goes to pacman untouched.
	if old, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer term.Restore(int(os.Stdin.Fd()), old)
	}
	go io.Copy(ptmx, os.Stdin)

	repaint(os.Stdout, ptmx)
	return cmd.Wait()
}

// flushAfter is how long to wait for the rest of a line before showing a
// partial one (e.g. a "[Y/n]" prompt, which never ends in a newline).
const flushAfter = 20 * time.Millisecond

// repaint copies r to w, restyling progress bars. Bars are only matched in
// complete lines, so one that is split across two reads is not missed.
func repaint(w io.Writer, r io.Reader) {
	chunks := make(chan []byte)
	go func() {
		defer close(chunks)
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunks <- bytes.Clone(buf[:n])
			}
			if err != nil { // EIO here just means pacman exited
				return
			}
		}
	}()

	var pending []byte
	var idle <-chan time.Time
	for {
		select {
		case c, ok := <-chunks:
			if !ok {
				w.Write(restyle(pending))
				return
			}
			pending = append(pending, c...)
			if i := bytes.LastIndexAny(pending, "\r\n"); i >= 0 {
				w.Write(restyle(pending[:i+1]))
				pending = bytes.Clone(pending[i+1:])
			}
			idle = nil
			if len(pending) > 0 {
				idle = time.After(flushAfter)
			}
		case <-idle:
			w.Write(pending)
			pending, idle = nil, nil
		}
	}
}

// pacman draws "[######------]  45%".
var barRE = regexp.MustCompile(`\[([#-]{3,})\]( +(\d{1,3})%)`)

var bar = progress.New(progress.WithColors(purple, pink), progress.WithoutPercentage())

// restyle swaps each pacman bar for a bubbles bar of exactly the same width,
// filled to the percentage pacman printed next to it.
func restyle(b []byte) []byte {
	return barRE.ReplaceAllFunc(b, func(m []byte) []byte {
		sub := barRE.FindSubmatch(m)
		pct, _ := strconv.Atoi(string(sub[3]))
		bar.SetWidth(len(sub[1]) + 2) // +2: the bar also takes over the brackets
		return append([]byte(lipgloss.Sprint(bar.ViewAs(float64(pct)/100))), sub[2]...)
	})
}
