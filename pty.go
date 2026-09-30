package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"github.com/creack/pty"
	"golang.org/x/term"
)

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
	bol := true // whether the next byte written starts a line
	for {
		select {
		case c, ok := <-chunks:
			if !ok {
				w.Write(restyle(pending, bol))
				return
			}
			pending = append(pending, c...)
			if i := bytes.LastIndexAny(pending, "\r\n"); i >= 0 {
				w.Write(restyle(pending[:i+1], bol))
				pending = bytes.Clone(pending[i+1:])
				bol = true
			}
			idle = nil
			if len(pending) > 0 {
				idle = time.After(flushAfter)
			}
		case <-idle:
			w.Write(tint(pending, bol))
			pending, idle, bol = nil, nil, false
		}
	}
}

// pacman draws "[######------]  45%". With ILoveCandy in pacman.conf it draws
// "[------C o  o  ]  45%" instead: dashes behind a Pac-Man, pellets ahead of
// him, and every cell that is not a dash wrapped in its own colour codes.
var barRE = regexp.MustCompile(`\[((?:[#-]|\x1b\[1;33m[Cc]\x1b\[m|\x1b\[0;37m[o ]\x1b\[m){3,})\]( +(\d{1,3})%)`)

var bar = progress.New(progress.WithColors(purple, pink), progress.WithoutPercentage())

// restyle swaps each pacman bar for a bubbles bar of exactly the same width,
// filled to the percentage pacman printed next to it.
func restyle(b []byte, bol bool) []byte {
	return barRE.ReplaceAllFunc(tint(b, bol), func(m []byte) []byte {
		sub := barRE.FindSubmatch(m)
		pct, _ := strconv.Atoi(string(sub[3]))
		// Width, not len: the colour codes of a candy bar take no room.
		bar.SetWidth(lipgloss.Width(string(sub[1])) + 2) // +2: the bar also takes over the brackets
		return append([]byte(lipgloss.Sprint(bar.ViewAs(float64(pct)/100))), sub[2]...)
	})
}

// pacman opens its headings with "::" and its complaints with "error:" or
// "warning:", at times right after hiding or showing the cursor. With Color
// set in pacman.conf it paints them itself, those escape codes keep this from
// matching, and its colours win.
var prefixRE = regexp.MustCompile(`(?m)^((?:\x1b\[\?25[hl])*)(::|error:|warning:)`)

var prefixStyle = map[string]lipgloss.Style{
	"::":       lipgloss.NewStyle().Bold(true).Foreground(pink),
	"error:":   lipgloss.NewStyle().Bold(true).Foreground(red),
	"warning:": lipgloss.NewStyle().Bold(true).Foreground(yellow),
}

// tint colours those prefixes. bol says whether b starts at the start of a line.
func tint(b []byte, bol bool) []byte {
	head := 0
	if !bol {
		head = bytes.IndexByte(b, '\n') + 1
		if head == 0 {
			return b
		}
	}
	return append(b[:head:head], prefixRE.ReplaceAllFunc(b[head:], func(m []byte) []byte {
		sub := prefixRE.FindSubmatch(m)
		prefix := string(sub[2])
		return append(bytes.Clone(sub[1]), lipgloss.Sprint(prefixStyle[prefix].Render(prefix))...)
	})...)
}

// tinter tints what is written to it on its way to w, as it arrives: pacman
// also asks its questions on stderr, and those must not wait for a newline.
type tinter struct {
	w   io.Writer
	mid bool // in the middle of a line
}

func (t *tinter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	_, err := t.w.Write(tint(p, !t.mid))
	t.mid = p[len(p)-1] != '\n'
	return len(p), err
}
