package aibridgeproxyd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"time"

	"github.com/elazarl/goproxy"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// Server is the AI MITM (Man-in-the-Middle) proxy server.
// It is responsible for:
//   - intercepting HTTPS requests to AI providers
//   - decrypting requests using the configured CA certificate
//   - forwarding requests to aibridge for processing
type Server struct {
	logger     slog.Logger
	proxy      *goproxy.ProxyHttpServer
	httpServer *http.Server
}

// Options configures the AI proxy server.
type Options struct {
	// ListenAddr is the address the proxy server will listen on.
	ListenAddr string
	// CertFile is the path to the CA certificate file used for MITM.
	CertFile string
	// KeyFile is the path to the CA private key file used for MITM.
	KeyFile string
}

func New(ctx context.Context, logger slog.Logger, opts Options) (*Server, error) {
	logger.Info(ctx, "initializing AI proxy server")

	if opts.ListenAddr == "" {
		return nil, xerrors.New("listen address is required")
	}

	if opts.CertFile == "" || opts.KeyFile == "" {
		return nil, xerrors.New("cert file and key file are required")
	}

	// Load CA certificate for MITM
	if err := loadMitmCertificate(opts.CertFile, opts.KeyFile); err != nil {
		return nil, xerrors.Errorf("failed to load MITM certificate: %w", err)
	}

	proxy := goproxy.NewProxyHttpServer()

	// Decrypt all HTTPS requests via MITM. Requests are forwarded to
	// the original destination without modification for now.
	// TODO(ssncferreira): Route requests to aibridged
	//   See https://github.com/coder/internal/issues/1181
	proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)

	srv := &Server{
		logger: logger,
		proxy:  proxy,
	}

	// Start HTTP server in background
	srv.httpServer = &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info(ctx, "starting AI proxy", slog.F("addr", opts.ListenAddr))
		if err := srv.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "aibridgeproxyd server error", slog.Error(err))
		}
	}()

	return srv, nil
}

// loadMitmCertificate loads the CA certificate and key for MITM into goproxy.
func loadMitmCertificate(certFile, keyFile string) error {
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return xerrors.Errorf("load x509 keypair: %w", err)
	}

	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return xerrors.Errorf("parse certificate: %w", err)
	}

	goproxy.GoproxyCa = tls.Certificate{
		Certificate: tlsCert.Certificate,
		PrivateKey:  tlsCert.PrivateKey,
		Leaf:        x509Cert,
	}

	return nil
}

// Close gracefully shuts down the proxy server.
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
