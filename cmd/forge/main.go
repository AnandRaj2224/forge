package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/tui"
)

func main() {
	dbPath := flag.String("db", "", "path to forge sqlite db (default dev/forge-dev.db)")
	flag.Parse()
	path := *dbPath
	if path == "" {
		path = os.Getenv("FORGE_DB")
	}
	if path == "" {
		path = filepath.Join("dev", "forge-dev.db")
		if _, err := os.Stat(path); err != nil {
			// Fallback when launched from another cwd: walk up.
			abs, _ := filepath.Abs(".")
			for d := abs; d != "/" && d != "."; d = filepath.Dir(d) {
				cand := filepath.Join(d, "dev", "forge-dev.db")
				if _, err := os.Stat(cand); err == nil {
					path = cand
					break
				}
			}
		}
	}
	database, err := db.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	app := tui.NewApp(database)
	// Route renderer output through a filter that drops the legacy xterm
	// modifyOtherKeys sequences. Bubble Tea v2 enables modifyOtherKeys
	// alongside the Kitty keyboard protocol for xterm compatibility, but
	// kitty reports modifyOtherKeys as superseded by its own protocol and
	// prints a warning. Forge keybindings (j/k/arrows/Enter/Esc/Tab/Ctrl+C
	// plus plain r/s/x/digits) decode fully under the modern Kitty
	// protocol, which passes through untouched, so the legacy request is
	// unnecessary noise here.
	p := tea.NewProgram(app, tea.WithOutput(&legacyKeyboardFilter{w: os.Stdout}))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
}

// legacyKeyboardFilter wraps a terminal output stream and removes the xterm
// modifyOtherKeys enable/reset sequences (CSI > 4 ; … m) from the output
// byte stream while passing everything else through, including the Kitty
// keyboard protocol negotiation. It is streaming-safe: a sequence split
// across Write calls is still recognized via a small hold-back buffer.
//
// The filter also preserves the wrapped stream's terminal identity: Bubble
// Tea detects window size, listens for resizes, and grades color support by
// asserting its output to a file-with-fd interface. A Write-only wrapper
// would fail that assertion, leaving the program at a 0x0 viewport (blank
// screen) with degraded colors, so Read/Close/Fd delegate to the underlying
// stream when it supports them.
type legacyKeyboardFilter struct {
	w       io.Writer
	pending []byte
}

var legacyKeyboardSeqs = [][]byte{
	[]byte("\x1b[>4;2m"),
	[]byte("\x1b[>4;1m"),
	[]byte("\x1b[>4;0m"),
	[]byte("\x1b[>4m"),
}

func matchLegacySeq(b []byte) int {
	for _, s := range legacyKeyboardSeqs {
		if bytes.HasPrefix(b, s) {
			return len(s)
		}
	}
	return 0
}

// isLegacyPrefix reports whether b (the entire unprocessed tail) could be
// the start of a legacy sequence completed by a later Write.
func isLegacyPrefix(b []byte) bool {
	for _, s := range legacyKeyboardSeqs {
		if len(b) < len(s) && bytes.HasPrefix(s, b) {
			return true
		}
	}
	return false
}

func (f *legacyKeyboardFilter) Write(p []byte) (int, error) {
	data := append(f.pending, p...)
	f.pending = f.pending[:0]
	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		if n := matchLegacySeq(data[i:]); n > 0 {
			i += n
			continue
		}
		if isLegacyPrefix(data[i:]) {
			// Hold the trailing partial sequence for the next Write.
			f.pending = append(f.pending, data[i:]...)
			break
		}
		out = append(out, data[i])
		i++
	}
	if len(out) > 0 {
		if _, err := f.w.Write(out); err != nil {
			return 0, err
		}
	}
	// Report full consumption: filtering never back-pressures the renderer.
	return len(p), nil
}

// Read delegates to the underlying stream when it is readable. Bubble Tea
// never reads program output; this exists so the filter keeps the full
// file interface of the stream it wraps.
func (f *legacyKeyboardFilter) Read(p []byte) (int, error) {
	if r, ok := f.w.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, io.EOF
}

// Close delegates to the underlying stream when it is closable.
func (f *legacyKeyboardFilter) Close() error {
	if c, ok := f.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Fd exposes the underlying file descriptor so Bubble Tea still recognizes
// terminal output for size detection and resize handling. Without this,
// wrapping stdout breaks TTY detection and the TUI runs at 0x0 (blank).
// When the wrapped stream has no descriptor, an invalid fd is reported so
// it is never mistaken for a terminal.
func (f *legacyKeyboardFilter) Fd() uintptr {
	if ff, ok := f.w.(interface{ Fd() uintptr }); ok {
		return ff.Fd()
	}
	return ^uintptr(0)
}
