//go:build mage

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/coder/flog"
	"github.com/magefile/mage/mg"
)

type cmd struct {
	*exec.Cmd
}

// shell offers a cleaner API over magefile's 'sh' helper.
func shell(fmtStr string, args ...interface{}) *cmd {
	return &cmd{exec.Command("sh", "-c", fmt.Sprintf(fmtStr, args...))}
}

func (c *cmd) cd(dir string) *cmd {
	c.Dir = dir
	return c
}

type syncWriter struct {
	sync.Mutex
	w io.Writer
}

func (s *syncWriter) Write(p []byte) (n int, err error) {
	s.Lock()
	defer s.Unlock()
	return s.w.Write(p)
}

var (
	stdout = &syncWriter{w: os.Stdout}
	stderr = &syncWriter{w: os.Stderr}
)

type prefixWriter struct {
	prefix string
	w      io.Writer
}

func (p *prefixWriter) Write(b []byte) (n int, err error) {
	_, err = p.w.Write([]byte(p.prefix + string(b)))
	return len(b), err
}

func ellipse(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (c *cmd) run() error {
	log := flog.New()
	cmdline := strings.Join(c.Args[2:], " ")
	if mg.Verbose() {
		logPrefix := ellipse(cmdline, 16) + ": "
		log.W = stderr
		c.Stdout = &prefixWriter{
			prefix: logPrefix,
			w:      stdout,
		}

		c.Stderr = &prefixWriter{
			prefix: logPrefix,
			w:      stderr,
		}
	}
	log.Info("running: `%s`", cmdline)
	start := time.Now()
	err := c.Cmd.Run()
	log.Info("ran `%s` in %s", cmdline, time.Since(start).Truncate(time.Millisecond))
	return err
}
