package provisionersdk

import (
	"io"
	"os"

	"storj.io/drpc"
)

// Transport creates a dRPC transport using stdin and stdout.
func TransportStdio() drpc.Transport {
	return &transport{
		in:  os.Stdin,
		out: os.Stdout,
	}
}

// TransportPipe creates an in-memory pipe for dRPC transport.
func TransportPipe() (drpc.Transport, drpc.Transport) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	clientTransport := &transport{clientReader, clientWriter, nil}
	serverTransport := &transport{serverReader, serverWriter, nil}

	return clientTransport, serverTransport
}

// transport wraps an input and output to pipe data.
type transport struct {
	in    io.Reader
	out   io.Writer
	close func()
}

func (s *transport) Read(data []byte) (int, error) {
	return s.in.Read(data)
}

func (s *transport) Write(data []byte) (int, error) {
	return s.out.Write(data)
}

func (s *transport) Close() error {
	if s.close != nil {
		s.close()
	}
	return nil
}
