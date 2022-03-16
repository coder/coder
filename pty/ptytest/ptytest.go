package ptytest

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/pty"
)

var (
	// Used to ensure terminal output doesn't have anything crazy!
	// See: https://stackoverflow.com/a/29497680
	stripAnsi = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")

	// See: https://man7.org`/linux/man-pages/man3/tcflow.3.html
	//  (004, EOT, Ctrl-D) End-of-file character (EOF).  More
	// precisely: this character causes the pending tty buffer to
	// be sent to the waiting user program without waiting for
	// end-of-line.  If it is the first character of the line,
	// the read(2) in the user program returns 0, which signifies
	// end-of-file.  Recognized when ICANON is set, and then not
	// passed as input.
	VEOF byte = 4

	KeyUp    = []byte{0x1b, '[', 'A'}
	KeyDown  = []byte{0x1b, '[', 'B'}
	KeyRight = []byte{0x1b, '[', 'C'}
	KeyLeft  = []byte{0x1b, '[', 'D'}
)

func New(t *testing.T) *PTY {
	ptty, err := pty.New()
	require.NoError(t, err)

	// In testing we want to avoid raw mode. It takes away our
	// ability to use VEOF to flush the terminal data.
	//
	// If we find another way to flush PTY data to the TTY, we
	// can avoid using VEOF entirely.
	if runtime.GOOS != "windows" {
		initializePTY(t, ptty)
	}

	return create(t, ptty)
}

func Start(t *testing.T, cmd *exec.Cmd) (*PTY, *os.Process) {
	ptty, ps, err := pty.Start(cmd)
	require.NoError(t, err)
	return create(t, ptty), ps
}

func create(t *testing.T, ptty pty.PTY) *PTY {
	reader, writer := io.Pipe()
	scanner := bufio.NewScanner(reader)
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	go func() {
		for scanner.Scan() {
			if scanner.Err() != nil {
				return
			}
			t.Log(stripAnsi.ReplaceAllString(scanner.Text(), ""))
		}
	}()

	t.Cleanup(func() {
		_ = ptty.Close()
	})
	return &PTY{
		t:   t,
		PTY: ptty,

		outputWriter: writer,
		runeReader:   bufio.NewReaderSize(ptty.Output(), utf8.UTFMax),
	}
}

type PTY struct {
	t *testing.T
	pty.PTY

	outputWriter io.Writer
	runeReader   *bufio.Reader
}

func (p *PTY) ExpectMatch(str string) string {
	var buffer bytes.Buffer
	multiWriter := io.MultiWriter(&buffer, p.outputWriter)
	runeWriter := bufio.NewWriterSize(multiWriter, utf8.UTFMax)
	for {
		var r rune
		r, _, err := p.runeReader.ReadRune()
		require.NoError(p.t, err)
		_, err = runeWriter.WriteRune(r)
		require.NoError(p.t, err)
		err = runeWriter.Flush()
		require.NoError(p.t, err)
		if strings.Contains(buffer.String(), str) {
			break
		}
	}
	p.t.Logf("matched %q = %q", str, stripAnsi.ReplaceAllString(buffer.String(), ""))
	return buffer.String()
}

func (p *PTY) Write(data []byte) {
	_, err := p.Input().Write(append(data, VEOF))
	require.NoError(p.t, err)
}

func (p *PTY) WriteLine(str string) {
	_, err := p.Input().Write(append([]byte(str), VEOF))
	require.NoError(p.t, err)
	p.WriteEnter()
}

func (p *PTY) WriteEnter() {
	newline := []byte{'\r'}
	if runtime.GOOS == "windows" {
		newline = append(newline, '\n')
	}
	newline = append(newline, VEOF)
	_, err := p.Input().Write(newline)
	require.NoError(p.t, err)
}
