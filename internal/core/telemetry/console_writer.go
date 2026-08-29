package telemetry

import (
	"bytes"
	"io"
	"os"
	"sync"
	"unicode/utf8"

	"golang.org/x/term"
)

// consoleWriter keeps development logs on one bounded line.
type consoleWriter struct {
	dst   io.Writer
	width int
	mu    sync.Mutex
}

func newConsoleWriter(dst io.Writer) *consoleWriter {
	return &consoleWriter{dst: dst, width: consoleWidth()}
}

func (w *consoleWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	inputLen := len(p)

	if w.width <= 0 {
		return w.dst.Write(p)
	}

	var out bytes.Buffer
	for len(p) > 0 {
		lineEnd := bytes.IndexByte(p, '\n')
		line := p
		newline := false
		if lineEnd >= 0 {
			line = p[:lineEnd]
			newline = true
		}

		out.Write(truncateRunes(line, w.width))
		if newline {
			out.WriteByte('\n')
			p = p[lineEnd+1:]
		} else {
			p = nil
		}
	}

	_, err := w.dst.Write(out.Bytes())
	if err != nil {
		return 0, err
	}
	return inputLen, nil
}

func defaultConsoleWidthForFd() int {
	if columns, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && columns > 0 {
		return columns
	}
	return defaultConsoleWidth
}

func truncateRunes(line []byte, width int) []byte {
	if width <= 0 || utf8.RuneCount(line) <= width {
		return line
	}
	return []byte(string([]rune(string(line))[:width]))
}
