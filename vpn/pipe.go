package vpn

import (
	"io"
	"os"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
)

// BidirectionalPipe combines a pair of files that can be used for bidirectional
// communication.
type BidirectionalPipe struct {
	read  *os.File
	write *os.File
}

var _ io.ReadWriteCloser = BidirectionalPipe{}

// NewBidirectionalPipe creates a new BidirectionalPipe from the given file
// descriptors.
func NewBidirectionalPipe(readFd, writeFd uintptr) (BidirectionalPipe, error) {
	read := os.NewFile(readFd, "pipe_read")
	_, err := read.Stat()
	if err != nil {
		return BidirectionalPipe{}, xerrors.Errorf("stat pipe_read (fd=%v): %w", readFd, err)
	}
	write := os.NewFile(writeFd, "pipe_write")
	_, err = write.Stat()
	if err != nil {
		return BidirectionalPipe{}, xerrors.Errorf("stat pipe_write (fd=%v): %w", writeFd, err)
	}
	return BidirectionalPipe{
		read:  read,
		write: write,
	}, nil
}

// Read implements io.Reader. Data is read from the read pipe.
func (b BidirectionalPipe) Read(p []byte) (int, error) {
	n, err := b.read.Read(p)
	if err != nil {
		return n, xerrors.Errorf("read from pipe_read (fd=%v): %w", b.read.Fd(), err)
	}
	return n, nil
}

// Write implements io.Writer. Data is written to the write pipe.
func (b BidirectionalPipe) Write(p []byte) (n int, err error) {
	n, err = b.write.Write(p)
	if err != nil {
		return n, xerrors.Errorf("write to pipe_write (fd=%v): %w", b.write.Fd(), err)
	}
	return n, nil
}

// Close implements io.Closer. Both the read and write pipes are closed.
func (b BidirectionalPipe) Close() error {
	var err error
	rErr := b.read.Close()
	if rErr != nil {
		err = multierror.Append(err, xerrors.Errorf("close pipe_read (fd=%v): %w", b.read.Fd(), rErr))
	}
	wErr := b.write.Close()
	if err != nil {
		err = multierror.Append(err, xerrors.Errorf("close pipe_write (fd=%v): %w", b.write.Fd(), wErr))
	}
	return err
}
