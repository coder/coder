package provisionerd

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"storj.io/drpc/drpcconn"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisioner/echo"
	agpl "github.com/coder/coder/v2/provisionerd"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// Executor is responsible for executing the remote provisioners.
//
// TODO: this interface is where we will run Kubernetes Jobs in a future
// version; right now, only the unit tests implement this interface.
type Executor interface {
	// Execute a provisioner that connects back to the remoteConnector.  errCh
	// allows signaling of errors asynchronously and is closed on completion
	// with no error.
	Execute(
		ctx context.Context,
		provisionerType database.ProvisionerType,
		jobID, token, daemonCert, daemonAddress string) (errCh <-chan error)
}

type waiter struct {
	ctx    context.Context
	job    *proto.AcquiredJob
	respCh chan<- agpl.ConnectResponse
	token  string
}

type remoteConnector struct {
	ctx      context.Context
	executor Executor
	cert     string
	addr     string
	listener net.Listener
	logger   slog.Logger
	tlsCfg   *tls.Config

	mu      sync.Mutex
	waiters map[string]waiter
}

func NewRemoteConnector(ctx context.Context, logger slog.Logger, exec Executor) (agpl.Connector, error) {
	// nolint: gosec
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, xerrors.Errorf("failed to listen: %w", err)
	}
	go func() {
		<-ctx.Done()
		ce := listener.Close()
		logger.Debug(ctx, "listener closed", slog.Error(ce))
	}()
	r := &remoteConnector{
		ctx:      ctx,
		executor: exec,
		listener: listener,
		addr:     listener.Addr().String(),
		logger:   logger,
		waiters:  make(map[string]waiter),
	}
	err = r.genCert()
	if err != nil {
		return nil, xerrors.Errorf("failed to generate certificate: %w", err)
	}
	go r.listenLoop()
	return r, nil
}

func (r *remoteConnector) genCert() error {
	privateKey, cert, err := GenCert()
	if err != nil {
		return err
	}
	r.cert = string(cert)
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return xerrors.Errorf("failed to marshal private key: %w", err)
	}
	pkPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})
	certKey, err := tls.X509KeyPair(cert, pkPEM)
	if err != nil {
		return xerrors.Errorf("failed to create TLS certificate: %w", err)
	}
	r.tlsCfg = &tls.Config{Certificates: []tls.Certificate{certKey}, MinVersion: tls.VersionTLS13}
	return nil
}

// GenCert is a helper function that generates a private key and certificate.  It
// is exported so that we can test a certificate generated in exactly the same
// way, but with a different private key.
func GenCert() (*ecdsa.PrivateKey, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate private key: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Coder Provisioner Daemon",
		},
		DNSNames:  []string{serverName},
		NotBefore: time.Now(),
		// cert is valid for 5 years, which is much longer than we expect this
		// process to stay up.  The idea is that the certificate is self-signed
		// and is valid for as long as the daemon is up and starting new remote
		// provisioners
		NotAfter: time.Now().Add(time.Hour * 24 * 365 * 5),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to create certificate: %w", err)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return privateKey, cert, nil
}

func (r *remoteConnector) listenLoop() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			r.logger.Info(r.ctx, "stopping listenLoop", slog.Error(err))
			return
		}
		go r.handleConn(conn)
	}
}

func (r *remoteConnector) handleConn(conn net.Conn) {
	logger := r.logger.With(slog.F("remote_addr", conn.RemoteAddr()))

	// If we hit an error while setting up, we want to close the connection.
	// This construction makes the default to close until we explicitly set
	// closeConn = false just before handing the connection over the respCh.
	closeConn := true
	defer func() {
		if closeConn {
			ce := conn.Close()
			logger.Debug(r.ctx, "closed connection", slog.Error(ce))
		}
	}()

	tlsConn := tls.Server(conn, r.tlsCfg)
	err := tlsConn.HandshakeContext(r.ctx)
	if err != nil {
		logger.Info(r.ctx, "failed TLS handshake", slog.Error(err))
		return
	}
	w, err := r.authenticate(tlsConn)
	if err != nil {
		logger.Info(r.ctx, "failed provisioner authentication", slog.Error(err))
		return
	}
	logger = logger.With(slog.F("job_id", w.job.JobId))
	logger.Info(r.ctx, "provisioner connected")
	closeConn = false // we're passing the conn over the channel
	w.respCh <- agpl.ConnectResponse{
		Job:    w.job,
		Client: sdkproto.NewDRPCProvisionerClient(drpcconn.New(tlsConn)),
	}
}

