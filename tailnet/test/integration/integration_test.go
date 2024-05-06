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
	"strings"
	"sync"
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
		Name:            "BasicLoopbackDERP",
		SetupNetworking: integration.SetupNetworkingLoopback,
		StartServer:     integration.StartServerBasic,
		StartClient:     integration.StartClientBasic,
		RunTests:        integration.TestSuite,
	},
	{
		Name:            "EasyNATDERP",
		SetupNetworking: integration.SetupNetworkingEasyNAT,
		StartServer:     integration.StartServerBasic,
		StartClient:     integration.StartClientBasic,
		RunTests:        integration.TestSuite,
	},
}

//nolint:paralleltest,tparallel
func TestIntegration(t *testing.T) {
	if *isSubprocess {
		handleTestSubprocess(t)
		return
	}

	for _, topo := range topologies {
		topo := topo
		t.Run(topo.Name, func(t *testing.T) {
			// These can run in parallel because every test should be in an
			// isolated NetNS.
			t.Parallel()

			log := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			networking := topo.SetupNetworking(t, log)

			// Fork the three child processes.
			serverErrCh, closeServer := startServerSubprocess(t, topo.Name, networking)
			// client1 runs the tests.
			client1ErrCh, _ := startClientSubprocess(t, topo.Name, networking, 1)
			client2ErrCh, closeClient2 := startClientSubprocess(t, topo.Name, networking, 2)

			// Wait for client1 to exit.
			require.NoError(t, <-client1ErrCh, "client 1 exited")

			// Close client2 and the server.
			closeClient2()
			require.NoError(t, <-client2ErrCh, "client 2 exited")
			closeServer()
			require.NoError(t, <-serverErrCh, "server exited")
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
				// Wait for connectivity.
				peerIP := tailnet.IPFromUUID(peerID)
				if !conn.AwaitReachable(testutil.Context(t, testutil.WaitLong), peerIP) {
					t.Fatalf("peer %v did not become reachable", peerIP)
				}

				topo.RunTests(t, log, serverURL, myID, peerID, conn)
				// then exit
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
	return startSubprocess(t, "server", networking.ProcessServer.NetNS, []string{
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
		netNS      = networking.ProcessClient1.NetNS
	)
	if clientNumber == 2 {
		myID, peerID = peerID, myID
		accessURL = networking.ServerAccessURLClient2
		netNS = networking.ProcessClient2.NetNS
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

	return startSubprocess(t, clientName, netNS, flags)
}

func startSubprocess(t *testing.T, processName string, netNS *os.File, flags []string) (<-chan error, func()) {
	name := os.Args[0]
	// Always use verbose mode since it gets piped to the parent test anyways.
	args := append(os.Args[1:], append([]string{"-test.v=true"}, flags...)...)

	if netNS != nil {
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
	if netNS != nil {
		cmd.ExtraFiles = []*os.File{netNS}
	}

	out := &testWriter{
		name: processName,
		t:    t,
	}
	t.Cleanup(out.Flush)
	cmd.Stdout = out
	cmd.Stderr = out
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

type testWriter struct {
	mut  sync.Mutex
	name string
	t    *testing.T

	capturedLines []string
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.mut.Lock()
	defer w.mut.Unlock()
	str := string(p)
	split := strings.Split(str, "\n")
	for _, s := range split {
		if s == "" {
			continue
		}

		// If a line begins with "\s*--- (PASS|FAIL)" or is just PASS or FAIL,
		// then it's a test result line. We want to capture it and log it later.
		trimmed := strings.TrimSpace(s)
		if strings.HasPrefix(trimmed, "--- PASS") || strings.HasPrefix(trimmed, "--- FAIL") || trimmed == "PASS" || trimmed == "FAIL" {
			w.capturedLines = append(w.capturedLines, s)
			continue
		}

		w.t.Logf("%s output: \t%s", w.name, s)
	}
	return len(p), nil
}

func (w *testWriter) Flush() {
	w.mut.Lock()
	defer w.mut.Unlock()
	for _, s := range w.capturedLines {
		w.t.Logf("%s output: \t%s", w.name, s)
	}
	w.capturedLines = nil
}
