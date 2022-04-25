package ptytest

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/pty"
)

var (
	// Used to ensure terminal output doesn't have anything crazy!
	// See: https://stackoverflow.com/a/29497680
	stripAnsi = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
)

func New(t *testing.T) *PTY {
	ptty, err := pty.New()
	require.NoError(t, err)

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
	complete, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go func() {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-complete.Done():
			return
		case <-timer.C:
		}
		_ = p.Close()
		p.t.Errorf("match exceeded deadline: wanted %q; got %q", str, buffer.String())
	}()
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

func (p *PTY) Write(r rune) {
	_, err := p.Input().Write([]byte{byte(r)})
	require.NoError(p.t, err)
}

func (p *PTY) WriteLine(str string) {
	newline := []byte{'\r'}
	if runtime.GOOS == "windows" {
		newline = append(newline, '\n')
	}
	_, err := p.Input().Write(append([]byte(str), newline...))
	require.NoError(p.t, err)
}