var (
	errInvalidJobID = xerrors.New("invalid jobID")
	errInvalidToken = xerrors.New("invalid token")
)

func (r *remoteConnector) pullWaiter(jobID, token string) (waiter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// provisioners authenticate with a jobID and token.  The jobID is required
	// because we need to use public information for the lookup, to avoid timing
	// attacks against the token.
	w, ok := r.waiters[jobID]
	if !ok {
		return waiter{}, errInvalidJobID
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(w.token)) == 1 {
		delete(r.waiters, jobID)
		return w, nil
	}
	return waiter{}, errInvalidToken
}

func (r *remoteConnector) Connect(
	ctx context.Context, job *proto.AcquiredJob, respCh chan<- agpl.ConnectResponse,
) {
	pt := database.ProvisionerType(job.Provisioner)
	if !pt.Valid() {
		go errResponse(job, respCh, xerrors.Errorf("invalid provisioner type: %s", job.Provisioner))
	}
	tb := make([]byte, 16) // 128-bit token
	n, err := rand.Read(tb)
	if err != nil {
		go errResponse(job, respCh, err)
		return
	}
	if n != 16 {
		go errResponse(job, respCh, xerrors.New("short read generating token"))
	}
	token := base64.StdEncoding.EncodeToString(tb)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.waiters[job.JobId] = waiter{
		ctx:    ctx,
		job:    job,
		respCh: respCh,
		token:  token,
	}
	go r.handleContextExpired(ctx, job.JobId)
	errCh := r.executor.Execute(ctx, pt, job.JobId, token, r.cert, r.addr)
	go r.handleExecError(job.JobId, errCh)
}

func (r *remoteConnector) handleContextExpired(ctx context.Context, jobID string) {
	<-ctx.Done()
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.waiters[jobID]
	if !ok {
		// something else already responded.
		return
	}
	delete(r.waiters, jobID)
	// separate goroutine, so we don't hold the lock while trying to write
	// to the channel.
	go func() {
		w.respCh <- agpl.ConnectResponse{
			Job:   w.job,
			Error: ctx.Err(),
		}
	}()
}

func (r *remoteConnector) handleExecError(jobID string, errCh <-chan error) {
	err := <-errCh
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.waiters[jobID]
	if !ok {
		// something else already responded.
		return
	}
	delete(r.waiters, jobID)
	// separate goroutine, so we don't hold the lock while trying to write
	// to the channel.
	go func() {
		w.respCh <- agpl.ConnectResponse{
			Job:   w.job,
			Error: err,
		}
	}()
}

func errResponse(job *proto.AcquiredJob, respCh chan<- agpl.ConnectResponse, err error) {
	respCh <- agpl.ConnectResponse{
		Job:   job,
		Error: err,
	}
}

// EphemeralEcho starts an Echo provisioner that connects to provisioner daemon,
// handles one job, then exits.
func EphemeralEcho(
	ctx context.Context,
	logger slog.Logger,
	cacheDir, jobID, token, daemonCert, daemonAddress string,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workdir := filepath.Join(cacheDir, "echo")
	err := os.MkdirAll(workdir, 0o777)
	if err != nil {
		return xerrors.Errorf("create workdir %s: %w", workdir, err)
	}
	conn, err := DialTLS(ctx, daemonCert, daemonAddress)
	if err != nil {
		return err
	}
	defer conn.Close()
	err = AuthenticateProvisioner(conn, token, jobID)
	if err != nil {
		return err
	}
	// so it's a little confusing, but the provisioner is the client with
	// respect to TLS, but is the server with respect to dRPC
	exitErr := echo.Serve(ctx, &provisionersdk.ServeOptions{
		Conn:          conn,
		Logger:        logger.Named("echo"),
		WorkDirectory: workdir,
	})
	logger.Debug(ctx, "echo.Serve done", slog.Error(exitErr))

	if xerrors.Is(exitErr, context.Canceled) {
		return nil
	}
	return exitErr
}

// DialTLS establishes a TLS connection to the given addr using the given cert
// as the root CA
func DialTLS(ctx context.Context, cert, addr string) (*tls.Conn, error) {
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(cert))
	if !ok {
		return nil, xerrors.New("failed to parse daemon certificate")
	}
	cfg := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13, ServerName: serverName}
	d := net.Dialer{}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, xerrors.Errorf("dial: %w", err)
	}
	tc := tls.Client(nc, cfg)
	// Explicitly handshake so we don't have to mess with setting read
	// and write deadlines.
	err = tc.HandshakeContext(ctx)
	if err != nil {
		_ = nc.Close()
		return nil, xerrors.Errorf("TLS handshake: %w", err)
	}
	return tc, nil
}

