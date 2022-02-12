// +build windows

// Original copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

package conpty

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// ConPty represents a windows pseudo console.
type ConPty struct {
	hpCon       windows.Handle
	pipeFdIn    windows.Handle
	pipeFdOut   windows.Handle
	pipe3 windows.Handle
	pipe4 windows.Handle
	consoleSize uintptr
	outputR      *os.File
	outputW     *os.File
	inputR      *os.File
	inputW     *os.File
	closed     bool
}

// New returns a new ConPty pseudo terminal device
func New(columns int16, rows int16) (*ConPty, error) {
	c := &ConPty{
		consoleSize: uintptr(columns) + (uintptr(rows) << 16),
	}

	return c, c.createPseudoConsoleAndPipes()
}

// Close closes the pseudo-terminal and cleans up all attached resources
func (c *ConPty) Close() error {
	if (c.closed) {
		return nil
	}

	err := closePseudoConsole(c.hpCon)
	c.outputR.Close()
	c.outputW.Close()
	c.inputR.Close()
	c.inputW.Close()
	c.closed = true
	return err
}

// OutPipe returns the output pipe of the pseudo terminal
func (c *ConPty) OutPipe() *os.File {
	return c.outputR
}

func (c *ConPty) Reader() io.Reader {
	return c.outputW
}

// InPipe returns input pipe of the pseudo terminal
// Note: It is safer to use the Write method to prevent partially-written VT sequences
// from corrupting the terminal
func (c *ConPty) InPipe() *os.File {
	return c.inputR
}

func (c *ConPty) WriteString(str string) (int, error) {
	return c.inputW.WriteString(str)
}

func (c *ConPty) createPseudoConsoleAndPipes() error {
	// These are the readers/writers for "stdin", but we only need this to
	// successfully call CreatePseudoConsole. After, we can throw it away.
	var hPipeInW, hPipeInR windows.Handle

	// Create the stdin pipe
	if err := windows.CreatePipe(&hPipeInR, &hPipeInW, nil, 0); err != nil {
		return err
	}

	// Create the stdout pipe
	if err := windows.CreatePipe(&c.pipeFdOut, &c.pipeFdIn, nil, 0); err != nil {
		return err
	}

	// Create the pty with our stdin/stdout
	if err := createPseudoConsole(c.consoleSize, hPipeInR, c.pipeFdIn, &c.hpCon); err != nil {
		return fmt.Errorf("failed to create pseudo console: %d, %v", uintptr(c.hpCon), err)
	}

	c.pipe3 = hPipeInR
	c.pipe4 = hPipeInW

	c.outputR = os.NewFile(uintptr(c.pipeFdIn), "|0")
	c.outputW = os.NewFile(uintptr(c.pipeFdOut), "|1")

	c.inputR = os.NewFile(uintptr(c.pipe3), "|2")
	c.inputW = os.NewFile(uintptr(c.pipe4), "|3")
	c.closed = false

	return nil
}

func (c *ConPty) Resize(cols uint16, rows uint16) error {
	return resizePseudoConsole(c.hpCon, uintptr(cols)+(uintptr(rows)<<16))
}
