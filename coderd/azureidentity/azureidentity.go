package azureidentity

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/smallstep/pkcs7"
	"golang.org/x/xerrors"
)

// allowedSigners matches valid common names listed here:
// https://docs.microsoft.com/en-us/azure/virtual-machines/windows/instance-metadata-service?tabs=linux#tabgroup_14
var allowedSigners = regexp.MustCompile(`^(.*\.)?metadata\.(azure\.(com|us|cn)|microsoftazure\.de)$`)

// The pkcs7 library has a global variable that is incremented
// each time a parse occurs.
var pkcs7Mutex sync.Mutex

// allowedCertHosts contains the hosts Azure intermediate
// certificates are served from. Only these hosts are permitted
// when fetching issuing certificates referenced in the signer
// certificate. This prevents SSRF via crafted
// IssuingCertificateURL values.
//
// Source: https://learn.microsoft.com/en-us/azure/security/fundamentals/azure-ca-details
var allowedCertHosts = map[string]bool{
	"www.microsoft.com":    true,
	"cacerts.digicert.com": true,
}

// maxCertResponseBytes is the maximum size of a certificate
// response body we will read. Azure intermediate certificates
// are typically under 4 KiB; 1 MiB is a generous upper bound
// that prevents memory exhaustion from malicious responses.
const maxCertResponseBytes = 1 << 20 // 1 MiB

// extraBlockedNetworks lists special-use CIDR ranges that the
// stdlib classification methods (IsLoopback, IsPrivate, etc.) do
// not cover. Blocking these prevents SSRF against carrier-grade
// NAT, network-benchmarking, documentation, discard-only, and
// the all-zeros "this network" range.
//
// IPv6 ranges already handled by stdlib:
//   - ::1/128        (IsLoopback)
//   - fc00::/7       (IsPrivate, ULA)
//   - fe80::/10      (IsLinkLocalUnicast)
//   - ff00::/8       (IsMulticast)
//   - ::/128         (IsUnspecified)
var extraBlockedNetworks []*net.IPNet

func init() {
	for _, cidr := range []string{
		// IPv4 special-use ranges.
		"0.0.0.0/8",     // RFC 1122 "this network".
		"100.64.0.0/10", // RFC 6598 carrier-grade NAT.
		"198.18.0.0/15", // RFC 2544 benchmarking.

		// IPv6 special-use ranges not covered by stdlib.
		"64:ff9b:1::/48", // RFC 8215 IPv4/IPv6 translation.
		"100::/64",       // RFC 6666 discard-only.
		"2001:2::/48",    // RFC 5180 benchmarking.
		"2001:db8::/32",  // RFC 3849 documentation.
	} {
		_, network, _ := net.ParseCIDR(cidr)
		extraBlockedNetworks = append(extraBlockedNetworks, network)
	}
}

// isPrivateIP reports whether the IP is on a network that must
// not be reachable when fetching certificates. IPv4-mapped IPv6
// addresses are canonicalized to IPv4 first so a literal like
// ::ffff:169.254.169.254 cannot bypass the IPv4 ranges.
func isPrivateIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		ip.IsInterfaceLocalMulticast() {
		return true
	}
	for _, network := range extraBlockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// certFetchClient is an HTTP client that refuses to connect
// to private or link-local IP addresses. This provides
// defense-in-depth against SSRF even if the host allowlist is
// somehow bypassed (e.g. via DNS rebinding).
var certFetchClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, xerrors.Errorf("split host/port: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, xerrors.Errorf("resolve host: %w", err)
			}
			if len(ips) == 0 {
				return nil, xerrors.Errorf("no addresses for %q", host)
			}
			// Reject up front so a single tainted answer
			// short-circuits the dial rather than racing it.
			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, xerrors.Errorf(
						"certificate fetch blocked: %q resolved to private IP %s",
						host, ip.IP,
					)
				}
			}
			// Dial the validated IP directly. If we dialed by
			// hostname here, Go's stdlib would re-resolve and a
			// hostile resolver could swap in a private IP after
			// validation (DNS rebinding). TLS verification still
			// uses the URL host via the Transport's TLS config.
			var d net.Dialer
			var firstErr error
			for _, ip := range ips {
				conn, derr := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if derr == nil {
					return conn, nil
				}
				if firstErr == nil {
					firstErr = derr
				}
			}
			return nil, firstErr
		},
	},
}

// IsAllowedCertificateURL reports whether rawURL points to a
// host on the allowlist, uses http or https, and targets a
// standard PKI distribution port. Microsoft and DigiCert serve
// these artifacts on 80/443 only; any other port is rejected to
// keep the SSRF surface as narrow as the hostname itself.
func IsAllowedCertificateURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if !allowedCertHosts[u.Hostname()] {
		return false
	}
	switch u.Port() {
	case "", "80", "443":
		return true
	default:
		return false
	}
}

type metadata struct {
	VMID string `json:"vmId"`
}

type Options struct {
	// Roots is the trusted root certificate pool. If nil,
	// the embedded root certificate pool is used.
	Roots *x509.CertPool
	// Intermediates are additional intermediate certificates to
	// inject into the PKCS7 object for chain verification. Azure
	// PKCS7 envelopes typically only contain the signing cert, so
	// intermediates must be supplied externally. When nil, the
	// hardcoded Azure intermediate certificates are used.
	Intermediates []*x509.Certificate
	// CurrentTime, if non-zero, overrides the verification
	// timestamp for certificate chain validation.
	CurrentTime time.Time
	// Offline disables fetching of issuing certificates when
	// chain verification fails.
	Offline bool
}

// Validate ensures the signature was signed by an Azure certificate.
// It returns the associated VM ID if successful.
//
// Verification has two parts, both handled by VerifyWithChainAtTime:
//  1. PKCS7 signature check: proves the content was signed by the
//     private key corresponding to the certificate in the envelope.
//  2. Certificate chain check: proves the signing certificate
//     chains to a trusted root through known intermediates.
func Validate(ctx context.Context, signature string, options Options) (string, error) {
	data, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return "", xerrors.Errorf("decode base64: %w", err)
	}
	pkcs7Mutex.Lock()
	pkcs7Data, err := pkcs7.Parse(data)
	pkcs7Mutex.Unlock()
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
	// Azure PKCS7 envelopes typically contain only the signing
	// certificate. Inject intermediate certificates so the
	// library can build a chain from signer to trusted root.
	intermediates := options.Intermediates
	if intermediates == nil {
		intermediates, err = ParseCertificates()
		if err != nil {
			return "", xerrors.Errorf("parse hardcoded certificates: %w", err)
		}
	}
	pkcs7Data.Certificates = append(pkcs7Data.Certificates, intermediates...)
	// Resolve root trust store. VerifyWithChainAtTime skips
	// chain verification when the trust store is nil, so we
	// must always provide one.
	roots := options.Roots
	if roots == nil {
		roots, err = rootCertPool()
		if err != nil {
			return "", xerrors.Errorf("load roots: %w", err)
		}
	}

	currentTime := options.CurrentTime
	if currentTime.IsZero() {
		currentTime = time.Now()
	}

	// VerifyWithChainAtTime validates both the PKCS7 signature
	// (proving the content was signed by the certificate's
	// private key) and the certificate chain (proving the signer
	// chains to a trusted root).
	err = pkcs7Data.VerifyWithChainAtTime(roots, currentTime)
	if err != nil {
		if options.Offline {
			return "", xerrors.Errorf("verify pkcs7: %w", err)
		}

		// The chain verification may fail when the signing
		// certificate was issued by an intermediate not yet in
		// our hardcoded list. Fetch the issuing certificates
		// and retry.
		ctx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		for _, certURL := range signer.IssuingCertificateURL {
			if !IsAllowedCertificateURL(certURL) {
				return "", xerrors.New("issuing certificate URL not on allowlist")
			}
			req, err := http.NewRequestWithContext(ctx, "GET", certURL, nil)
			if err != nil {
				return "", xerrors.New("construct certificate request")
			}
			res, err := certFetchClient.Do(req)
			if err != nil {
				return "", xerrors.New("certificate fetch unsuccessful")
			}
			limited := io.LimitReader(res.Body, maxCertResponseBytes+1)
			certData, err := io.ReadAll(limited)
			_ = res.Body.Close()
			if err != nil {
				return "", xerrors.New("read certificate response body")
			}
			if int64(len(certData)) > maxCertResponseBytes {
				return "", xerrors.New(
					"certificate response exceeds maximum size",
				)
			}
			cert, err := x509.ParseCertificate(certData)
			if err != nil {
				// Do not wrap the parse error; it may contain
				// fragments of the HTTP response body, which
				// could leak internal data to the caller.
				return "", xerrors.New(
					"fetched data is not a valid certificate",
				)
			}
			pkcs7Data.Certificates = append(pkcs7Data.Certificates, cert)
		}
		err = pkcs7Data.VerifyWithChainAtTime(roots, currentTime)
		if err != nil {
			return "", xerrors.New("signature verification failed after fetching issuing certificates")
		}
	}

	var metadata metadata
	err = json.Unmarshal(pkcs7Data.Content, &metadata)
	if err != nil {
		return "", xerrors.Errorf("unmarshal metadata: %w", err)
	}
	return metadata.VMID, nil
}

