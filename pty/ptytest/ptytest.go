package ptytest

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/coder/coder/pty"
	"github.com/stretchr/testify/require"
)

var (
	// Used to ensure terminal output doesn't have anything crazy!
	// See: https://stackoverflow.com/a/29497680
	stripAnsi = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
)

func New(t *testing.T) *PTY {
	pty, err := pty.New()
	require.NoError(t, err)
	return create(t, pty)
}

func Start(t *testing.T, cmd *exec.Cmd) *PTY {
	pty, err := pty.Start(cmd)
	require.NoError(t, err)
	return create(t, pty)
}

func create(t *testing.T, pty pty.PTY) *PTY {
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
		_ = pty.Close()
	})
	return &PTY{
		t:   t,
		PTY: pty,

		outputWriter: writer,
		runeReader:   bufio.NewReaderSize(pty.Output(), utf8.UTFMax),
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
	return buffer.String()
}

func (p *PTY) WriteLine(str string) {
	_, err := p.PTY.Input().Write([]byte(str + "\n"))
	require.NoError(p.t, err)
}
