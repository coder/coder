//go:build linux
// +build linux

package integration_test

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/test/integration"
	"github.com/coder/coder/v2/testutil"
)

const runTestEnv = "CODER_TAILNET_TESTS"

var (
	isSubprocess = flag.Bool("subprocess", false, "Signifies that this is a test subprocess")
	testID       = flag.String("test-name", "", "Which test is being run")
	role         = flag.String("role", "", "The role of the test subprocess: server, client")

	// Role: server
	serverListenAddr = flag.String("server-listen-addr", "", "The address to listen on for the server")

	// Role: client
	clientName      = flag.String("client-name", "", "The name of the client for logs")
	clientServerURL = flag.String("client-server-url", "", "The url to connect to the server")
	clientMyID      = flag.String("client-id", "", "The id of the client")
	clientPeerID    = flag.String("client-peer-id", "", "The id of the other client")
	clientRunTests  = flag.Bool("client-run-tests", false, "Run the tests in the client subprocess")
)

func TestMain(m *testing.M) {
	if run := os.Getenv(runTestEnv); run == "" {
		_, _ = fmt.Printf("skipping tests as %q is not set...\n", runTestEnv)
		return
	}
	if runtime.GOOS != "linux" {
		_, _ = fmt.Printf("GOOS %q is not linux", runtime.GOOS)
		os.Exit(1)
		return
	}
	if os.Getuid() != 0 {
		_, _ = fmt.Println("UID is not 0")
		os.Exit(1)
		return
	}

	flag.Parse()
	os.Exit(m.Run())
}

var topologies = []integration.TestTopology{
	{
		Name:            "BasicLoopback",
		SetupNetworking: integration.SetupNetworkingLoopback,
		StartServer:     integration.StartServerBasic,
		StartClient:     integration.StartClientBasic,
		RunTests: func(t *testing.T, log slog.Logger, serverURL *url.URL, myID, peerID uuid.UUID, conn *tailnet.Conn) {
			// Test basic connectivity
			peerIP := tailnet.IPFromUUID(peerID)
			_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
			require.NoError(t, err, "ping peer")
		},
	},
}

//nolint:paralleltest
func TestIntegration(t *testing.T) {
	if *isSubprocess {
		handleTestSubprocess(t)
		return
	}

	for _, topo := range topologies {
		//nolint:paralleltest
		t.Run(topo.Name, func(t *testing.T) {
			log := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

			networking := topo.SetupNetworking(t, log)

			// Fork the three child processes.
			serverErrCh, closeServer := startServerSubprocess(t, topo.Name, networking)
			// client1 runs the tests.
			client1ErrCh, _ := startClientSubprocess(t, topo.Name, networking, 1)
			client2ErrCh, closeClient2 := startClientSubprocess(t, topo.Name, networking, 2)

			// Wait for client1 to exit.
			require.NoError(t, <-client1ErrCh)

			// Close client2 and the server.
			closeClient2()
			require.NoError(t, <-client2ErrCh)
			closeServer()
			require.NoError(t, <-serverErrCh)
		})
	}
}

func handleTestSubprocess(t *testing.T) {
	// Find the specific topology.
	var topo integration.TestTopology
	for _, t := range topologies {
		if t.Name == *testID {
			topo = t
			break
		}
	}
	require.NotEmptyf(t, topo.Name, "unknown test topology %q", *testID)

	testName := topo.Name + "/"
	if *role == "server" {
		testName += "server"
	} else {
		testName += *clientName
	}

	//nolint:parralleltest
	t.Run(testName, func(t *testing.T) {
		log := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		switch *role {
		case "server":
			log = log.Named("server")
			topo.StartServer(t, log, *serverListenAddr)
			// no exit

		case "client":
			log = log.Named(*clientName)
			serverURL, err := url.Parse(*clientServerURL)
			require.NoErrorf(t, err, "parse server url %q", *clientServerURL)
			myID, err := uuid.Parse(*clientMyID)
			require.NoErrorf(t, err, "parse client id %q", *clientMyID)
			peerID, err := uuid.Parse(*clientPeerID)
			require.NoErrorf(t, err, "parse peer id %q", *clientPeerID)

			waitForServerAvailable(t, serverURL)

			conn := topo.StartClient(t, log, serverURL, myID, peerID)

			if *clientRunTests {
				topo.RunTests(t, log, serverURL, myID, peerID, conn)
				// and exit
				return
			}
		}

		// Wait for signals.
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
		<-signals
	})
}

func waitForServerAvailable(t *testing.T, serverURL *url.URL) {
	const delay = 100 * time.Millisecond
	const reqTimeout = 2 * time.Second
	const timeout = 30 * time.Second
	client := http.Client{
		Timeout: reqTimeout,
	}

	u, err := url.Parse(serverURL.String() + "/derp/latency-check")
	require.NoError(t, err)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(delay) {
		//nolint:noctx
		resp, err := client.Get(u.String())
		if err != nil {
			t.Logf("waiting for server to be available: %v", err)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Logf("waiting for server to be available: got status %d", resp.StatusCode)
			continue
		}
		return
	}

	t.Fatalf("server did not become available after %v", timeout)
}

func startServerSubprocess(t *testing.T, topologyName string, networking integration.TestNetworking) (<-chan error, func()) {
	return startSubprocess(t, networking.ProcessServer.NetNSFd, []string{
		"--subprocess",
		"--test-name=" + topologyName,
		"--role=server",
		"--server-listen-addr=" + networking.ServerListenAddr,
	})
}

func startClientSubprocess(t *testing.T, topologyName string, networking integration.TestNetworking, clientNumber int) (<-chan error, func()) {
	require.True(t, clientNumber == 1 || clientNumber == 2)

	var (
		clientName = fmt.Sprintf("client%d", clientNumber)
		myID       = integration.Client1ID
		peerID     = integration.Client2ID
		accessURL  = networking.ServerAccessURLClient1
	)
	if clientNumber == 2 {
		myID, peerID = peerID, myID
		accessURL = networking.ServerAccessURLClient2
	}

	flags := []string{
		"--subprocess",
		"--test-name=" + topologyName,
		"--role=client",
		"--client-name=" + clientName,
		"--client-server-url=" + accessURL,
		"--client-id=" + myID.String(),
		"--client-peer-id=" + peerID.String(),
	}
	if clientNumber == 1 {
		flags = append(flags, "--client-run-tests")
	}

	return startSubprocess(t, networking.ProcessClient1.NetNSFd, flags)
}

func startSubprocess(t *testing.T, netNSFd int, flags []string) (<-chan error, func()) {
	name := os.Args[0]
	args := append(os.Args[1:], flags...)

	if netNSFd > 0 {
		// We use nsenter to enter the namespace.
		// We can't use `setns` easily from Golang in the parent process because
		// you can't execute the syscall in the forked child thread before it
		// execs.
		// We can't use `setns` easily from Golang in the child process because
		// by the time you call it, the process has already created multiple
		// threads.
		args = append([]string{"--net=/proc/self/fd/3", name}, args...)
		name = "nsenter"
	}

	cmd := exec.Command(name, args...)
	if netNSFd > 0 {
		cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(netNSFd), "")}
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}
	err := cmd.Start()
	require.NoError(t, err)

	waitErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		waitErr <- err
		close(waitErr)
	}()

	closeFn := func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		case <-waitErr:
			return
		}
		<-waitErr
	}

	t.Cleanup(func() {
		select {
		case err := <-waitErr:
			if err != nil {
				t.Logf("subprocess exited: " + err.Error())
			}
			return
		default:
		}

		closeFn()
	})

	return waitErr, closeFn
}
