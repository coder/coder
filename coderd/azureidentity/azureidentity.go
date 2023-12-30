package azureidentity

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"go.mozilla.org/pkcs7"
	"golang.org/x/xerrors"
)

// allowedSigners matches valid common names listed here:
// https://docs.microsoft.com/en-us/azure/virtual-machines/windows/instance-metadata-service?tabs=linux#tabgroup_14
var allowedSigners = regexp.MustCompile(`^(.*\.)?metadata\.(azure\.(com|us|cn)|microsoftazure\.de)$`)

// The pkcs7 library has a global variable that is incremented
// each time a parse occurs.
var pkcs7Mutex sync.Mutex

type metadata struct {
	VMID string `json:"vmId"`
}

type Options struct {
	x509.VerifyOptions
	Offline bool
}

// Validate ensures the signature was signed by an Azure certificate.
// It returns the associated VM ID if successful.
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
	if options.Intermediates == nil {
		options.Intermediates = x509.NewCertPool()
		for _, cert := range Certificates {
			block, rest := pem.Decode([]byte(cert))
			if len(rest) != 0 {
				return "", xerrors.Errorf("invalid certificate. %d bytes remain", len(rest))
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return "", xerrors.Errorf("parse certificate: %w", err)
			}
			options.Intermediates.AddCert(cert)
		}
	}

	_, err = signer.Verify(options.VerifyOptions)
	if err != nil {
		if !errors.As(err, &x509.UnknownAuthorityError{}) {
			return "", xerrors.Errorf("verify signature: %w", err)
		}
		if options.Offline {
			return "", xerrors.Errorf("certificate from %v is not cached: %w", signer.IssuingCertificateURL, err)
		}

		ctx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		for _, certURL := range signer.IssuingCertificateURL {
			req, err := http.NewRequestWithContext(ctx, "GET", certURL, nil)
			if err != nil {
				return "", xerrors.Errorf("new request %q: %w", certURL, err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", xerrors.Errorf("no cached certificate for %q found. error fetching: %w", certURL, err)
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
		_, err = signer.Verify(options.VerifyOptions)
		if err != nil {
			return "", err
		}
	}

	var metadata metadata
	err = json.Unmarshal(pkcs7Data.Content, &metadata)
	if err != nil {
		return "", xerrors.Errorf("unmarshal metadata: %w", err)
	}
	return metadata.VMID, nil
}

// Certificates are manually downloaded from Azure, then processed with OpenSSL
// and added here. See: https://learn.microsoft.com/en-us/azure/security/fundamentals/azure-ca-details
//
// 1. Download the certificate
// 2. Convert to PEM format: `openssl x509 -in cert.pem -text`
// 3. Paste the contents into the array below
var Certificates = []string{
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
MIIHejCCBWKgAwIBAgITMwAAAB2+lJbz24uN5wAAAAAAHTANBgkqhkiG9w0BAQwF
ADBlMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MTYwNAYDVQQDEy1NaWNyb3NvZnQgUlNBIFJvb3QgQ2VydGlmaWNhdGUgQXV0aG9y
aXR5IDIwMTcwHhcNMjAwMTE3MjAyMjQ3WhcNMjQwNjI3MjAyMjQ3WjBZMQswCQYD
VQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9uMSowKAYDVQQD
EyFNaWNyb3NvZnQgQXp1cmUgVExTIElzc3VpbmcgQ0EgMDEwggIiMA0GCSqGSIb3
DQEBAQUAA4ICDwAwggIKAoICAQDHnXA65F7nz+6cVZtRRywzQPSI8B2jKGmPGq6q
meBn3+Mcq0coJ2r1y+4NdjDwMfEOg3fYXwc631sZBdAa9sCL5b5M1tNZ2jVUpB6f
jkt4S91up5f42L2Z1XiV47bCmHFyH/+lrbKXBeWsJUPS2SXI8Gh+yqGbivkx6Vwj
Lcw/FzVXZhlvySNAv+1PxO3r15/Vw4qPEmJBkjOt2izuQbSwr9PdvtXeBhgUkGKJ
Uavuz3dZERJFgT5tMsGdlW/BiEuQy643pRJAZygsstwdMkLnfXaVbIlHhOxebGPD
HvellcUHGzsmHRGP4//VKQ5ThvrwPmIDrwYZChI4qsoFaT/BGaDDIo3qYc4ON2+U
Iva+VK72KKNfaEf/KX+ByTQzxdPxScBVTFvD5ggSxrLYp9zRNfjXlk5BrdE86R44
Csn2gz9rTuGwpNRu6T9pAxU+0mH3PMW4ApBUNlAhF7Gh/3+WJqS5+GD4jRQnniIn
Wu8Z3073OOyMclXHBx6tRcxsoDqnpq8RBEyofIbw0engXawcJgh1YGa5wKp5/dyb
Rsjy4uvnI1hlRrTGR8M1b19R70hRu+9bLJHBIycYkDUA0EVhxIdzcbzW+5BZQF51
XUZJL4Y6URnIRYUwM6psJbXVoVgxMgjCgtsf5kmb2bZWRmPoN+2O9qh84ncLclu6
wXrWSQIDAQABo4ICLTCCAikwDgYDVR0PAQH/BAQDAgGGMBAGCSsGAQQBgjcVAQQD
AgEAMB0GA1UdDgQWBBQPIF3XoVeV25LPK9DHwncEznKAdjBUBgNVHSAETTBLMEkG
BFUdIAAwQTA/BggrBgEFBQcCARYzaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3Br
aW9wcy9Eb2NzL1JlcG9zaXRvcnkuaHRtMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggr
BgEFBQcDAjAZBgkrBgEEAYI3FAIEDB4KAFMAdQBiAEMAQTASBgNVHRMBAf8ECDAG
AQH/AgEAMB8GA1UdIwQYMBaAFAnLWX+GsnCPGsM548DZ6b+7TbIjMHAGA1UdHwRp
MGcwZaBjoGGGX2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01p
Y3Jvc29mdCUyMFJTQSUyMFJvb3QlMjBDZXJ0aWZpY2F0ZSUyMEF1dGhvcml0eSUy
MDIwMTcuY3JsMIGuBggrBgEFBQcBAQSBoTCBnjBtBggrBgEFBQcwAoZhaHR0cDov
L3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9jZXJ0cy9NaWNyb3NvZnQlMjBSU0El
MjBSb290JTIwQ2VydGlmaWNhdGUlMjBBdXRob3JpdHklMjAyMDE3LmNydDAtBggr
BgEFBQcwAYYhaHR0cDovL29uZW9jc3AubWljcm9zb2Z0LmNvbS9vY3NwMA0GCSqG
SIb3DQEBDAUAA4ICAQBsIzB1Oq0LJ9OWJZkajhhidJgsgceMPZaa2EKlBDTb6TF2
kIK1MyzSgA8Z6rITwm6y+pp0xGAMYKAUu6m2GJk/cZrrS6/xKlBSy55tb0gn6rov
Vd4SFDh8Trjwav8vdgcUlY6c/EcL6CT3o3WSZuFkrZmCHpCGsJOgPB88CH3ekK9l
5DCrEr/vvJYHgdAAafNHOuwZjDcDOdsqP5yx0+oUdRzjJYr9xAP9HlEKmEAMqrr8
KPweRRExp5Nf+yAR/FPbhwqBRxtLx4AnYQes15UM1A/GhqmbcV01aagTGsdaD4rq
ilpHZz6kxuGKimfvKnq2L6cbk+lPxvr7fk4OcvoNtMmIYQ3eEYikGa/Lewj1VdjJ
UkoOL5noYdDcHYSaHUCnHNQ1GlYBRCMnieiUoW6eEsOYsy2cPQbhN8kRKyk5cJBn
l7V3cVQHKLawpv2ntgS0HdPGR3DvRLPUCzltKp+vXekrO+jVKngRD4qOF+l4hHHa
NpD6EEEZptjkyh+CJZCTbB8XzB8+Fs2hqEp/JyMHNJIVS3BqMM93ErU6GzyDOL4C
756Maid66w+qBXisqQe/B6Lghlhbl1iYqTFnD67FkXJUfXMSvgS4D6yNIwoVIt9h
L2OtP2pR1gg63M/VobXTgv4hMs+3euIIa50A9yD3BH8b/N+kNqJrk6l7jTxRYQ==
-----END CERTIFICATE-----`,
	// Microsoft Azure TLS Issuing CA 02
	`-----BEGIN CERTIFICATE-----
MIIHejCCBWKgAwIBAgITMwAAAB7GdJ8FhRe00AAAAAAAHjANBgkqhkiG9w0BAQwF
ADBlMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MTYwNAYDVQQDEy1NaWNyb3NvZnQgUlNBIFJvb3QgQ2VydGlmaWNhdGUgQXV0aG9y
aXR5IDIwMTcwHhcNMjAwMTE3MjAyMjQ4WhcNMjQwNjI3MjAyMjQ4WjBZMQswCQYD
VQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9uMSowKAYDVQQD
EyFNaWNyb3NvZnQgQXp1cmUgVExTIElzc3VpbmcgQ0EgMDIwggIiMA0GCSqGSIb3
DQEBAQUAA4ICDwAwggIKAoICAQDgYjtSuhZOHxyOrd5iZKYO5caHlIC/Lc/cLiNs
9FIGPLxa2t1QaO0M5wuCt7PY6ilhMiEGNdS3tNdPG0mGv0ytGoqDuoraRoooBBvr
x/4qrUFz0rug/NOK4tDRWdUjj/SM42LkIiugFp7Qqj+FgHGQD7aTazSzwSMoqpok
w0uHZRYNXbJDLlaU+M9DKYBDJsUJ60meteawUNub51WwTYoNOBQrIdVd0vL3uRs4
dPKCKy/4OcaveaEa4BHjgSHonoEKaCqy2MqN0dU7ePB5kiQgWEOdkHN+EYMUHWbC
1zFK1ri9SSwGT+snqOO8kksSgo4aufwFFr0AT9zf3D/4icyiisNt6if5JFaxNA0l
Q4iIl1hdHPWm0hoWJfUj5fOiB3CACM8HpSeEiqR61W4dP8OGB3RYuUG3QKezuSsX
8QWIUDnh8orNNc5KWJ+kqlBRr2yvHmfMvQHJbfDyFH7TBuquQYXZQWZAw1d5+9EZ
V8GL5Zc32f51feJfoWKfhi1u40pqcWSxv1xM+Dl7U7VsC1fiJCDnvxAxe/Sgms1t
klziL1SJz6ItT6jeE7nTb9BsgRebUQv2gY1KqZctWGH3isIEVbPN8+ZLojonJnRm
Sh/UqlOW5yrHuyJcn2S4OsgMWMkzXuwPWnCd/6RpgiJCf/XXy1BXOIWG92MiCGBp
qptv9wIDAQABo4ICLTCCAikwDgYDVR0PAQH/BAQDAgGGMBAGCSsGAQQBgjcVAQQD
AgEAMB0GA1UdDgQWBBQAq5H8IWIml5qoeRthQZBgqWJn/TBUBgNVHSAETTBLMEkG
BFUdIAAwQTA/BggrBgEFBQcCARYzaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3Br
aW9wcy9Eb2NzL1JlcG9zaXRvcnkuaHRtMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggr
BgEFBQcDAjAZBgkrBgEEAYI3FAIEDB4KAFMAdQBiAEMAQTASBgNVHRMBAf8ECDAG
AQH/AgEAMB8GA1UdIwQYMBaAFAnLWX+GsnCPGsM548DZ6b+7TbIjMHAGA1UdHwRp
MGcwZaBjoGGGX2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01p
Y3Jvc29mdCUyMFJTQSUyMFJvb3QlMjBDZXJ0aWZpY2F0ZSUyMEF1dGhvcml0eSUy
MDIwMTcuY3JsMIGuBggrBgEFBQcBAQSBoTCBnjBtBggrBgEFBQcwAoZhaHR0cDov
L3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9jZXJ0cy9NaWNyb3NvZnQlMjBSU0El
MjBSb290JTIwQ2VydGlmaWNhdGUlMjBBdXRob3JpdHklMjAyMDE3LmNydDAtBggr
BgEFBQcwAYYhaHR0cDovL29uZW9jc3AubWljcm9zb2Z0LmNvbS9vY3NwMA0GCSqG
SIb3DQEBDAUAA4ICAQCfj3iFPoZGiD5Auq1xC3Df2mioOtvocf216fQ5O00JZ/5M
WGa0FbWxX1iw0ydJwUhUPGATJGTbRhldiRCQvdVRd/AEKGwNbSq4ZctxdL3Vyqoi
a/ayfiC5h+2TSsVWYLeofp7YLHGON5BJSHm3+xCtxNfwI+yju7/pe15mkW79FHE2
uTMj2Nd9KiJw9VyoyIcTz8jf7l8/ugZYcSuwLoRaF+UeV8gF9laiJnR7ZkYOdGAA
Go33eFC+IcsIKEDpNTqOGDjh0tgc+tHSY+WS2yacm8ZIeFdaGldgOausvnSn++YO
n4QaWenbSpxfTopM/RdhsUjabhIp+lT56I2pwPJZlmk2NP05NLBkaCeeYe8gSzrD
f7+RiHPQwyrDzvjWIVqzY+isgvj8zNV352UFYVpweeyLXj0gnrh9d2WO6UU7NglZ
KaD/CWdYUyAIa0hAS3zn8aWuwuY5vOWX+amGqz40jhruDVnNKnlKfrkZJRYeLJ9P
P96nam0gd2LRm7HrgGFMlyeDa6/F4Gb2qxPDELS1gjnNPac05VUMz1FxalvuQKjy
CrLNjhyk+Fidz9h+YOYYNksCjUEg2sT9mfLzDAA3FJd+wgYPpZXKyzuM+uwSG3gr
yzv+GGXrYDwL9iWADwOjZhYRyvKZHoSb9PGPpqLQGhA7xzyb+dkh2kvvrAbffQ==
-----END CERTIFICATE-----`,
	// Microsoft Azure TLS Issuing CA 05
	`-----BEGIN CERTIFICATE-----
MIIHejCCBWKgAwIBAgITMwAAAB7GdJ8FhRe00AAAAAAAHjANBgkqhkiG9w0BAQwF
ADBlMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MTYwNAYDVQQDEy1NaWNyb3NvZnQgUlNBIFJvb3QgQ2VydGlmaWNhdGUgQXV0aG9y
aXR5IDIwMTcwHhcNMjAwMTE3MjAyMjQ4WhcNMjQwNjI3MjAyMjQ4WjBZMQswCQYD
VQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9uMSowKAYDVQQD
EyFNaWNyb3NvZnQgQXp1cmUgVExTIElzc3VpbmcgQ0EgMDIwggIiMA0GCSqGSIb3
DQEBAQUAA4ICDwAwggIKAoICAQDgYjtSuhZOHxyOrd5iZKYO5caHlIC/Lc/cLiNs
9FIGPLxa2t1QaO0M5wuCt7PY6ilhMiEGNdS3tNdPG0mGv0ytGoqDuoraRoooBBvr
x/4qrUFz0rug/NOK4tDRWdUjj/SM42LkIiugFp7Qqj+FgHGQD7aTazSzwSMoqpok
w0uHZRYNXbJDLlaU+M9DKYBDJsUJ60meteawUNub51WwTYoNOBQrIdVd0vL3uRs4
dPKCKy/4OcaveaEa4BHjgSHonoEKaCqy2MqN0dU7ePB5kiQgWEOdkHN+EYMUHWbC
1zFK1ri9SSwGT+snqOO8kksSgo4aufwFFr0AT9zf3D/4icyiisNt6if5JFaxNA0l
Q4iIl1hdHPWm0hoWJfUj5fOiB3CACM8HpSeEiqR61W4dP8OGB3RYuUG3QKezuSsX
8QWIUDnh8orNNc5KWJ+kqlBRr2yvHmfMvQHJbfDyFH7TBuquQYXZQWZAw1d5+9EZ
V8GL5Zc32f51feJfoWKfhi1u40pqcWSxv1xM+Dl7U7VsC1fiJCDnvxAxe/Sgms1t
klziL1SJz6ItT6jeE7nTb9BsgRebUQv2gY1KqZctWGH3isIEVbPN8+ZLojonJnRm
Sh/UqlOW5yrHuyJcn2S4OsgMWMkzXuwPWnCd/6RpgiJCf/XXy1BXOIWG92MiCGBp
qptv9wIDAQABo4ICLTCCAikwDgYDVR0PAQH/BAQDAgGGMBAGCSsGAQQBgjcVAQQD
AgEAMB0GA1UdDgQWBBQAq5H8IWIml5qoeRthQZBgqWJn/TBUBgNVHSAETTBLMEkG
BFUdIAAwQTA/BggrBgEFBQcCARYzaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3Br
aW9wcy9Eb2NzL1JlcG9zaXRvcnkuaHRtMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggr
BgEFBQcDAjAZBgkrBgEEAYI3FAIEDB4KAFMAdQBiAEMAQTASBgNVHRMBAf8ECDAG
AQH/AgEAMB8GA1UdIwQYMBaAFAnLWX+GsnCPGsM548DZ6b+7TbIjMHAGA1UdHwRp
MGcwZaBjoGGGX2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01p
Y3Jvc29mdCUyMFJTQSUyMFJvb3QlMjBDZXJ0aWZpY2F0ZSUyMEF1dGhvcml0eSUy
MDIwMTcuY3JsMIGuBggrBgEFBQcBAQSBoTCBnjBtBggrBgEFBQcwAoZhaHR0cDov
L3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9jZXJ0cy9NaWNyb3NvZnQlMjBSU0El
MjBSb290JTIwQ2VydGlmaWNhdGUlMjBBdXRob3JpdHklMjAyMDE3LmNydDAtBggr
BgEFBQcwAYYhaHR0cDovL29uZW9jc3AubWljcm9zb2Z0LmNvbS9vY3NwMA0GCSqG
SIb3DQEBDAUAA4ICAQCfj3iFPoZGiD5Auq1xC3Df2mioOtvocf216fQ5O00JZ/5M
WGa0FbWxX1iw0ydJwUhUPGATJGTbRhldiRCQvdVRd/AEKGwNbSq4ZctxdL3Vyqoi
a/ayfiC5h+2TSsVWYLeofp7YLHGON5BJSHm3+xCtxNfwI+yju7/pe15mkW79FHE2
uTMj2Nd9KiJw9VyoyIcTz8jf7l8/ugZYcSuwLoRaF+UeV8gF9laiJnR7ZkYOdGAA
Go33eFC+IcsIKEDpNTqOGDjh0tgc+tHSY+WS2yacm8ZIeFdaGldgOausvnSn++YO
n4QaWenbSpxfTopM/RdhsUjabhIp+lT56I2pwPJZlmk2NP05NLBkaCeeYe8gSzrD
f7+RiHPQwyrDzvjWIVqzY+isgvj8zNV352UFYVpweeyLXj0gnrh9d2WO6UU7NglZ
KaD/CWdYUyAIa0hAS3zn8aWuwuY5vOWX+amGqz40jhruDVnNKnlKfrkZJRYeLJ9P
P96nam0gd2LRm7HrgGFMlyeDa6/F4Gb2qxPDELS1gjnNPac05VUMz1FxalvuQKjy
CrLNjhyk+Fidz9h+YOYYNksCjUEg2sT9mfLzDAA3FJd+wgYPpZXKyzuM+uwSG3gr
yzv+GGXrYDwL9iWADwOjZhYRyvKZHoSb9PGPpqLQGhA7xzyb+dkh2kvvrAbffQ==
-----END CERTIFICATE-----`,
	// Microsoft Azure TLS Issuing CA 06
	`-----BEGIN CERTIFICATE-----
MIIHejCCBWKgAwIBAgITMwAAACCi8UkaN/vTHwAAAAAAIDANBgkqhkiG9w0BAQwF
ADBlMQswCQYDVQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9u
MTYwNAYDVQQDEy1NaWNyb3NvZnQgUlNBIFJvb3QgQ2VydGlmaWNhdGUgQXV0aG9y
aXR5IDIwMTcwHhcNMjAwMTE3MjAyMjUwWhcNMjQwNjI3MjAyMjUwWjBZMQswCQYD
VQQGEwJVUzEeMBwGA1UEChMVTWljcm9zb2Z0IENvcnBvcmF0aW9uMSowKAYDVQQD
EyFNaWNyb3NvZnQgQXp1cmUgVExTIElzc3VpbmcgQ0EgMDYwggIiMA0GCSqGSIb3
DQEBAQUAA4ICDwAwggIKAoICAQC1RgEZeem8dygVEhrj3OB+bqDRZBR+Hu6b0wsU
YuI//s/l28IxM/uQoqQzqo8ff41o7JC+XZOz2kJMyLYnbAAJwwltZXmQTiIuvC33
9W403mYDGIsHUXd2N/W/5XBqYrj9ndt1veWB5DDE/CJVqLGCF9H+JGG+tq6Ycn5w
I2G/+DnffU4Z4ZQMRrX70m13AVSCgmH2YKkb1fuZUXlJ3dWjY0WFczzqIn4Esdn0
cstfqTtVM2jDDZyDhO9W1WFtANYa9fcO5Dm9CpCV6AC0ww49bpFBA6kLgXwcUXi+
Zl4rtTu1htL5bYNFWf0mc3LQbHQZcdQZJlmAVtutSmYAYr66peRC0yinfKoLR3C0
AOX5eiQdEILOVw2lnoTfOhqbOSVvAK4xrY3FgCD/l/AkGln2gp7kZugZP9Nz0FoY
HYvOKsANLLEXxScB1D0zUpxPBIhlZNn67dLvDxtacEkLBbjSX3GCBxL9bzsz+7hD
rg8nbJgo4ZcbqCtp+f9mlUdwTE7vqePoJfGpV6AcnMi5o9yfz1IKg/Ei+X0wlHoB
rHYmv0mutDdqFe70Ne8IzDfi12tq44/2+GZWYF1aIw5gIw8CEXQ1Lo0LtEVbCahS
zM0iB6adgmae5iKC68kmstmGAbu0ojcC/eV+h3tpEvHnnwQR488Fv/RcRqq8ue4u
9XNnAwIDAQABo4ICLTCCAikwDgYDVR0PAQH/BAQDAgGGMBAGCSsGAQQBgjcVAQQD
AgEAMB0GA1UdDgQWBBTVwWc6wqOd9HdSW1kSOCnmVWi7pTBUBgNVHSAETTBLMEkG
BFUdIAAwQTA/BggrBgEFBQcCARYzaHR0cDovL3d3dy5taWNyb3NvZnQuY29tL3Br
aW9wcy9Eb2NzL1JlcG9zaXRvcnkuaHRtMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggr
BgEFBQcDAjAZBgkrBgEEAYI3FAIEDB4KAFMAdQBiAEMAQTASBgNVHRMBAf8ECDAG
AQH/AgEAMB8GA1UdIwQYMBaAFAnLWX+GsnCPGsM548DZ6b+7TbIjMHAGA1UdHwRp
MGcwZaBjoGGGX2h0dHA6Ly93d3cubWljcm9zb2Z0LmNvbS9wa2lvcHMvY3JsL01p
Y3Jvc29mdCUyMFJTQSUyMFJvb3QlMjBDZXJ0aWZpY2F0ZSUyMEF1dGhvcml0eSUy
MDIwMTcuY3JsMIGuBggrBgEFBQcBAQSBoTCBnjBtBggrBgEFBQcwAoZhaHR0cDov
L3d3dy5taWNyb3NvZnQuY29tL3BraW9wcy9jZXJ0cy9NaWNyb3NvZnQlMjBSU0El
MjBSb290JTIwQ2VydGlmaWNhdGUlMjBBdXRob3JpdHklMjAyMDE3LmNydDAtBggr
BgEFBQcwAYYhaHR0cDovL29uZW9jc3AubWljcm9zb2Z0LmNvbS9vY3NwMA0GCSqG
SIb3DQEBDAUAA4ICAQDGHyDRhzjI21gw4YByzhIRWSok4ZgB/yOIczgpjn7ZZ3Ck
yiTqjI6DOW2nA57R80vIoOSnMSJtdt4R5j5wkyr13P9i3rJMt5m619WH1MD/oLQw
q7iOJm78SRa83AmCoHUHL4zrHwmf7SHzZbaXFznldsrpB0x+RekdgmH3/iIQSsSC
9SSjvFQALyDl6XszM3OTyGAHOsCv1fL6lZN1JJAnYOaYYix8Ox8Bpv0DfL0L11kY
4SIuWoWnxhLqXNupacqh20ZrE+QsPXfCaHf3Tqe48UzomSzN2xUEWZOa1I9o5Iyl
VE+YxMidNtyHp0NIaEu4gOecxRcDzQJm2wuQwPoCoRjjBT5FRjD4igCn0kSDdodo
9QWZxJSLmw9CAKSso2+PJWgNBKsKBEl13kF8QXfobCe+ZX+GgMmQOEIdu7EVCTkY
KldRJwcLgsRI+Y+l9h42WM+OLsOzkcxC8U2Qn1A3R1kW5cesGhhwyCOpikF293Xi
OTQy9AT/MI8UYKHUofB9Enno2R7yzl67TeG04xlNrr7rJyOsWbr2xqG5gN1DXkyZ
uTBdQhR+/Zx+LZOXfh5eK7/L27OVGJuiLsydRJ2O8X6UiDGTMrgW81e71Mu6o7hb
fSxXO15d3jGSEBIxfoDmBxNtp4I4i+YOKBd85a1FkTlrKV4koW/vWpsL+6DWBA==
-----END CERTIFICATE-----`,
}