// ParseCertificates parses the hardcoded Azure intermediate
// certificates and returns them as x509.Certificate values.
func ParseCertificates() ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for _, certPEM := range Certificates {
		block, rest := pem.Decode([]byte(certPEM))
		if len(rest) != 0 {
			return nil, xerrors.Errorf("invalid certificate. %d bytes remain", len(rest))
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, xerrors.Errorf("parse certificate: %w", err)
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

// Certificates are manually downloaded from Azure, then processed with OpenSSL
// and added here. See: https://learn.microsoft.com/en-us/azure/security/fundamentals/azure-ca-details
//
// 1. Download the certificate
// 2. Convert to PEM format: `openssl x509 -in cert.pem -text`
// 3. Paste the contents into the array below
var Certificates = []string{
	// Microsoft Azure ECC TLS Issuing CA 03
	`-----BEGIN CERTIFICATE-----
MIIDXTCCAuOgAwIBAgIQAVKe6DaPC11yukM+LY6mLTAKBggqhkjOPQQDAzBhMQsw
CQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3d3cu
ZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBHMzAe
Fw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVTMR4w
HAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jvc29m
dCBBenVyZSBFQ0MgVExTIElzc3VpbmcgQ0EgMDMwdjAQBgcqhkjOPQIBBgUrgQQA
IgNiAASWQZj7wTifz52AAaZuhd5vnHlA6omsawVbdr1pX7FP6cPvZ8ABw/JX24u1
0nk6VWg7aC2Ey3cwi4mcSJWG4MOcb/ymon7q0iHlnLFjB3wKOZDbNafqe6E3fyAy
f2QcREijggFiMIIBXjASBgNVHRMBAf8ECDAGAQH/AgEAMB0GA1UdDgQWBBRy4Jah
UeowDFi19RmrmnzNl1UQLjAfBgNVHSMEGDAWgBSz20ik+aHF2K42QcwRY2liKbxL
xjAOBgNVHQ8BAf8EBAMCAYYwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMC
MHYGCCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNl
cnQuY29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20v
RGlnaUNlcnRHbG9iYWxSb290RzMuY3J0MEIGA1UdHwQ7MDkwN6A1oDOGMWh0dHA6
Ly9jcmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMy5jcmwwHQYD
VR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMAoGCCqGSM49BAMDA2gAMGUCMQC2
v2Br7lTZJSweZMFP38SguGYcoFeKFb9TA3KAxeuGbAk5BnKY0DohnJiFncj8GFkC
MGHYkSqHik6yPbKi1OaJkVl9grldr+Y+z+jgUwWIaJ6ljXXj8cPXpyFgz3UEDnip
Eg==
-----END CERTIFICATE-----`,
	// Microsoft Azure ECC TLS Issuing CA 04
	`-----BEGIN CERTIFICATE-----
MIIDXDCCAuOgAwIBAgIQAjk9SNcCQlp8tBwACw7XyjAKBggqhkjOPQQDAzBhMQsw
CQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3d3cu
ZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBHMzAe
Fw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVTMR4w
HAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jvc29m
dCBBenVyZSBFQ0MgVExTIElzc3VpbmcgQ0EgMDQwdjAQBgcqhkjOPQIBBgUrgQQA
IgNiAARPTjQp1si15xHY4NHuaYml1SVS2WNRqzy5Pe5cjp4gxINQbtjyKSJL2Kkn
PFcl+Q657jLtO7gW5Oo2U4SrPf0KryBIzmpxdIWFv7OIRW/DsNpBY27x1kkcLfMa
VlD41KejggFiMIIBXjASBgNVHRMBAf8ECDAGAQH/AgEAMB0GA1UdDgQWBBQ18ecR
MmjmssjaceZw8+g8uA4HGzAfBgNVHSMEGDAWgBSz20ik+aHF2K42QcwRY2liKbxL
xjAOBgNVHQ8BAf8EBAMCAYYwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMC
MHYGCCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNl
cnQuY29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20v
RGlnaUNlcnRHbG9iYWxSb290RzMuY3J0MEIGA1UdHwQ7MDkwN6A1oDOGMWh0dHA6
Ly9jcmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMy5jcmwwHQYD
VR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMAoGCCqGSM49BAMDA2cAMGQCMFrb
S3clttzDrBUuwHuTyZPgSxVR4ShEvcjfJFFzv8n4TRORvsHt730s9ki6IB37+AIw
IT4LyBa6AKnYLFZZG7vGPF+exAK0qvyQ1Vw60KLBatMs+QpGXXWErmWRerrVGsYi
-----END CERTIFICATE-----`,
	// Microsoft Azure ECC TLS Issuing CA 07
	`-----BEGIN CERTIFICATE-----
MIIDXTCCAuOgAwIBAgIQDx8VdYLNzTNzS9xfzZQaMzAKBggqhkjOPQQDAzBhMQsw
CQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3d3cu
ZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBHMzAe
Fw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVTMR4w
HAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jvc29m
dCBBenVyZSBFQ0MgVExTIElzc3VpbmcgQ0EgMDcwdjAQBgcqhkjOPQIBBgUrgQQA
IgNiAATokm9hNnECQj2lbZM9is6plTI2rgjbWOkOLqclsWYe7hly1d9YsaivU9rw
QAhByBfxuBIAOuvgcUoYhihMsGuzwe8REVxJzkNIvQMi6cyUZL4bSMkZa/9R8qt9
eAlQ2XKjggFiMIIBXjASBgNVHRMBAf8ECDAGAQH/AgEAMB0GA1UdDgQWBBTDXqxA
dsAGTeMrlJkwYHM0mCnGUTAfBgNVHSMEGDAWgBSz20ik+aHF2K42QcwRY2liKbxL
xjAOBgNVHQ8BAf8EBAMCAYYwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMC
MHYGCCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNl
cnQuY29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20v
RGlnaUNlcnRHbG9iYWxSb290RzMuY3J0MEIGA1UdHwQ7MDkwN6A1oDOGMWh0dHA6
Ly9jcmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMy5jcmwwHQYD
VR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMAoGCCqGSM49BAMDA2gAMGUCMQD4
NlZZatULuw0uN/yBMq9WikJwL8IHljJyU1EyPmv3XOKab+TbGSFWK/x6QeCH4lkC
MGnBJi1rXgd9ieBW4PSmq1v0Jd5YrBptoNMGk5J+dDOj7L3ItN16Lyjk9coSKgZS
zw==
-----END CERTIFICATE-----`,
	// Microsoft Azure ECC TLS Issuing CA 08
	`-----BEGIN CERTIFICATE-----
MIIDXDCCAuOgAwIBAgIQDvLl2DaBUgJV6Sxgj7wv9DAKBggqhkjOPQQDAzBhMQsw
CQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3d3cu
ZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBHMzAe
Fw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVTMR4w
HAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jvc29m
dCBBenVyZSBFQ0MgVExTIElzc3VpbmcgQ0EgMDgwdjAQBgcqhkjOPQIBBgUrgQQA
IgNiAATlQzoKIJQIe8bd4sX2x9XBtFvoh5m7Neph3MYORvv/rg2Ew7Cfb00eZ+zS
njUosyOUCspenehe0PyKtmq6pPshLu5Ww/hLEoQT3drwxZ5PaYHmGEGoy2aPBeXa
23k5ruijggFiMIIBXjASBgNVHRMBAf8ECDAGAQH/AgEAMB0GA1UdDgQWBBStVB0D
VHHGL17WWxhYzm4kxdaiCjAfBgNVHSMEGDAWgBSz20ik+aHF2K42QcwRY2liKbxL
xjAOBgNVHQ8BAf8EBAMCAYYwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMC
MHYGCCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNl
cnQuY29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20v
RGlnaUNlcnRHbG9iYWxSb290RzMuY3J0MEIGA1UdHwQ7MDkwN6A1oDOGMWh0dHA6
Ly9jcmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMy5jcmwwHQYD
VR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMAoGCCqGSM49BAMDA2cAMGQCMD+q
5Uq1fSGZSKRhrnWKKXlp4DvfZCEU/MF3rbdwAaXI/KVM65YRO9HvRbfDpV3x1wIw
CHvqqpg/8YJPDn8NJIS/Rg+lYraOseXeuNYzkjeY6RLxIDB+nLVDs9QJ3/co89Cd
-----END CERTIFICATE-----`,
	// Microsoft Azure RSA TLS Issuing CA 03
	`-----BEGIN CERTIFICATE-----
MIIFrDCCBJSgAwIBAgIQBRllJkSaXj0aOHSPXc/rzDANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jv
c29mdCBBenVyZSBSU0EgVExTIElzc3VpbmcgQ0EgMDMwggIiMA0GCSqGSIb3DQEB
AQUAA4ICDwAwggIKAoICAQCUaitvevlZirydcTjMIt2fr5ei7LvQx7bdIVobgEZ1
Qlqf3BH6etKdmZChydkN0XXAb8Ysew8aCixKtrVeDCe5xRRCnKaFcEvqg2cSfbpX
FevXDvfbTK2ed7YASOJ/pv31stqHd9m0xWZLCmsXZ8x6yIxgEGVHjIAOCyTAgcQy
8ItIjmxn3Vu2FFVBemtP38Nzur/8id85uY7QPspI8Er8qVBBBHp6PhxTIKxAZpZb
XtBf2VxIKbvUGEvCxWCrKNfv+j0oEqDpXOqGFpVBK28Q48u/0F+YBUY8FKP4rfgF
I4lG9mnzMmCL76k+HjyBtU5zikDGqgm4mlPXgSRqEh0CvQS7zyrBRWiJCfK0g67f
69CVGa7fji8pz99J59s8bYW7jgyro93LCGb4N3QfJLurB//ehDp33XdIhizJtopj
UoFUGLnomVnMRTUNtMSAy7J4r1yjJDLufgnrPZ0yjYo6nyMiFswCaMmFfclUKtGz
zbPDpIBuf0hmvJAt0LyWlYUst5geusPxbkM5XOhLn7px+/y+R0wMT3zNZYQxlsLD
bXGYsRdE9jxcIts+IQwWZGnmHhhC1kvKC/nAYcqBZctMQB5q/qsPH652dc73zOx6
Bp2gTZqokGCv5PGxiXcrwouOUIlYgizBDYGBDU02S4BRDM3oW9motVUonBnF8JHV
RwIDAQABo4IBYjCCAV4wEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQU/glx
QFUFEETYpIF1uJ4a6UoGiMgwHwYDVR0jBBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYX
jzkwDgYDVR0PAQH/BAQDAgGGMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD
AjB2BggrBgEFBQcBAQRqMGgwJAYIKwYBBQUHMAGGGGh0dHA6Ly9vY3NwLmRpZ2lj
ZXJ0LmNvbTBABggrBgEFBQcwAoY0aHR0cDovL2NhY2VydHMuZGlnaWNlcnQuY29t
L0RpZ2lDZXJ0R2xvYmFsUm9vdEcyLmNydDBCBgNVHR8EOzA5MDegNaAzhjFodHRw
Oi8vY3JsMy5kaWdpY2VydC5jb20vRGlnaUNlcnRHbG9iYWxSb290RzIuY3JsMB0G
A1UdIAQWMBQwCAYGZ4EMAQIBMAgGBmeBDAECAjANBgkqhkiG9w0BAQwFAAOCAQEA
AQkxu6RRPlD3yrYhxg9jIlVZKjAnC9H+D0SSq4j1I8dNImZ4QjexTEv+224CSvy4
zfp9gmeRfC8rnrr4FN4UFppYIgqR4H7jIUVMG9ECUcQj2Ef11RXqKOg5LK3fkoFz
/Nb9CYvg4Ws9zv8xmE1Mr2N6WDgLuTBIwul2/7oakjj8MA5EeijIjHgB1/0r5mPm
eFYVx8xCuX/j7+q4tH4PiHzzBcfqb3k0iR4DlhiZfDmy4FuNWXGM8ZoMM43EnRN/
meqAcMkABZhY4gqeWZbOgxber297PnGOCcIplOwpPfLu1A1K9frVwDzAG096a8L0
+ItQCmz7TjRH4ptX5Zh9pw==
-----END CERTIFICATE-----`,
	// Microsoft Azure RSA TLS Issuing CA 04
	`-----BEGIN CERTIFICATE-----
MIIFrDCCBJSgAwIBAgIQCfluwpVVXyR0nq8eXc7UnTANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jv
c29mdCBBenVyZSBSU0EgVExTIElzc3VpbmcgQ0EgMDQwggIiMA0GCSqGSIb3DQEB
AQUAA4ICDwAwggIKAoICAQDBeUy13eRZ/QC5bN7/IOGxodny7Xm2BFc88d3cca3y
HyyVx1Y60+afY6DAo/2Ls1uzAfbDfMzAVWJazPH4tckaItDv//htEbbNJnAGvZPB
4VqNviwDEmlAWT/MTAmzXfTgWXuUNgRlzZbjoFaPm+t6iJ6HdvDpWQAJbsBUZCga
t257tM28JnAHUTWdiDBn+2z6EGh2DA6BCx04zHDKVSegLY8+5P80Lqze0d6i3T2J
J7rfxCmxUXfCGOv9iQIUZfhv4vCb8hsm/JdNUMiomJhSPa0bi3rda/swuJHCH//d
wz2AGzZRRGdj7Kna4t6ToxK17lAF3Q6Qp368C9cE6JLMj+3UbY3umWCPRA5/Dms4
/wl3GvDEw7HpyKsvRNPpjDZyiFzZGC2HZmGMsrZMT3hxmyQwmz1O3eGYdO5EIq1S
W/vT1yShZTSusqmICQo5gWWRZTwCENekSbVX9qRr77o0pjKtuBMZTGQTixwpT/rg
Ul7Mr4M2nqK55Kovy/kUN1znfPdW/Fj9iCuvPKwKFdyt2RVgxJDvgIF/bNoRkRxh
wVB6qRgs4EiTrNbRoZAHEFF5wRBf9gWn9HeoI66VtdMZvJRH+0/FDWB4/zwxS16n
nADJaVPXh6JHJFYs9p0wZmvct3GNdWrOLRAG2yzbfFZS8fJcX1PYxXXo4By16yGW
hQIDAQABo4IBYjCCAV4wEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQUO3DR
U+l2JZ1gqMpmD8abrm9UFmowHwYDVR0jBBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYX
jzkwDgYDVR0PAQH/BAQDAgGGMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD
AjB2BggrBgEFBQcBAQRqMGgwJAYIKwYBBQUHMAGGGGh0dHA6Ly9vY3NwLmRpZ2lj
ZXJ0LmNvbTBABggrBgEFBQcwAoY0aHR0cDovL2NhY2VydHMuZGlnaWNlcnQuY29t
L0RpZ2lDZXJ0R2xvYmFsUm9vdEcyLmNydDBCBgNVHR8EOzA5MDegNaAzhjFodHRw
Oi8vY3JsMy5kaWdpY2VydC5jb20vRGlnaUNlcnRHbG9iYWxSb290RzIuY3JsMB0G
A1UdIAQWMBQwCAYGZ4EMAQIBMAgGBmeBDAECAjANBgkqhkiG9w0BAQwFAAOCAQEA
o9sJvBNLQSJ1e7VaG3cSZHBz6zjS70A1gVO1pqsmX34BWDPz1TAlOyJiLlA+eUF4
B2OWHd3F//dJJ/3TaCFunjBhZudv3busl7flz42K/BG/eOdlg0kiUf07PCYY5/FK
YTIch51j1moFlBqbglwkdNIVae2tOu0OdX2JiA+bprYcGxa7eayLetvPiA77ynTc
UNMKOqYB41FZHOXe5IXDI5t2RsDM9dMEZv4+cOb9G9qXcgDar1AzPHEt/39335zC
HofQ0QuItCDCDzahWZci9Nn9hb/SvAtPWHZLkLBG6I0iwGxvMwcTTc9Jnb4Flysr
mQlwKsS2MphOoI23Qq3cSA==
-----END CERTIFICATE-----`,
	// Microsoft Azure RSA TLS Issuing CA 07
	`-----BEGIN CERTIFICATE-----
MIIFrDCCBJSgAwIBAgIQCkOpUJsBNS+JlXnscgi6UDANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jv
c29mdCBBenVyZSBSU0EgVExTIElzc3VpbmcgQ0EgMDcwggIiMA0GCSqGSIb3DQEB
AQUAA4ICDwAwggIKAoICAQC1ZF7KYus5OO3GWqJoR4xznLDNCjocogqeCIVdi4eE
BmF3zIYeuXXNoJAUF+mn86NBt3yMM0559JZDkiSDi9MpA2By4yqQlTHzfbOrvs7I
4LWsOYTEClVFQgzXqa2ps2g855HPQW1hZXVh/yfmbtrCNVa//G7FPDqSdrAQ+M8w
0364kyZApds/RPcqGORjZNokrNzYcGub27vqE6BGP6XeQO5YDFobi9BvvTOO+ZA9
HGIU7FbdLhRm6YP+FO8NRpvterfqZrRt3bTn8GT5LsOTzIQgJMt4/RWLF4EKNc97
CXOSCZFn7mFNx4SzTvy23B46z9dQPfWBfTFaxU5pIa0uVWv+jFjG7l1odu0WZqBd
j0xnvXggu564CXmLz8F3draOH6XS7Ys9sTVM3Ow20MJyHtuA3hBDv+tgRhrGvNRD
MbSzTO6axNWvL46HWVEChHYlxVBCTfSQmpbcAdZOQtUfs9E4sCFrqKcRPdg7ryhY
fGbj3q0SLh55559ITttdyYE+wE4RhODgILQ3MaYZoyiL1E/4jqCOoRaFhF5R++vb
YpemcpWx7unptfOpPRRnnN4U3pqZDj4yXexcyS52Rd8BthFY/cBg8XIR42BPeVRl
OckZ+ttduvKVbvmGf+rFCSUoy1tyRwQNXzqeZTLrX+REqgFDOMVe0I49Frc2/Avw
3wIDAQABo4IBYjCCAV4wEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQUzhUW
O+oCo6Zr2tkr/eWMUr56UKgwHwYDVR0jBBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYX
jzkwDgYDVR0PAQH/BAQDAgGGMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD
AjB2BggrBgEFBQcBAQRqMGgwJAYIKwYBBQUHMAGGGGh0dHA6Ly9vY3NwLmRpZ2lj
ZXJ0LmNvbTBABggrBgEFBQcwAoY0aHR0cDovL2NhY2VydHMuZGlnaWNlcnQuY29t
L0RpZ2lDZXJ0R2xvYmFsUm9vdEcyLmNydDBCBgNVHR8EOzA5MDegNaAzhjFodHRw
Oi8vY3JsMy5kaWdpY2VydC5jb20vRGlnaUNlcnRHbG9iYWxSb290RzIuY3JsMB0G
A1UdIAQWMBQwCAYGZ4EMAQIBMAgGBmeBDAECAjANBgkqhkiG9w0BAQwFAAOCAQEA
bbV8m4/LCSvb0nBF9jb7MVLH/9JjHGbn0QjB4R4bMlGHbDXDWtW9pFqMPrRh2Q76
Bqm+yrrgX83jPZAcvOd7F7+lzDxZnYoFEWhxW9WnuM8Te5x6HBPCPRbIuzf9pSUT
/ozvbKFCDxxgC2xKmgp6NwxRuGcy5KQQh4xkq/hJrnnF3RLakrkUBYFPUneip+wS
BzAfK3jHXnkNCPNvKeLIXfLMsffEzP/j8hFkjWL3oh5yaj1HmlW8RE4Tl/GdUVzQ
D1x42VSusQuRGtuSxLhzBNBeJtyD//2u7wY2uLYpgK0o3X0iIJmwpt7Ovp6Bs4tI
E/peia+Qcdk9Qsr+1VgCGA==
-----END CERTIFICATE-----`,
	// Microsoft Azure RSA TLS Issuing CA 08
	`-----BEGIN CERTIFICATE-----
MIIFrDCCBJSgAwIBAgIQDvt+VH7fD/EGmu5XaW17oDANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMzA2MDgwMDAwMDBaFw0yNjA4MjUyMzU5NTlaMF0xCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xLjAsBgNVBAMTJU1pY3Jv
c29mdCBBenVyZSBSU0EgVExTIElzc3VpbmcgQ0EgMDgwggIiMA0GCSqGSIb3DQEB
AQUAA4ICDwAwggIKAoICAQCy7oIFzcDVZVbomWZtSwrAX8LiKXsbCcwuFL7FHkD5
m67olmOdTueOKhNER5ykFs/meKG1fwzd35/+Q1+KTxcV89IIXmErtSsj8EWu7rdE
AVYnYMFbstqwkIVNEoz4OIM82hn+N5p57zkHGPogzF6TOPRUOK8yYyCPeqnHvoVp
E5b0kZL4QT8bdyhSRQbUsUiSaOuF5y3eZ9Vc92baDkhY7CFZE2ThLLv5PQ0WxzLo
t3t18d2vQP5x29I0n6NFsj37J2d/EH/Z6a/lhAVzKjfYloGcQ1IPyDEIGh9gYJnM
LFZiUbm/GBmlpKVr8M03OWKCR0thRbfnU6UoskrwGrECAnnojFEUw+j8i6gFLBNW
XtBOtYvgl8SHCCVKUUUl4YOfR5zF4OkKirJuUbOmB2AOmLjYJIcabDvxMcmryhQi
nog+/+jgHJnY62opgStkdaImMPzyLB7ZaWVnxpRdtFKO1ZvGkZeRNvbPAUKR2kNe
knuh3NtFvz2dY3xP7AfhyLE/t8vW72nAzlRKz++L70CgCvj/yeObPwaAPDd2sZ0o
j2u/N+k6egGq04e+GBW+QYCSoJ5eAY36il0fu7dYSHYDo7RB5aPTLqnybp8wMeAa
tcagc8U9OM42ghELTaWFARuyoCmgqR7y8fAU9Njhcqrm6+0Xzv/vzMfhL4Ulpf1G
7wIDAQABo4IBYjCCAV4wEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQU9n4v
vYCjSrJwW+vfmh/Y7cphgAcwHwYDVR0jBBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYX
jzkwDgYDVR0PAQH/BAQDAgGGMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD
AjB2BggrBgEFBQcBAQRqMGgwJAYIKwYBBQUHMAGGGGh0dHA6Ly9vY3NwLmRpZ2lj
ZXJ0LmNvbTBABggrBgEFBQcwAoY0aHR0cDovL2NhY2VydHMuZGlnaWNlcnQuY29t
L0RpZ2lDZXJ0R2xvYmFsUm9vdEcyLmNydDBCBgNVHR8EOzA5MDegNaAzhjFodHRw
Oi8vY3JsMy5kaWdpY2VydC5jb20vRGlnaUNlcnRHbG9iYWxSb290RzIuY3JsMB0G
A1UdIAQWMBQwCAYGZ4EMAQIBMAgGBmeBDAECAjANBgkqhkiG9w0BAQwFAAOCAQEA
loABcB94CeH6DWKwa4550BTzLxlTHVNseQJ5SetnPpBuPNLPgOLe9Y7ZMn4ZK6mh
feK7RiMzan4UF9CD5rF3TcCevo3IxrdV+YfBwvlbGYv+6JmX3mAMlaUb23Y2pONo
ixFJEOcAMKKR55mSC5W4nQ6jDfp7Qy/504MQpdjJflk90RHsIZGXVPw/JdbBp0w6
pDb4o5CqydmZqZMrEvbGk1p8kegFkBekp/5WVfd86BdH2xs+GKO3hyiA8iBrBCGJ
fqrijbRnZm7q5+ydXF3jhJDJWfxW5EBYZBJrUz/a+8K/78BjwI8z2VYJpG4t6r4o
tOGB5sEyDPDwqx00Rouu8g==
-----END CERTIFICATE-----`,
	// Microsoft TLS RSA Root G2
	`-----BEGIN CERTIFICATE-----
MIIFiTCCBHGgAwIBAgIQCwxrLEZpF7BHc8ZH1K/AyDANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yNTA1MjEwMDAwMDBaFw0yOTA2MTkyMzU5NTlaMFExCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xIjAgBgNVBAMTGU1pY3Jv
c29mdCBUTFMgUlNBIFJvb3QgRzIwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIK
AoICAQDf6oufR+EoEHGvQdYZ25JX3mur5i7erTpgg7cTmKxbuTILe+ufcidrXUCr
vhgGk7IN0hLtuHT1fy/qqBeU9jMWV4reIHwh3bfarN5OZLBazUt18+8CZE3tUtqj
jwTokfjX+z8Z/U5FOV7oKcPW8mevswCUwY3h8EoYmDn6wAmEM0EFAwWr9HXhU6Uh
klxETOZgV6SQApfH1diTBDJK7YVR7dbFuqA/Noovb0w5qARpIoQ7dRT32T60qdAH
QTiBfkZIHegZ5nC4oKoY3XK/fn21bE4ZcBGEBBOB1GL9nGvxHN3/7Kfg5seNMUu/
8mszzNGMtv6xG6NKqF8OfzF2OD8HR2wBqKylFNqCsF8fbLyJGsASKst7lx8oLjEW
ilNMdWb5fQHWwmCqZY8xnnLLzJst5UQZk1erbo7C2S5lsHIt56HDoX5JHVln1gnU
GBJtwJVFeMnxYGrk9u4GJDtzSloRwj6XYcB47u8TpzDiSjgt7lgXEyC3NirfCzK0
wjixkd0SsEW2fMCxHWKhnd1xEhWWAZ0KCfWx3bPZ4DhCNPZptsOvFnP+1EP4Q+RY
+U+z8+zWPZQ6QDgVqwyG0GTOGmPohJRVCVq2BLbRPpoVx2QRgNAbgg5N/0WesmUH
JR/bmsjG7NZbhVAEnxzLXSCCZ5554t/o8uhvxCByMIblnXUnNQIDAQABo4IBSzCC
AUcwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQU3pGGSLehMVkx8UtfB6nciHna
qHYwHwYDVR0jBBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYXjzkwDgYDVR0PAQH/BAQD
AgGGMBMGA1UdJQQMMAoGCCsGAQUFBwMBMHYGCCsGAQUFBwEBBGowaDAkBggrBgEF
BQcwAYYYaHR0cDovL29jc3AuZGlnaWNlcnQuY29tMEAGCCsGAQUFBzAChjRodHRw
Oi8vY2FjZXJ0cy5kaWdpY2VydC5jb20vRGlnaUNlcnRHbG9iYWxSb290RzIuY3J0
MEIGA1UdHwQ7MDkwN6A1oDOGMWh0dHA6Ly9jcmwzLmRpZ2ljZXJ0LmNvbS9EaWdp
Q2VydEdsb2JhbFJvb3RHMi5jcmwwEwYDVR0gBAwwCjAIBgZngQwBAgIwDQYJKoZI
hvcNAQEMBQADggEBAAu8tCs3dMVLpzYCNsav4RPMipqXG/zjRIzuVADl5EEaRvAL
djT/mVViNaqtipwMWmLMQ8DL6kodvWsdr7EZJWac93luWyWAJIGFx3ktNV9CCXjt
n+Jl1cQgUIIQj2o67RiOSImrpgn44YD8BnUWJyVaj7g6cGwYR/Bj9FMO2RU1IPOR
PRMBoOL6JAhFVnfRZ6kxQtBX/xomvsVD2FepY/+v8zrY9ntLEKKXoc9mvmdnCfm1
TOerGSu/Ij193sb372M4LN1WxPkJUtrf44hv1W1r9whBL44+hjGf8XxK9dZhpEZG
KO9XurBvktjSdyXte6YpzjtyeRHU4KdUbTUrpHo=
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 02
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAAAxJZKFvRCA7IgAAAAAADDANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgwMTIw
MDMwMFoXDTI5MDYwMzIwMDMwMFowVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAwMjCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBALFf
yY9swhGdLUa31wstRz9z5Kg7nDbxaCBFQF5wYUrMSZceyBaSsy13mG08dhwgisMv
DGOfv69rBwYah+MKkNaUAN7gHXT1xc44NZMg+QhaZqjbsyA0nUOFRRIIF3ClrguD
qttEyOtoR1WahF3ZqRjCUoahH2JAZa7U81468pFe21rbtaROBWKY7N0Voa+FJ8ZL
rDKswmimzMnSfTdrxhCQBXkivGPm2X7ZwxCMknFtfeJ2FD0Ki8sjYBC4GBl2xOKh
dtoBzYO9Ae3YGK9XQu4Nha6pkhh5ywEzxk6CbETWKfTPxlF+4ZFi+Iyo6tr5QKBY
yHhumjrUQOdQGMmZHupCPme+dwWLnBsIthM85cE8p4yir0mhkUVlMZgDwPUhu8QP
3x4DFqW+OHlq2puE5aOXX4d3ypb/u1H47yEkwuK1fDl7ROViyRaIHNsTIuz4trEc
AFVOPpZ63AwFHI3jXiMALVv/4lWAQYU2lTD1mZO3buY0RbwzlYZzCimVwZdX1dbu
n8F0w8WgYj530r1tEONpi36oUbDYSsNBvqhP2mrDWCUWHFk8rQ113LE/VRzRdguI
56IxJQN7UUxZKzf+lSRUQqu6J1874QcvdqDAy8t2kR6dpuf9SkDi1I+hPbqGRJ1p
2Bkji1+hg+VlV4tN1nykYypkQ1RHhS8EsKrBL0o/AgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFLgvM6Z8UU9/
Hy3VyBVCOKSyDo8vMBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBACGusqgM8zXYTiHTNvrDXqobFI9g
GF1dNgkZIizyNNI8EMiG/fq7bhDwbokxZH2xDIfoNgtGI8r88DX8dQV3aUm07IKW
lu/qV9VJO8gF5/GyxHrgxCvW/IXBoJNnHGLyCWH6rJjuwG3cGIPYplNMUfRnyGCk
SYR1qcRW0Dx5OTh/JlrXAy7/UJIBU9COSAlKv1APr49CYz4iYl25la+tEonWkVE2
qZHrnRuCxyOR7mYlQWKIzdkQVnChmsvzjEjgkW3qv4dHGvanfUeKlou+t0tm4MB7
rm2wmTV4ydACIEzKDnV40wNz7JFHAgJ6KtGDk8KfhIk1Nn2iRPxzo34EIBWL9uuU
E6C3le07w3Z1LoABEJ2vYMKPFVUwG7v4A1+Y5QQtGrGs9NrpHA6QGOkOypPIyHp/
hoZ2Gp3WkyN5UXNDKJIGmE/clGQt86/K3MqZ9RiwwnHYM0+IO/KTinNTSbW+ZhMg
Fxki/Ug55kLA33b4T+cT6HUXWr5yM9iLAW3oyxTIhld1nD5esMt70bNF7WgLW0AA
txkxhDYDmKQ3oyHrrGPZWLz4N7wxHCZbyHbDgjCyiPYujpqsQ6fxthalQtkV6ycu
GLP2sZhSv89myfSgfHkwtcr7bRL0my0R94CXneQhqcXG3undRwlgikU9gfiuTaZG
h8VmoQHGVMiqVtXE
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 04
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAAAsT5WZ9SptVgAAAAAAACzANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgwMTIw
MDI1OVoXDTI5MDYwMzIwMDI1OVowVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAwNDCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAJw6
JAhaGJLyntVgzeLm+4BH20SuK91tEHAhFUUpqtLH3ObEQrGgjVLgT1w5VQY2TRfB
WGY04oVSn+Kk/sbewsI7hr/KrYcpBusdSR1fgdu3pKxWGtYSh/1fQEnioAxqhMZO
b98kuJqVdFpZf63pPBMVeEDM7NrviDZKkN7qYweUw4NqGq6Y5vFkgFopZwToVvQh
psVGjcjdAqu8BvBsR3gjuziwu/tNcbDfIsN/Gn75napBKtHeaN2VdCU4ZskWEcVZ
PSqxaLmTO2boPOH8p/8sa1DgwLnIcXOTsXe/7apNDgpV2xOccuBprYFM2iP5Bss/
7UKKhowN0gwVJdCGaOt4VqouXAizTTOATu41PC/Den3BZnJgaJD06/YI7BPXiZJf
XFL0h5V4sTbhs0JTbjo3NwfIc3Ueu11uZ8mafMtK88bN8E71hvsUNRlGPZeGcmTd
Qzbv1FeCACIMozrts2VwZfmCpbq40urAaIo1N6BA9f4CiWaoMPiUR2JXR7J7m4zH
lbzrmvbGjESJ2xbmHv3nifyBNTUw6i99iWRSs0YZNOM7V08KGCAx78X9ubEn9pdZ
NfsKwkTW0LLtVU0dV0h1EtfymGAWnsQGnNSufi5lx1PkIiUYMGNqkFfFlLT35U1M
DVTHH6k9TQpGCWLQIyJR4443TMX0AUCZBYLTorBTAgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFFQMvOwY933x
A+KEvjRkRGfPdR9lMBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBAHxIccK2wEWrdA/GP0ni/A/Wdf3N
UNHgS7Oz0aiZX/5dNQ1sC93QrWFgGIk44vC3NdK1IToMDliZOHzU190CTdTc9e6Q
43tnk6is1BtQu8VP5tPxtR7w/5m8IzOwyKimJ9bRW+1vFN5LBxoMUP0O377rT7KY
EMsiKuYd10unrhXRATYJC4ZDT07nxX5co2uDLkk+lIiZi1LTlj9xmCQvN4L6bHTy
vNsGIbu4UGdwJBW2CyKP97kn5AN8hJW3ZgSpklXCvRHHIQpyf2XAYKZQSen2I0gg
Oo6SJqgXjJivFKc9zkytwI6MPETxf/sT+RTXezM9EF5k5yc9DEzicddmzq73TrZk
ulQrt/0D15hnmDeyCmMg5bD72KSNOi5CIpoi9CZVgzAVx6JCs7/QNsU2UqdzZ3pz
blSsvOmJ2KXrH22sJ1DEyOvUHFQpTbu23qvXx/EfFGS6f0cxZe95fRTE8BnkgHbn
OygAm0RvJFf1B9yOWrQAWJdWsQv6CHVx3htTyO698KsiTL1rul2KRFk8JGuqvOl9
i19KTdeMVLCrdpuAKE1FdGUQYCH5jnlf2pL7F4QA28SuglmBPCd3nlb3B8i9vj2R
xZeK5pwPWRZYSGx9pBYy7RbJLaeW9eT9xc9lpN3XAOjJDvSdsqQCgMwb8CjrsDF3
NJ7DzNfImza7xSXi
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 06
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAAA3vac0tciNzVgAAAAAADTANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgwMTIw
MDMwMVoXDTI5MDYwMzIwMDMwMVowVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAwNjCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAMA6
3O0lAmKs1KFQsHRLSvpoHnItA3OBYhuwcRTjN+/jZp2gCThyItWRknozQ1z2e3ku
VknTIZzBVzgMAbMC5vGd1WNYEatYL2jU+9MtrLUKJyVpCEkGFavOSHJh/7y0wNJd
MdGceI32eNhzOjJjg7BuvwRreP7wop6GJOQ0qMX/aFwgk63E9AbzyEwqaBMR3GjJ
eTvmzs6k6TgXpT0nxP6mtVkK6bL+AmR5pm+6SKwr0dFJszzpn18qFsep36B1IaPD
jf9/vnjnCplS96Yni2wPSLmEAgSnIw677sQKlwjcWZw9Hsr/h3KUn3EewxdQItrq
5Ss1hYNd/ILa7oGzwkf6Z0KyK2UvYxjWTNzdun3nvfXhqWOKUqde1S3nIh46tCQz
m3jlEKKQd/YBBziZfHABUYrs2X859cEihTJENpRXJcwOnr5/fz78ZntgsCGpzepk
inb9QoxGwiU4fhAEZ1sjPnILE64/6mbRfH79nkl1runTkuDJfRMUGtWtKUI+8Rkr
Ji5x7sACp2nPYY/d631rda0pmRzmSbqPuma5thB96714U3d28epdz8Pu6xudP31c
YX0WF6UGuxocZtUZtrbzoQ9m0dBtdC3tD/pnbO6Kk1oJ1AlwKjLNWhj77HkauWon
Ah1b6vznIL614ukB0lg3xXOjNcwxaUqKa5te1Ea9AgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFAxda81KNAFg
NDQkAeA/UAWD66hFMBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBAMN7IRVg4E0mXAS3hbmC1eXyI7Vc
ZEHqZawlEK8DD8wM8pQnws+95Pd7kRhqie7pyibPRXbGtdHtScqOkE7bbjmrGKe+
GdG6wLUP8TD02NaPmho9pqumBRz2PoXwyNztvgooOywDxDXAxtGVV0vKc7tPYCbb
3KAHZJkJM6Kuee9DWVwEmhsiXryZjsGwBD7fEoXcC8BOtwekpZiu9SWM5ETTFRyr
tIUgy2S2IYSI6yxgska7/NTJuc6yjfs71c6QO8KJ+Bz1yoepefpVuZ4t269mej8k
jE1ri+3tKa4iNlCBVpLk9moe0Jtir267WQk46CjJd5VuUw79Q+rkupbTM0hoIIdA
GUeWhPBooyuE6CP6vpmyhGQooYCeUk3CGG8zkv+yhGjyoM/sCu54OfqxoMukmeut
eMn9FRVD5FyltEZ5FZ2p7p+aGqjsg5poy5fLyl4qfAEDhKdM7ZLqy6D4Is6POqof
fdRfQ+r3VVvXI9dHr4o49zMQVgUUV/la+kOWk+WqNZrh+aONK09gs2fReMK8xExF
ntTP6qV5mbRsgKxea/w+jLWTYyHLdPOsA1OaifWGVBNIzlaH5wrWhyoRwRKb+1I0
2sBzhfNVJf8gDI/lxJEpPTgIjMTm97Q+KW8C1QMprzVbUWVisUMp0Azxm+ZE4PoM
KWDfAOw0TwsQ6jyn
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 08
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAABHxAKfrBeuhAAAAAAAAETANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgxNDIz
MDM0MFoXDTI5MDYwMzIzMDM0MFowVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAwODCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAOdW
tSZphPC6ib4yyTEHy9WgBJ0sdgI+X31mtN8N9QouoqaVVKURKVPbfJnmmZMuD/n4
hedo1DDxuO5qc1bEfF1hWbiltLwGE+cttQqPyzgu4KYhnj8buvoj4kVElrXgc+9n
qTJ5LeHIdeMCGKMbAgmjVlNrw8mq5PX1n3iWg9dZIcBe/wDsEcG7h+MFxsrZ4Ebu
sNZuxBGjo8O2xIkJi76spN1iTDG4jhrTOQU7viUCzVAWAPnV0/AQbRXCtgz0hozA
46d0+vdh99UDO3MAaqtHU60TQzFovz3HJJ6eGVRh11oIT4JFchuYPZcAF8JfiF6W
PaW8ihg4lXRGbijiy+OnT9Cs26Mga6PyfyiIW3MQ5MKwN9zL5q1J0gZjhTqd8h+5
+/QlptCMhuoVkc/UGsvVOVtlKbdn5cp5QK3xVN40z+o+Yh5Qh2RHizK1aXFkU6E7
K6yGLtIevJCaQjAoTrGj4JnphmqU7k4Fx1MwxV/gpvkJh3bml5SUck+F6QHZc44K
lTFgJB4a94tTD7LbbysFNXtnBFlD9/rOJB9lj1wL2yzPRe7kcgUay0Is+ZAa22bK
7y0JhD7sN8K+DqmU/Q8NliECD65IDH0MzPyhleKes5zDL1TC79p7NGoMZk/uKlcL
VKETn1u878Zjj5YwFLyiQT76L4zI887/da70Q2cBAgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFA+yMoDtf4qc
AIQ45tjX9nCFd+16MBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBALH1cTiVRvY627z2zjtZPaftzKA5
tsGFiJF2d9OJZv6EHbZzPq9z5lcSX9YgzWfHecgBO1xNCfP/tmgt4gGWC31L42Hm
AwjXsYB6kZsumOjCEsaVff4o+6dvsVUwjrEmC3Bd3Szmyl5++1ZVIV53mxSLxBOJ
QvpYuwzdC/r7+JO/mB8OmkUPzpXM0MSWtElZE/e6gpcNBnI/y2EU00OhsB+zzQ0H
Kc0Dzk/Qc+P3B1A/xD5ER97Tj14NUz+KfMIIiY5QK6QnoqcrXHdXcXbGCUFixztD
rcVFsc4nazkf8I8QXba4hBm6xetE/7/KIoV0bLEjiP0GtHOEh/u3OSUaVUerdsog
rFnTLeBDyQ6GuTDOl8m/01f8ZRDDnayFpjT8JxfxeKhCXGW/avXsEr3orIzGr720
WtESmCwsBdPcXwCo6kqzkMNfDk/MGEffOR8w0tHK4IjBYIB2Whh82HX412gslYYc
GfzRoQCQ++/ZZuEeog+c0mWCb59zaAm1772pxD7C0DRtUrCp/lrFWMmga9561S7G
8duFJgbXoOQhfKVE8mrfesrsr5S5hKIVABr1Mgi7XeJePfcEV4qv5+ZHcW8sdFrB
o00ACacNAf4Ys3p/x756lhnDffgY8WA9vST4dIn9WPfBLxE8odWUOUASpReiKbB6
y/9qbWptUU9CiA3R
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 10
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAAA8zIGU37kKuTwAAAAAADzANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgwMTIw
MDMwM1oXDTI5MDYwMzIwMDMwM1owVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAxMDCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAPAM
T1tf/EIctM4/9QcrpoN+yZ15z/bprKV3wzep+vcH9S2Y+BFm60IqDtLRBhn2dxNf
hOzWUZNsIeMOhab/0uz9JIK9BPnvjePhxd110ASaThQ2GfstEqMVwPNvakTVcWzx
S5gbeTD3nBe/fTIJOVs2jKAIu0AslinufL0O+OxtzsFOdsFYLk4ymsd8y8e/t133
NVR4zLGHugXFNQFwBMPoXfixtN9HzUxmmuhn1J4eoCEfM0cFO0QIz2uIUlkyePVB
jiUu0AINAc929y005GedaLGAtk1SsyCXK6VTjHeVtXOAzYj/2pc24+dvMqB18bu/
+jxlqzYRv3b9R/9sh2C+DOXqlvULojcnANHnAjAB1YABwpDO77Pr03hgvgo/+2zG
wtGrJxcXCYR5kUKOdmg3EZvOx3Ypv9Vc4nwNX2dS/W05+lEt37KIA/FhIKr4tLKf
0/oosLWn44O6+kQ7d9yiLCvo4lOImvsMIN6ie06AkHEbfJfU2/w9msGh3urnrkzl
rq92rIfNZLyiNBrTZsNrYXyb9eZZefuADhZrwPEp9O2dl446xCmTBzT/4r+tmlkl
m4YdQ37LbpX1juCpi1eATgvmYH3ASdUEvCDKBNJc6j+MSX8dubpgbde0ZLNcNOo4
8/nB+KkLfrr10fx6G3/bCGV9w5cF7K8vx94M+rI/AgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFNBMg9GOcS49
NLH/m3ksjnTU4ngGMBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBADUZyumodeHYyv0lwTtS4eeeK5Ti
9DrST9oGIlIaARjjorq3txwkMnUNZ0R9nUqCS/rjROlG9gBFCcJS6Wcll8e3i1p3
fEAelOO8jG04KbwnfRISPcvL5MRG4qUBwBDRIPoOA+RD2yaHJazIoLMEal7wQz8P
e/XOI8O3yb773pt9k7OHPt/G2z3J9KxxANKkZYE2WZ8cNuWJ0XqZSntVS8LVjNB5
AXmVDzlDi7MKe5LVWhAYdukdDW8yMfS90RbxqKNn8g6acAzjlq8D9G29FHlqNsPx
tnO7xgvVJkaIVEVwqswfPYtv4+QXpoEA+32DWIDi8jw7oxhiEzZn/0/i5W9qZ+bo
WmQ6oEWdPxcMZofwgSc0ILA1JGQodkN6dJjiK4AJCrywuQdHKSgufeB3QaSMNni6
Mx1WjtkQNYlZgwBpzrd4ve2vgj/OyIkymFkIXeEBlljEZRl9JoWdEJbllcURzoJv
FwZxFQ8svzcyUhVotJWOU12X7ePbEz7BMbF5k3N9cjsbTE8GSRWEc/MdWlEspNRY
4Bm/NUgpYJmr6ntCA76cPRn3R1sLrIJXqg29/yJgMN8sT1fTJdXa/Y4GUU4FNXiY
OKMnMW8xmqmqTaw6RGhgcGj0U2vNsi2uJhiH34xXtfhSwVbnwLFNXwpVaxQrVPs9
qca9YAf4sPRL4+6r
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 12
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAABB9WYYP1k1yQwAAAAAAEDANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgxNDIz
MDMzOVoXDTI5MDYwMzIzMDMzOVowVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAxMjCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAKHG
TgBCMs4LbHALHnm618HNCEhlXTLmoRK/un/49LlfpuKidaPMdQ0mNb4pl6iWKnUo
TTRCk638rRcUAemUhpTO9pdPfzX/uaaseB6h88hlBVGQV5UyrE7hGeH3zCMXBVjZ
ghGwt4DKvgO/a3YO43xMupzkFJfx1SddBW4oR160OYgr6FLRELEboaASwYsuoYl8
wLo0O1SqBxz++ZNEfsspAamx3so6+XLVtpeMME/mOYdwebrBrtzS4nmE/9qknWFT
SLo//8NRd7PQ49pzLGf6CyVCiRZIvG7y2+jesPhICU+s9vJ3qBr2go1jU1h5Rpvv
TPHQGsmNTWpepQKcfBfK5rt8YzF9NHBaaLIAcCe90bIYKENMutS5Z6BVn69ZYyMi
3DklCE3V7uozYYkIei5zoI2NIfdjGQaXaEImqA12cwknJfqkWhA1bErK6n4Gx0Y+
MqIgIE0wpRBuwrk46ncEAX4NKiRQd1XOpUwKfI/O/I7kdVjrq+Ghd86HJtuSqwUF
WgV3JbUArAqZtgC5LjFoIjf2lCGzuD2uDBSKM9d8dLhcJRWeJDy7pheaQxsDQcxz
cPz0XOdW5KgdZIrkSWjRChpWY5LcCo5O9SEqvJCtmeIo4TUzW5CTxYG6fkEgSSA0
wiciw/x8SE7YVqxKybryGpZ3y3WxGd2mxktUEhufAgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFDGlpYlD78es
MRU+SHrjBsbp7bwqMBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBAIvpSERgLgnzdc+XVB99zGCGNpur
hIXJ2S+lopZDMMP/lqi4uwX3RSlmjGNKCwfHmMy3KjTMMPqiurxuX3vP6Yx7h3g0
p0+1m7F3PYBgCibUcMJfwtZbKu/Oot19mHsAsHu01BDZTlPbowPpVD8qNtpsiDl4
PjOe9/EW5M/HbrKrZg0ZvLm8ezsePgP0CezXoa2SQSlLssUOWUn6iKxdi0d65jXv
FPYRfOSmWKcQ/SBGWeUjsSuctga3DNzExktOHySKjskO3JTYo/hm7hnMdxLeVGHI
poenawCSZH4kxZCkO8SXrqV4gvh88CHlZ12mBvNw2kskEGYTgRdfpfGLudwxdvV+
AOGu60olNg8VosFWJMcZYPFTAFoZTwdBSprBnt93sBUGXDPwWQNxpSvO50DR+r/u
sdY3/zfFSfQUC5X2/BOuwSUgDdJ2lf/ettl/+TGAVVmNR7PfuwHl5obG3LR964JV
jPLmFw8Vc4CU8YuUStyGwQxse9CPrp9YpcPsztiJB2ugB6/FhxM7UDYfpvdr2nxh
spxBAlg9L1a/mJjzgS0l4kRnmq0zxIMRrMchgi/a7GfwhYq2meVkNd5ectf7SdM5
O9HIQ5cE3PcHH62mEZW2Y+A09CQ9FQoK1bxf67CbYfFcEy6htrbirmjXVoThyo1P
XoXm1+8l+n5NKWWC
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 14
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAABJ+c5NH51vhoQAAAAAAEjANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgxNDIz
MDM0MVoXDTI5MDYwMzIzMDM0MVowVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAxNDCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBALQ0
O5HV7D0M0P5XR9tDj3H/ASlro7t5dRQHJwq8g9plX9RsHSqmsqA28+gFlKjEMc5F
8cJCovAXh51G1mCU+jzzcH/UWEOIEXj5WrEVjigNT3MwnxkWE981eGAxmkFwBiDF
DsnQkRxgHGA3B8RxfsaFcMM5NSm+/EjQ3TaYXFbjn2smJMp9WbdMixVHbS3vNNyQ
0UtnWVBzBTLwrUSaT+e0qC8oUilP2MShMGJ91UZmzvLeYoUfDGHcXIWkFCqkCch4
6S28IlWc1wagx/uzq+zt1nalPrb54BLUcX07iHXnGOtrJ5sp72g0VrQoWFefhajG
BL9+zQvF+Tzi8isM6WKTe80PC7jmTi/2ze59IkFSnDw2pD36KucFrx0WwwK923MZ
oet9r0JsO6IBBfKWS1BHMfbwsV4MJtnvQaFOdNl/TLfTlgOUFrlggPnLRsFx5hno
UEH3jnhzZcKwrENaEDyijneNs7qrqUf4lJdZe3bV1LoguppP4N0WLu5Jh1TjceLa
6pM9wsGaN4XMxdeyxQHa+W1eLBrjFKSIEUukA97x77XGd3XSRxQnq6F4Y5K98Cqn
aGDWZWZ0IptnXSS5FkK7A9qXVRjnC5waqwWISwi/wliIEJq4Y/Vf7sN3NgrvfYPg
HC39Qo5Fbs/MpwXe+FgPyjUPWpWkE7VL1GX0KucpAgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFFJo9PoSVuP2
2EKvMAtAuDkj9fcrMBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBAFAy7Y42/cuUwX522YzqhW3Cks15
m7hqbu3yszkCcAcdOZjPLxXWHp8oPm98u27+yoXreavUQ0bZlMzWsAcw7g6kCjWm
BVh78k1uKxQzlFrHznpMlsEtbgIzuatjCtP70NO2/pe64JzWNRuADvTM/RSKeEnG
WpU3U09YZzc/qEcvzfsLtqN88GX8/may9tDctPDI8Kkx8jdQYLG9bM+Gnm5b0RQH
Ja65N7W50zo16Jjy3jv1zxm+UOvjt27atgcm+EmocqAzUtws7dxdnrdaBmgqndMC
Jg1tNrQ5UxJfXhCgoVurdC/UYMSCxkPMZ0PI1D7yvmJAFzfUTDXGZw+l3V9JwEOg
u+0/a/QcEVDdXLM4cFM+KvmM6NBGFX+ktBvk8IIq8gld7IdTGohZQ9EmpBa32ZT4
XKU6Atst09IFJYmlr/6X/FaNDeM22Kh7TSlTdjuDA8ybygSVwPjpgKFWho4gAQrX
BhGwff3pRgb2RGDS/Fw91FgLW3NePKcLC6a7u7reXhc/NIBPWoovCE+imo9p9Oem
VTHFF0qvux5MQ78kbeZrxv7x+EU5OK56+jIGpWZFfsdB5La4cwgEkVL7vYfoaRET
T85pMUZup9ZRlYcuqDSfH2r5cokDcwCKjarG8YrjKiQ9i3hLzRs2sQEG3wjf2lrb
B99kBMBp4Ylf6v3t
-----END CERTIFICATE-----`,
	// Microsoft TLS G2 RSA CA OCSP 16
	`-----BEGIN CERTIFICATE-----
MIIHuDCCBaCgAwIBAgITMwAAAA5Ck48l3FGpmwAAAAAADjANBgkqhkiG9w0BAQwF
ADBRMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MSIwIAYDVQQDExlNaWNyb3NvZnQgVExTIFJTQSBSb290IEcyMB4XDTI1MDgwMTIw
MDMwMloXDTI5MDYwMzIwMDMwMlowVzELMAkGA1UEBhMCVVMxHjAcBgNVBAoTFU1p
Y3Jvc29mdCBDb3Jwb3JhdGlvbjEoMCYGA1UEAxMfTWljcm9zb2Z0IFRMUyBHMiBS
U0EgQ0EgT0NTUCAxNjCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAJNy
X7D8oHoR3W/OE5vzT0QuP+ym4r+vL8gHmczj1YdNWzOn5VlHxR2Ue+hTR6PxfOQi
pbhH/gAeXr1wd5YE1XZX/IOzeVFX9CQUTXlJmrfcR5L2PKY1KtG2b18b1mC+0YKi
bzeF5WokVOeIh/A+1wBe2ufVNOMOr+HU+HdaVdRnE/dBSF9PLGB1KAGos1pwhcdY
hQbfoUwroVfZqWy6HIa6AfbQFBoF+Isx5ZXyMTfVEaKYnT/vci9REEBe4uMbQpYG
N2gF5Pq41VRdHuGU2vJRo+Q+e77DrqVBQhY9kdqQvQitSirIRRgwLlD3yqZHw+8D
z0o9fmx8sqe5RhonEpqZEkyiK1ql5aO7ocrOcu9HY7C+c0lHzsKp1US0QY3zRzfM
bAdjHNiWguQ/bnZTZJ3c+MIzrovLWxR0QC0ICE+g8gOUz4LH4jOIUKkf0sF6UCwh
xs3AYjG2/tEC5lOksVJ5lu5lWTnR26I0owa+IWrima4tKugtCDqQWojn8AGp69AE
xCFpDz3Jpn7xvzlygpCXOEy27yV+YfL/DL71ve19R3VW+PbzqOFtgzLIUV/9JpKB
38iUFDKAlq6mCd5M12QokTJaJ5JpZIRKoR68xBG7FVUd0IynFmcgR0RaZ2wYugHe
lDzagm1XcVRDbPLKvM27gBdVztl7jC2dUE/27+iHAgMBAAGjggKBMIICfTAOBgNV
HQ8BAf8EBAMCAYYwEAYJKwYBBAGCNxUBBAMCAQAwHQYDVR0OBBYEFAY58FbR7ZDI
NqOgD5T+YpSn5vw3MBMGA1UdIAQMMAowCAYGZ4EMAQICMBMGA1UdJQQMMAoGCCsG
AQUFBwMBMBkGCSsGAQQBgjcUAgQMHgoAUwB1AGIAQwBBMBIGA1UdEwEB/wQIMAYB
Af8CAQAwHwYDVR0jBBgwFoAU3pGGSLehMVkx8UtfB6nciHnaqHYwgasGA1UdHwSB
ozCBoDCBnaCBmqCBl4ZJaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9j
cmwvTWljcm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyLmNybIZKaHR0cDov
L2NybDIubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01pY3Jvc29mdCUyMFRMUyUy
MFJTQSUyMFJvb3QlMjBHMi5jcmwwggEQBggrBgEFBQcBAQSCAQIwgf8wYwYIKwYB
BQUHMAKGV2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY2VydHMvTWlj
cm9zb2Z0JTIwVExTJTIwUlNBJTIwUm9vdCUyMEcyJTIwLSUyMHhzaWduLmNydDBp
BggrBgEFBQcwAoZdaHR0cDovL2NhaXNzdWVycy5taWNyb3NvZnQuY29tL3BraW9w
cy9jZXJ0cy9NaWNyb3NvZnQlMjBUTFMlMjBSU0ElMjBSb290JTIwRzIlMjAtJTIw
eHNpZ24uY3J0MC0GCCsGAQUFBzABhiFodHRwOi8vb25lb2NzcC5taWNyb3NvZnQu
Y29tL29jc3AwDQYJKoZIhvcNAQEMBQADggIBAIGGI1JWs93TO6gypc7n3H7V5Qim
hS8nVFE3Y3ZNdG7utJvyrxAgO1d7q52kBgwLZ1M8lcluTDmrfCIZu+vs+UyNmZ6J
h+kAJgGwmTKPCqTihbJ/h10jiSoW4JftFu5QMljZdJ14UlLrQTwwfYGxrd0QVnqz
r4S8Q/rP/2DTBQSQj/uLauKBaVKoPQL10IxIkcuIj83C0aMqPUDZWjXgy8dBEej8
tMKgBlK3O5nN5ZkXAPkXjI1FIZRL03QD8besLM+Vb4tlcvb2k8XdQpEv0RK8bjeY
66I+Q2anOq0kQI6oiJ4c/QFEoFLVcJiCTY86hZmTSw1i4Tsnxhwy5N7UtK7SGJ3m
JAJwhdwy3lrMPgShw2yzLlbbODGYqwa7BzpDPQEtEHVdbK78Qv03TWH/w6KQGv2I
FtqjVibfJnsQEgjms0mr6hRODs4G0LIfBqDs4JC2o5AnDc/N2/CDhnVdfHbMrvbc
2fqNxx/4TQevSBliM5pN5s3nQR166CCTmavh92N49ykEb3Q+iHY6hBkI76e/Db4b
daeq7IdaXEMYURG5kj3kn70K4SY3cUCHoRNdkQQzNXB7OIW5jgG65HL9F1uSh9B7
KmJjEVz9Kzh/Kx9y3KEmb4eRyi4tc9CtEkFY3CmW0gbpBXhwmzEGHQ6T08YoSoiR
DpR9auXiVitH82FI
-----END CERTIFICATE-----`,
	// Microsoft RSA TLS CA 01
	`-----BEGIN CERTIFICATE-----
MIIFWjCCBEKgAwIBAgIQDxSWXyAgaZlP1ceseIlB4jANBgkqhkiG9w0BAQsFADBa
MQswCQYDVQQGEwJJRTESMBAGA1UEChMJQmFsdGltb3JlMRMwEQYDVQQLEwpDeWJl
clRydXN0MSIwIAYDVQQDExlCYWx0aW1vcmUgQ3liZXJUcnVzdCBSb290MB4XDTIw
MDcyMTIzMDAwMFoXDTI0MTAwODA3MDAwMFowTzELMAkGA1UEBhMCVVMxHjAcBgNV
BAoTFU1pY3Jvc29mdCBDb3Jwb3JhdGlvbjEgMB4GA1UEAxMXTWljcm9zb2Z0IFJT
QSBUTFMgQ0EgMDEwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQCqYnfP
mmOyBoTzkDb0mfMUUavqlQo7Rgb9EUEf/lsGWMk4bgj8T0RIzTqk970eouKVuL5R
IMW/snBjXXgMQ8ApzWRJCZbar879BV8rKpHoAW4uGJssnNABf2n17j9TiFy6BWy+
IhVnFILyLNK+W2M3zK9gheiWa2uACKhuvgCca5Vw/OQYErEdG7LBEzFnMzTmJcli
W1iCdXby/vI/OxbfqkKD4zJtm45DJvC9Dh+hpzqvLMiK5uo/+aXSJY+SqhoIEpz+
rErHw+uAlKuHFtEjSeeku8eR3+Z5ND9BSqc6JtLqb0bjOHPm5dSRrgt4nnil75bj
c9j3lWXpBb9PXP9Sp/nPCK+nTQmZwHGjUnqlO9ebAVQD47ZisFonnDAmjrZNVqEX
F3p7laEHrFMxttYuD81BdOzxAbL9Rb/8MeFGQjE2Qx65qgVfhH+RsYuuD9dUw/3w
ZAhq05yO6nk07AM9c+AbNtRoEcdZcLCHfMDcbkXKNs5DJncCqXAN6LhXVERCw/us
G2MmCMLSIx9/kwt8bwhUmitOXc6fpT7SmFvRAtvxg84wUkg4Y/Gx++0j0z6StSeN
0EJz150jaHG6WV4HUqaWTb98Tm90IgXAU4AW2GBOlzFPiU5IY9jt+eXC2Q6yC/Zp
TL1LAcnL3Qa/OgLrHN0wiw1KFGD51WRPQ0Sh7QIDAQABo4IBJTCCASEwHQYDVR0O
BBYEFLV2DDARzseSQk1Mx1wsyKkM6AtkMB8GA1UdIwQYMBaAFOWdWTCCR1jMrPoI
VDaGezq1BE3wMA4GA1UdDwEB/wQEAwIBhjAdBgNVHSUEFjAUBggrBgEFBQcDAQYI
KwYBBQUHAwIwEgYDVR0TAQH/BAgwBgEB/wIBADA0BggrBgEFBQcBAQQoMCYwJAYI
KwYBBQUHMAGGGGh0dHA6Ly9vY3NwLmRpZ2ljZXJ0LmNvbTA6BgNVHR8EMzAxMC+g
LaArhilodHRwOi8vY3JsMy5kaWdpY2VydC5jb20vT21uaXJvb3QyMDI1LmNybDAq
BgNVHSAEIzAhMAgGBmeBDAECATAIBgZngQwBAgIwCwYJKwYBBAGCNyoBMA0GCSqG
SIb3DQEBCwUAA4IBAQCfK76SZ1vae4qt6P+dTQUO7bYNFUHR5hXcA2D59CJWnEj5
na7aKzyowKvQupW4yMH9fGNxtsh6iJswRqOOfZYC4/giBO/gNsBvwr8uDW7t1nYo
DYGHPpvnpxCM2mYfQFHq576/TmeYu1RZY29C4w8xYBlkAA8mDJfRhMCmehk7cN5F
JtyWRj2cZj/hOoI45TYDBChXpOlLZKIYiG1giY16vhCRi6zmPzEwv+tk156N6cGS
Vm44jTQ/rs1sa0JSYjzUaYngoFdZC4OfxnIkQvUIA4TOFmPzNPEFdjcZsgbeEz4T
cGHTBPK4R28F44qIMCtHRV55VMX53ev6P3hRddJb
-----END CERTIFICATE-----`,
	// Microsoft RSA TLS CA 02
	`-----BEGIN CERTIFICATE-----
MIIFWjCCBEKgAwIBAgIQD6dHIsU9iMgPWJ77H51KOjANBgkqhkiG9w0BAQsFADBa
MQswCQYDVQQGEwJJRTESMBAGA1UEChMJQmFsdGltb3JlMRMwEQYDVQQLEwpDeWJl
clRydXN0MSIwIAYDVQQDExlCYWx0aW1vcmUgQ3liZXJUcnVzdCBSb290MB4XDTIw
MDcyMTIzMDAwMFoXDTI0MTAwODA3MDAwMFowTzELMAkGA1UEBhMCVVMxHjAcBgNV
BAoTFU1pY3Jvc29mdCBDb3Jwb3JhdGlvbjEgMB4GA1UEAxMXTWljcm9zb2Z0IFJT
QSBUTFMgQ0EgMDIwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQD0wBlZ
qiokfAYhMdHuEvWBapTj9tFKL+NdsS4pFDi8zJVdKQfR+F039CDXtD9YOnqS7o88
+isKcgOeQNTri472mPnn8N3vPCX0bDOEVk+nkZNIBA3zApvGGg/40Thv78kAlxib
MipsKahdbuoHByOB4ZlYotcBhf/ObUf65kCRfXMRQqOKWkZLkilPPn3zkYM5GHxe
I4MNZ1SoKBEoHa2E/uDwBQVxadY4SRZWFxMd7ARyI4Cz1ik4N2Z6ALD3MfjAgEED
woknyw9TGvr4PubAZdqU511zNLBoavar2OAVTl0Tddj+RAhbnX1/zypqk+ifv+d3
CgiDa8Mbvo1u2Q8nuUBrKVUmR6EjkV/dDrIsUaU643v/Wp/uE7xLDdhC5rplK9si
NlYohMTMKLAkjxVeWBWbQj7REickISpc+yowi3yUrO5lCgNAKrCNYw+wAfAvhFkO
eqPm6kP41IHVXVtGNC/UogcdiKUiR/N59IfYB+o2v54GMW+ubSC3BohLFbho/oZZ
5XyulIZK75pwTHmauCIeE5clU9ivpLwPTx9b0Vno9+ApElrFgdY0/YKZ46GfjOC9
ta4G25VJ1WKsMmWLtzyrfgwbYopquZd724fFdpvsxfIvMG5m3VFkThOqzsOttDcU
fyMTqM2pan4txG58uxNJ0MjR03UCEULRU+qMnwIDAQABo4IBJTCCASEwHQYDVR0O
BBYEFP8vf+EG9DjzLe0ljZjC/g72bPz6MB8GA1UdIwQYMBaAFOWdWTCCR1jMrPoI
VDaGezq1BE3wMA4GA1UdDwEB/wQEAwIBhjAdBgNVHSUEFjAUBggrBgEFBQcDAQYI
KwYBBQUHAwIwEgYDVR0TAQH/BAgwBgEB/wIBADA0BggrBgEFBQcBAQQoMCYwJAYI
KwYBBQUHMAGGGGh0dHA6Ly9vY3NwLmRpZ2ljZXJ0LmNvbTA6BgNVHR8EMzAxMC+g
LaArhilodHRwOi8vY3JsMy5kaWdpY2VydC5jb20vT21uaXJvb3QyMDI1LmNybDAq
BgNVHSAEIzAhMAgGBmeBDAECATAIBgZngQwBAgIwCwYJKwYBBAGCNyoBMA0GCSqG
SIb3DQEBCwUAA4IBAQCg2d165dQ1tHS0IN83uOi4S5heLhsx+zXIOwtxnvwCWdOJ
3wFLQaFDcgaMtN79UjMIFVIUedDZBsvalKnx+6l2tM/VH4YAyNPx+u1LFR0joPYp
QYLbNYkedkNuhRmEBesPqj4aDz68ZDI6fJ92sj2q18QvJUJ5Qz728AvtFOat+Ajg
K0PFqPYEAviUKr162NB1XZJxf6uyIjUlnG4UEdHfUqdhl0R84mMtrYINksTzQ2sH
YM8fEhqICtTlcRLr/FErUaPUe9648nziSnA0qKH7rUZqP/Ifmbo+WNZSZG1BbgOh
lk+521W+Ncih3HRbvRBE0LWYT8vWKnfjgZKxwHwJ
-----END CERTIFICATE-----`,
	// Microsoft Azure TLS Issuing CA 01
	`-----BEGIN CERTIFICATE-----
MIIF8zCCBNugAwIBAgIQCq+mxcpjxFFB6jvh98dTFzANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMDA3MjkxMjMwMDBaFw0yNDA2MjcyMzU5NTlaMFkxCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xKjAoBgNVBAMTIU1pY3Jv
c29mdCBBenVyZSBUTFMgSXNzdWluZyBDQSAwMTCCAiIwDQYJKoZIhvcNAQEBBQAD
ggIPADCCAgoCggIBAMedcDrkXufP7pxVm1FHLDNA9IjwHaMoaY8arqqZ4Gff4xyr
RygnavXL7g12MPAx8Q6Dd9hfBzrfWxkF0Br2wIvlvkzW01naNVSkHp+OS3hL3W6n
l/jYvZnVeJXjtsKYcXIf/6WtspcF5awlQ9LZJcjwaH7KoZuK+THpXCMtzD8XNVdm
GW/JI0C/7U/E7evXn9XDio8SYkGSM63aLO5BtLCv092+1d4GGBSQYolRq+7Pd1kR
EkWBPm0ywZ2Vb8GIS5DLrjelEkBnKCyy3B0yQud9dpVsiUeE7F5sY8Me96WVxQcb
OyYdEY/j/9UpDlOG+vA+YgOvBhkKEjiqygVpP8EZoMMijephzg43b5Qi9r5UrvYo
o19oR/8pf4HJNDPF0/FJwFVMW8PmCBLGstin3NE1+NeWTkGt0TzpHjgKyfaDP2tO
4bCk1G7pP2kDFT7SYfc8xbgCkFQ2UCEXsaH/f5YmpLn4YPiNFCeeIida7xnfTvc4
7IxyVccHHq1FzGygOqemrxEETKh8hvDR6eBdrBwmCHVgZrnAqnn93JtGyPLi6+cj
WGVGtMZHwzVvX1HvSFG771sskcEjJxiQNQDQRWHEh3NxvNb7kFlAXnVdRkkvhjpR
GchFhTAzqmwltdWhWDEyCMKC2x/mSZvZtlZGY+g37Y72qHzidwtyW7rBetZJAgMB
AAGjggGtMIIBqTAdBgNVHQ4EFgQUDyBd16FXlduSzyvQx8J3BM5ygHYwHwYDVR0j
BBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYXjzkwDgYDVR0PAQH/BAQDAgGGMB0GA1Ud
JQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjASBgNVHRMBAf8ECDAGAQH/AgEAMHYG
CCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNlcnQu
Y29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20vRGln
aUNlcnRHbG9iYWxSb290RzIuY3J0MHsGA1UdHwR0MHIwN6A1oDOGMWh0dHA6Ly9j
cmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5jcmwwN6A1oDOG
MWh0dHA6Ly9jcmw0LmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5j
cmwwHQYDVR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMBAGCSsGAQQBgjcVAQQD
AgEAMA0GCSqGSIb3DQEBDAUAA4IBAQAlFvNh7QgXVLAZSsNR2XRmIn9iS8OHFCBA
WxKJoi8YYQafpMTkMqeuzoL3HWb1pYEipsDkhiMnrpfeYZEA7Lz7yqEEtfgHcEBs
K9KcStQGGZRfmWU07hPXHnFz+5gTXqzCE2PBMlRgVUYJiA25mJPXfB00gDvGhtYa
+mENwM9Bq1B9YYLyLjRtUz8cyGsdyTIG/bBM/Q9jcV8JGqMU/UjAdh1pFyTnnHEl
Y59Npi7F87ZqYYJEHJM2LGD+le8VsHjgeWX2CJQko7klXvcizuZvUEDTjHaQcs2J
+kPgfyMIOY1DMJ21NxOJ2xPRC/wAh/hzSBRVtoAnyuxtkZ4VjIOh
-----END CERTIFICATE-----`,
	// Microsoft Azure TLS Issuing CA 02
	`-----BEGIN CERTIFICATE-----
MIIF8zCCBNugAwIBAgIQDGrpfM7VmYOGkKAKnqUyFDANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMDA3MjkxMjMwMDBaFw0yNDA2MjcyMzU5NTlaMFkxCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xKjAoBgNVBAMTIU1pY3Jv
c29mdCBBenVyZSBUTFMgSXNzdWluZyBDQSAwMjCCAiIwDQYJKoZIhvcNAQEBBQAD
ggIPADCCAgoCggIBAOBiO1K6Fk4fHI6t3mJkpg7lxoeUgL8tz9wuI2z0UgY8vFra
3VBo7QznC4K3s9jqKWEyIQY11Le0108bSYa/TK0aioO6itpGiigEG+vH/iqtQXPS
u6D804ri0NFZ1SOP9IzjYuQiK6AWntCqP4WAcZAPtpNrNLPBIyiqmiTDS4dlFg1d
skMuVpT4z0MpgEMmxQnrSZ615rBQ25vnVbBNig04FCsh1V3S8ve5Gzh08oIrL/g5
xq95oRrgEeOBIeiegQpoKrLYyo3R1Tt48HmSJCBYQ52Qc34RgxQdZsLXMUrWuL1J
LAZP6yeo47ySSxKCjhq5/AUWvQBP3N/cP/iJzKKKw23qJ/kkVrE0DSVDiIiXWF0c
9abSGhYl9SPl86IHcIAIzwelJ4SKpHrVbh0/w4YHdFi5QbdAp7O5KxfxBYhQOeHy
is01zkpYn6SqUFGvbK8eZ8y9Aclt8PIUftMG6q5BhdlBZkDDV3n70RlXwYvllzfZ
/nV94l+hYp+GLW7jSmpxZLG/XEz4OXtTtWwLV+IkIOe/EDF79KCazW2SXOIvVInP
oi1PqN4TudNv0GyBF5tRC/aBjUqply1YYfeKwgRVs83z5kuiOicmdGZKH9SqU5bn
Kse7IlyfZLg6yAxYyTNe7A9acJ3/pGmCIkJ/9dfLUFc4hYb3YyIIYGmqm2/3AgMB
AAGjggGtMIIBqTAdBgNVHQ4EFgQUAKuR/CFiJpeaqHkbYUGQYKliZ/0wHwYDVR0j
BBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYXjzkwDgYDVR0PAQH/BAQDAgGGMB0GA1Ud
JQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjASBgNVHRMBAf8ECDAGAQH/AgEAMHYG
CCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNlcnQu
Y29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20vRGln
aUNlcnRHbG9iYWxSb290RzIuY3J0MHsGA1UdHwR0MHIwN6A1oDOGMWh0dHA6Ly9j
cmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5jcmwwN6A1oDOG
MWh0dHA6Ly9jcmw0LmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5j
cmwwHQYDVR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMBAGCSsGAQQBgjcVAQQD
AgEAMA0GCSqGSIb3DQEBDAUAA4IBAQAzo/KdmWPPTaYLQW7J5DqxEiBT9QyYGUfe
Zd7TR1837H6DSkFa/mGM1kLwi5y9miZKA9k6T9OwTx8CflcvbNO2UkFW0VCldEGH
iyx5421+HpRxMQIRjligePtOtRGXwaNOQ7ySWfJhRhKcPKe2PGFHQI7/3n+T3kXQ
/SLu2lk9Qs5YgSJ3VhxBUznYn1KVKJWPE07M55kuUgCquAV0PksZj7EC4nK6e/UV
bPumlj1nyjlxhvNud4WYmr4ntbBev6cSbK78dpI/3cr7P/WJPYJuL0EsO3MgjS3e
DCX7NXp5ylue3TcpQfRU8BL+yZC1wqX98R4ndw7X4qfGaE7SlF7I
-----END CERTIFICATE-----`,
	// Microsoft Azure TLS Issuing CA 05
	`-----BEGIN CERTIFICATE-----
MIIF8zCCBNugAwIBAgIQDXvt6X2CCZZ6UmMbi90YvTANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMDA3MjkxMjMwMDBaFw0yNDA2MjcyMzU5NTlaMFkxCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xKjAoBgNVBAMTIU1pY3Jv
c29mdCBBenVyZSBUTFMgSXNzdWluZyBDQSAwNTCCAiIwDQYJKoZIhvcNAQEBBQAD
ggIPADCCAgoCggIBAKplDTmQ9afwVPQelDuu+NkxNJ084CNKnrZ21ABewE+UU4GK
DnwygZdK6agNSMs5UochUEDzz9CpdV5tdPzL14O/GeE2gO5/aUFTUMG9c6neyxk5
tq1WdKsPkitPws6V8MWa5d1L/y4RFhZHUsgxxUySlYlGpNcHhhsyr7EvFecZGA1M
fsitAWVp6hiWANkWKINfRcdt3Z2A23hmMH9MRSGBccHiPuzwrVsSmLwvt3WlRDgO
bJkE40tFYvJ6GXAQiaGHCIWSVObgO3zj6xkdbEFMmJ/zr2Wet5KEcUDtUBhA4dUU
oaPVz69u46V56Vscy3lXu1Ylsk84j5lUPLdsAxtultP4OPQoOTpnY8kxWkH6kgO5
gTKE3HRvoVIjU4xJ0JQ746zy/8GdQA36SaNiz4U3u10zFZg2Rkv2dL1Lv58EXL02
r5q5B/nhVH/M1joTvpRvaeEpAJhkIA9NkpvbGEpSdcA0OrtOOeGtrsiOyMBYkjpB
5nw0cJY1QHOr3nIvJ2OnY+OKJbDSrhFqWsk8/1q6Z1WNvONz7te1pAtHerdPi5pC
HeiXCNpv+fadwP0k8czaf2Vs19nYsgWn5uIyLQL8EehdBzCbOKJy9sl86S4Fqe4H
GyAtmqGlaWOsq2A6O/paMi3BSmWTDbgPLCPBbPte/bsuAEF4ajkPEES3GHP9AgMB
AAGjggGtMIIBqTAdBgNVHQ4EFgQUx7KcfxzjuFrv6WgaqF2UwSZSamgwHwYDVR0j
BBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYXjzkwDgYDVR0PAQH/BAQDAgGGMB0GA1Ud
JQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjASBgNVHRMBAf8ECDAGAQH/AgEAMHYG
CCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNlcnQu
Y29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20vRGln
aUNlcnRHbG9iYWxSb290RzIuY3J0MHsGA1UdHwR0MHIwN6A1oDOGMWh0dHA6Ly9j
cmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5jcmwwN6A1oDOG
MWh0dHA6Ly9jcmw0LmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5j
cmwwHQYDVR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMBAGCSsGAQQBgjcVAQQD
AgEAMA0GCSqGSIb3DQEBDAUAA4IBAQAe+G+G2RFdWtYxLIKMR5H/aVNFjNP7Jdeu
+oZaKaIu7U3NidykFr994jSxMBMV768ukJ5/hLSKsuj/SLjmAfwRAZ+w0RGqi/kO
vPYUlBr/sKOwr3tVkg9ccZBebnBVG+DLKTp2Ox0+jYBCPxla5FO252qpk7/6wt8S
Zk3diSU12Jm7if/jjkhkGB/e8UdfrKoLytDvqVeiwPA5FPzqKoSqN75byLjsIKJE
dNi07SY45hN/RUnsmIoAf93qlaHR/SJWVRhrWt3JmeoBJ2RDK492zF6TGu1moh4a
E6e00YkwTPWreuwvaLB220vWmtgZPs+DSIb2d9hPBdCJgvcho1c7
-----END CERTIFICATE-----`,
	// Microsoft Azure TLS Issuing CA 06
	`-----BEGIN CERTIFICATE-----
MIIF8zCCBNugAwIBAgIQAueRcfuAIek/4tmDg0xQwDANBgkqhkiG9w0BAQwFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMDA3MjkxMjMwMDBaFw0yNDA2MjcyMzU5NTlaMFkxCzAJBgNVBAYTAlVT
MR4wHAYDVQQKExVNaWNyb3NvZnQgQ29ycG9yYXRpb24xKjAoBgNVBAMTIU1pY3Jv
c29mdCBBenVyZSBUTFMgSXNzdWluZyBDQSAwNjCCAiIwDQYJKoZIhvcNAQEBBQAD
ggIPADCCAgoCggIBALVGARl56bx3KBUSGuPc4H5uoNFkFH4e7pvTCxRi4j/+z+Xb
wjEz+5CipDOqjx9/jWjskL5dk7PaQkzItidsAAnDCW1leZBOIi68Lff1bjTeZgMY
iwdRd3Y39b/lcGpiuP2d23W95YHkMMT8IlWosYIX0f4kYb62rphyfnAjYb/4Od99
ThnhlAxGtfvSbXcBVIKCYfZgqRvV+5lReUnd1aNjRYVzPOoifgSx2fRyy1+pO1Uz
aMMNnIOE71bVYW0A1hr19w7kOb0KkJXoALTDDj1ukUEDqQuBfBxReL5mXiu1O7WG
0vltg0VZ/SZzctBsdBlx1BkmWYBW261KZgBivrql5ELTKKd8qgtHcLQA5fl6JB0Q
gs5XDaWehN86Gps5JW8ArjGtjcWAIP+X8CQaWfaCnuRm6Bk/03PQWhgdi84qwA0s
sRfFJwHUPTNSnE8EiGVk2frt0u8PG1pwSQsFuNJfcYIHEv1vOzP7uEOuDydsmCjh
lxuoK2n5/2aVR3BMTu+p4+gl8alXoBycyLmj3J/PUgqD8SL5fTCUegGsdia/Sa60
N2oV7vQ17wjMN+LXa2rjj/b4ZlZgXVojDmAjDwIRdDUujQu0RVsJqFLMzSIHpp2C
Zp7mIoLrySay2YYBu7SiNwL95X6He2kS8eefBBHjzwW/9FxGqry57i71c2cDAgMB
AAGjggGtMIIBqTAdBgNVHQ4EFgQU1cFnOsKjnfR3UltZEjgp5lVou6UwHwYDVR0j
BBgwFoAUTiJUIBiV5uNu5g/6+rkS7QYXjzkwDgYDVR0PAQH/BAQDAgGGMB0GA1Ud
JQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjASBgNVHRMBAf8ECDAGAQH/AgEAMHYG
CCsGAQUFBwEBBGowaDAkBggrBgEFBQcwAYYYaHR0cDovL29jc3AuZGlnaWNlcnQu
Y29tMEAGCCsGAQUFBzAChjRodHRwOi8vY2FjZXJ0cy5kaWdpY2VydC5jb20vRGln
aUNlcnRHbG9iYWxSb290RzIuY3J0MHsGA1UdHwR0MHIwN6A1oDOGMWh0dHA6Ly9j
cmwzLmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5jcmwwN6A1oDOG
MWh0dHA6Ly9jcmw0LmRpZ2ljZXJ0LmNvbS9EaWdpQ2VydEdsb2JhbFJvb3RHMi5j
cmwwHQYDVR0gBBYwFDAIBgZngQwBAgEwCAYGZ4EMAQICMBAGCSsGAQQBgjcVAQQD
AgEAMA0GCSqGSIb3DQEBDAUAA4IBAQB2oWc93fB8esci/8esixj++N22meiGDjgF
+rA2LUK5IOQOgcUSTGKSqF9lYfAxPjrqPjDCUPHCURv+26ad5P/BYtXtbmtxJWu+
cS5BhMDPPeG3oPZwXRHBJFAkY4O4AF7RIAAUW6EzDflUoDHKv83zOiPfYGcpHc9s
kxAInCedk7QSgXvMARjjOqdakor21DTmNIUotxo8kHv5hwRlGhBJwps6fEVi1Bt0
trpM/3wYxlr473WSPUFZPgP1j519kLpWOJ8z09wxay+Br29irPcBYv0GMXlHqThy
8y4m/HyTQeI2IMvMrQnwqPpY+rLIXyviI2vLoI+4xKE4Rn38ZZ8m
-----END CERTIFICATE-----`,
}
