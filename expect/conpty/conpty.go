//go:build windows
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
	hpCon                    windows.Handle
	outPipePseudoConsoleSide windows.Handle
	outPipeOurSide           windows.Handle
	inPipeOurSide            windows.Handle
	inPipePseudoConsoleSide  windows.Handle
	consoleSize              uintptr
	outFilePseudoConsoleSide *os.File
	outFileOurSide           *os.File
	inFilePseudoConsoleSide  *os.File
	inFileOurSide            *os.File
	closed                   bool
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
	// Trying to close these pipes multiple times will result in an
	// access violation
	if c.closed {
		return nil
	}

	err := closePseudoConsole(c.hpCon)
	c.outFilePseudoConsoleSide.Close()
	c.outFileOurSide.Close()
	c.inFilePseudoConsoleSide.Close()
	c.inFileOurSide.Close()
	c.closed = true
	return err
}

// OutPipe returns the output pipe of the pseudo terminal
func (c *ConPty) OutPipe() *os.File {
	return c.outFilePseudoConsoleSide
}

func (c *ConPty) Reader() io.Reader {
	return c.outFileOurSide
}

// InPipe returns input pipe of the pseudo terminal
// Note: It is safer to use the Write method to prevent partially-written VT sequences
// from corrupting the terminal
func (c *ConPty) InPipe() *os.File {
	return c.inFilePseudoConsoleSide
}

func (c *ConPty) WriteString(str string) (int, error) {
	return c.inFileOurSide.WriteString(str)
}

func (c *ConPty) createPseudoConsoleAndPipes() error {
	// Create the stdin pipe
	if err := windows.CreatePipe(&c.inPipePseudoConsoleSide, &c.inPipeOurSide, nil, 0); err != nil {
		return err
	}

	// Create the stdout pipe
	if err := windows.CreatePipe(&c.outPipeOurSide, &c.outPipePseudoConsoleSide, nil, 0); err != nil {
		return err
	}

	// Create the pty with our stdin/stdout
	if err := createPseudoConsole(c.consoleSize, c.inPipePseudoConsoleSide, c.outPipePseudoConsoleSide, &c.hpCon); err != nil {
		return fmt.Errorf("failed to create pseudo console: %d, %v", uintptr(c.hpCon), err)
	}

	c.outFilePseudoConsoleSide = os.NewFile(uintptr(c.outPipePseudoConsoleSide), "|0")
	c.outFileOurSide = os.NewFile(uintptr(c.outPipeOurSide), "|1")

	c.inFilePseudoConsoleSide = os.NewFile(uintptr(c.inPipePseudoConsoleSide), "|2")
	c.inFileOurSide = os.NewFile(uintptr(c.inPipeOurSide), "|3")
	c.closed = false

	return nil
}

func (c *ConPty) Resize(cols uint16, rows uint16) error {
	return resizePseudoConsole(c.hpCon, uintptr(cols)+(uintptr(rows)<<16))
}
