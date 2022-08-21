package azureidentity

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"go.mozilla.org/pkcs7"
	"golang.org/x/xerrors"
)

// allowedSigners matches valid common names listed here:
// https://docs.microsoft.com/en-us/azure/virtual-machines/windows/instance-metadata-service?tabs=linux#tabgroup_14
var allowedSigners = regexp.MustCompile(`^(.*\.)?metadata\.(azure\.(com|us|cn)|microsoftazure\.de)$`)

type metadata struct {
	VMID string `json:"vmId"`
}

// Validate ensures the signature was signed by an Azure certificate.
// It returns the associated VM ID if successful.
func Validate(ctx context.Context, signature string, options x509.VerifyOptions) (string, error) {
	data, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return "", xerrors.Errorf("decode base64: %w", err)
	}
	pkcs7Data, err := pkcs7.Parse(data)
	if err != nil {
		return "", xerrors.Errorf("parse pkcs7: %w", err)
	}
	signer := pkcs7Data.GetOnlySigner()
	if signer == nil {
		return "", xerrors.New("no signers for signature")
	}
	if !allowedSigners.MatchString(signer.Subject.CommonName) {
		return "", xerrors.Errorf("unmatched common name of signer: %q", signer.Subject.CommonName)
	}
	if options.Intermediates == nil {
		options.Intermediates = x509.NewCertPool()
		for _, certURL := range signer.IssuingCertificateURL {
			req, err := http.NewRequestWithContext(ctx, "GET", certURL, nil)
			if err != nil {
				return "", xerrors.Errorf("new request %q: %w", certURL, err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", xerrors.Errorf("perform request %q: %w", certURL, err)
			}
			data, err := io.ReadAll(res.Body)
			if err != nil {
				_ = res.Body.Close()
				return "", xerrors.Errorf("read body %q: %w", certURL, err)
			}
			_ = res.Body.Close()
			cert, err := x509.ParseCertificate(data)
			if err != nil {
				return "", xerrors.Errorf("parse certificate %q: %w", certURL, err)
			}
			options.Intermediates.AddCert(cert)
		}
	}

	_, err = signer.Verify(options)
	if err != nil {
		return "", xerrors.Errorf("verify certificates: %w", err)
	}

	var metadata metadata
	err = json.Unmarshal(pkcs7Data.Content, &metadata)
	if err != nil {
		return "", xerrors.Errorf("unmarshal metadata: %w", err)
	}
	return metadata.VMID, nil
}
