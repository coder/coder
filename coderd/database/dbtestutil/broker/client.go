package broker

import (
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/drpcsdk"
)

// Client starts and manages a Broker in a subprocess.
type Client struct {
	DRPCBrokerClient
	cmd   *exec.Cmd
	stdin io.Closer
}

func NewClient() (client *Client, err error) {
	client = &Client{
		cmd: exec.Command("go", "run", "github.com/coder/coder/v2/cmd/dbtestbroker"),
	}

	stdin, err := client.cmd.StdinPipe()
	if err != nil {
		return nil, xerrors.Errorf("failed to open stdin pipe: %w", err)
	}
	client.stdin = stdin // needed to tear down the client at end of test
	stdout, err := client.cmd.StdoutPipe()
	if err != nil {
		return nil, xerrors.Errorf("failed to open stdout pipe: %w", err)
	}
	// the broker writes globally scoped errors to stderr; echo these out to the test process stderr.
	client.cmd.Stderr = os.Stderr
	err = client.cmd.Start()
	if err != nil {
		return nil, xerrors.Errorf("failed to start broker: %w", err)
	}
	defer func() {
		// closing stdin signals the broker to exit, which we should do to avoid leaking if we hit an error with yamux
		if err != nil {
			_ = stdin.Close()
		}
	}()
	conn := &readWriteCloser{r: stdout, w: stdin}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(conn, config)
	if err != nil {
		return nil, xerrors.Errorf("failed to create yamux session: %w", err)
	}
	client.DRPCBrokerClient = NewDRPCBrokerClient(drpcsdk.MultiplexedConn(session))
	return client, nil
}

func (c *Client) Close() error {
	if err := c.stdin.Close(); err != nil {
		return xerrors.Errorf("failed to close stdin: %w", err)
	}
	if err := c.cmd.Wait(); err != nil {
		return xerrors.Errorf("failed to wait: %w", err)
	}
	return nil
}

// Singleton manages a shared *Client and cleans it up when no longer needed by reference counting. This allows multiple
// tests to use a single client, reducing system overhead in terms of processes and postgres connections.
type Singleton struct {
	mu       sync.Mutex
	client   *Client
	refCount int
}

// CleanerUpper is just the Cleanup method from testing.TB.
type CleanerUpper interface {
	Cleanup(func())
}

// Get returns a Client and automatically handles reference counting via t.Cleanup
func (s *Singleton) Get(t CleanerUpper) (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		if s.refCount > 0 {
			panic(s.refCount) // programming error with reference counting
		}
		var err error
		s.client, err = NewClient()
		if err != nil {
			return nil, err
		}
	}
	s.refCount++
	t.Cleanup(s.decrement)
	return s.client, nil
}

func (s *Singleton) decrement() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refCount--
	if s.refCount == 0 {
		_ = s.client.Close()
		s.client = nil
	}
	if s.refCount < 0 {
		panic(s.refCount)
	}
}