// Authentication Protocol:
//
// Ephemeral provisioners connect to the connector using TLS.  This allows the
// provisioner to authenticate the daemon/connector based on the TLS certificate
// delivered to the provisioner out-of-band.
//
// The daemon/connector authenticates the provisioner by jobID and token, which
// are sent over the TLS connection separated by newlines.  The daemon/connector
// responds with a 3-byte response to complete the handshake.
//
// Although the token is unique to the job and unambiguous, we also send the
// jobID.  This allows the daemon/connector to look up the job based on public
// information (jobID), shielding the token from timing attacks.  I'm not sure
// how practical a timing attack against an in-memory golang map is, but it's
// better to avoid it entirely.  After the job is looked up by jobID, we do a
// constant time compare on the token to authenticate.
//
// Also note that we don't really have to worry about cross-version
// compatibility in this protocol, since the provisioners are always started by
// the same daemon/connector as they connect to.

// Responses are all exactly 3 bytes so that don't have to use a scanner
// which might accidentally buffer some of the first dRPC request.
const (
	responseOK           = "OK\n"
	responseInvalidJobID = "IJ\n"
	responseInvalidToken = "IT\n"
)

// serverName is the name on the x509 certificate the daemon/connector generates
// this name doesn't matter as long as both sides agree, since the provisioners
// get the IP address directly.  It is also fine to reuse, since each generates
// a unique private key and self-signs, we will not correctly authenticate to
// a different provisionerd.
const serverName = "provisionerd"

// AuthenticateProvisioner performs the provisioner's side of the authentication
// protocol.
func AuthenticateProvisioner(conn io.ReadWriter, token, jobID string) error {
	sb := strings.Builder{}
	_, _ = sb.WriteString(jobID)
	_, _ = sb.WriteString("\n")
	_, _ = sb.WriteString(token)
	_, _ = sb.WriteString("\n")
	_, err := conn.Write([]byte(sb.String()))
	if err != nil {
		return xerrors.Errorf("failed to write token: %w", err)
	}
	b := make([]byte, 3)
	_, err = conn.Read(b)
	if err != nil {
		return xerrors.Errorf("failed to read token resp: %w", err)
	}
	if string(b) != responseOK {
		// convert to a human-readable format
		var reason string
		switch string(b) {
		case responseInvalidJobID:
			reason = "invalid job ID"
		case responseInvalidToken:
			reason = "invalid token"
		default:
			reason = fmt.Sprintf("unknown response code: %s", b)
		}
		return xerrors.Errorf("authenticate protocol error: %s", reason)
	}
	return nil
}

// authenticate performs the daemon/connector's side of the authentication
// protocol.
func (r *remoteConnector) authenticate(conn io.ReadWriter) (waiter, error) {
	// it's fine to use a scanner here because the provisioner side doesn't hand
	// off the connection to the dRPC handler until after we send our response.
	scn := bufio.NewScanner(conn)
	if ok := scn.Scan(); !ok {
		return waiter{}, xerrors.Errorf("failed to receive jobID: %w", scn.Err())
	}
	jobID := scn.Text()
	if ok := scn.Scan(); !ok {
		return waiter{}, xerrors.Errorf("failed to receive job token: %w", scn.Err())
	}
	token := scn.Text()
	w, err := r.pullWaiter(jobID, token)
	if err == nil {
		_, err = conn.Write([]byte(responseOK))
		if err != nil {
			err = xerrors.Errorf("failed to write authentication response: %w", err)
			// if we fail here, it's our responsibility to send the error response on the respCh
			// because we're not going to return the waiter to the caller.
			go errResponse(w.job, w.respCh, err)
			return waiter{}, err
		}
		return w, nil
	}
	if xerrors.Is(err, errInvalidJobID) {
		_, wErr := conn.Write([]byte(responseInvalidJobID))
		r.logger.Debug(r.ctx, "responded invalid jobID", slog.Error(wErr))
	}
	if xerrors.Is(err, errInvalidToken) {
		_, wErr := conn.Write([]byte(responseInvalidToken))
		r.logger.Debug(r.ctx, "responded invalid token", slog.Error(wErr))
	}
	return waiter{}, err
}
