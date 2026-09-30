// archmage is a purely cosmetic wrapper around pacman. paru and yay are WIP:
// they can be selected with ARCHMAGE_BACKEND, but run unpainted for now.
//
// It runs the real, unmodified pacman and only repaints what it prints:
//   - progress bars ("[####----]") become bubbles progress bars
//   - listings, searches and package details become lipgloss tables
//   - file lists become lipgloss trees
//
// Everything else (prompts, errors, exit code, keystrokes) passes through as is.
// catalog.go lists every pacman command and what it is painted as.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

func main() {
	b, err := pickBackend()
	if err == nil {
		err = run(b, os.Args[1:])
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "archmage:", err)
		os.Exit(1)
	}
}

func run(b backend, args []string) error {
	in := parseArgs(args)
	cmd := b.command(args, in.needsRoot())

	switch {
	case !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())):
		// Piped or redirected: stay out of the way entirely.
		return passthrough(cmd)
	case b.wip:
		lipgloss.Fprintln(os.Stderr, wipNotice(b))
		return passthrough(cmd)
	case len(args) == 0:
		lipgloss.Println(overview())
		return nil
	case !in.ok:
		return passthrough(cmd)
	case in.has("h"):
		return runPainted(cmd, paintHelp)
	case in.has("V"):
		return runPainted(cmd, paintVersion)
	case in.needsRoot():
		return runWithBars(cmd)
	}
	if c := route(in); c != nil {
		return runPainted(cmd, c.paint)
	}
	return passthrough(cmd)
}

// passthrough runs cmd as if archmage was not there.
func passthrough(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// maxPainted is how many lines of output are still worth painting. Beyond it
// (`pacman -Ql` lists every file on the system) nobody is reading, and
// laying it all out would only make pacman look slow.
const maxPainted = 50000

// runPainted runs cmd and paints what it wrote to stdout once it is done.
// Output that paint does not recognise is printed exactly as pacman wrote it.
func runPainted(cmd *exec.Cmd, paint painter) error {
	cmd.Stdin, cmd.Stderr = os.Stdin, &tinter{w: os.Stderr}
	out, err := cmd.Output()

	if bytes.Count(out, []byte("\n")) > maxPainted {
		os.Stdout.Write(out)
	} else if painted, ok := paint(string(out)); ok {
		lipgloss.Println(painted)
	} else {
		os.Stdout.Write(out)
	}
	return err
}
