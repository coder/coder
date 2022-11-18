package codersdk

import (
	"context"
	"crypto/tls"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
)

func LoadCertificates(tlsCertFiles, tlsKeyFiles []string) ([]tls.Certificate, error) {
	if len(tlsCertFiles) != len(tlsKeyFiles) {
		return nil, xerrors.New("--tls-cert-file and --tls-key-file must be used the same amount of times")
	}
	if len(tlsCertFiles) == 0 {
		return nil, xerrors.New("--tls-cert-file is required when tls is enabled")
	}
	if len(tlsKeyFiles) == 0 {
		return nil, xerrors.New("--tls-key-file is required when tls is enabled")
	}

	certs := make([]tls.Certificate, len(tlsCertFiles))
	for i := range tlsCertFiles {
		certFile, keyFile := tlsCertFiles[i], tlsKeyFiles[i]
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, xerrors.Errorf("load TLS key pair %d (%q, %q): %w", i, certFile, keyFile, err)
		}

		certs[i] = cert
	}

	return certs, nil
}

func HandleOauth2ClientCertificates(ctx context.Context, cfg *TLSConfig) (context.Context, error) {
	if cfg != nil && cfg.ClientCertFile.Value != "" && cfg.ClientKeyFile.Value != "" {
		certificates, err := LoadCertificates([]string{cfg.ClientCertFile.Value}, []string{cfg.ClientKeyFile.Value})
		if err != nil {
			return nil, err
		}

		return context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{ //nolint:gosec
					Certificates: certificates,
				},
			},
		}), nil
	}
	return ctx, nil
}
