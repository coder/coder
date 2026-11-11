package agentgitsign

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
)

// sshsigMagic is the preamble of the SSHSIG format defined in
// https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.sshsig.
const sshsigMagic = "SSHSIG"

// SignSSHSig produces an armored SSHSIG signature, the format written by
// `ssh-keygen -Y sign`, over a message digest computed by the caller. Signing
// happens in memory, so the private key is never written to disk.
func SignSSHSig(signer ssh.Signer, namespace, hashAlgorithm string, digest []byte) (string, error) {
	if namespace == "" {
		return "", xerrors.New("namespace is required")
	}
	switch hashAlgorithm {
	case "sha256", "sha512":
	default:
		return "", xerrors.Errorf("unsupported hash algorithm %q", hashAlgorithm)
	}

	signedData := append([]byte(sshsigMagic), ssh.Marshal(struct {
		Namespace     string
		Reserved      string
		HashAlgorithm string
		Hash          string
	}{namespace, "", hashAlgorithm, string(digest)})...)

	var (
		sig *ssh.Signature
		err error
	)
	// RSA keys must not sign with the SHA-1 based "ssh-rsa" algorithm,
	// which ssh-keygen rejects for SSHSIG.
	if as, ok := signer.(ssh.AlgorithmSigner); ok && signer.PublicKey().Type() == ssh.KeyAlgoRSA {
		sig, err = as.SignWithAlgorithm(rand.Reader, signedData, ssh.KeyAlgoRSASHA512)
	} else {
		sig, err = signer.Sign(rand.Reader, signedData)
	}
	if err != nil {
		return "", xerrors.Errorf("sign: %w", err)
	}

	blob := append([]byte(sshsigMagic), ssh.Marshal(struct {
		Version       uint32
		PublicKey     string
		Namespace     string
		Reserved      string
		HashAlgorithm string
		Signature     string
	}{1, string(signer.PublicKey().Marshal()), namespace, "", hashAlgorithm, string(ssh.Marshal(sig))})...)

	return armorSSHSig(blob), nil
}

func armorSSHSig(blob []byte) string {
	encoded := base64.StdEncoding.EncodeToString(blob)
	var b strings.Builder
	_, _ = b.WriteString("-----BEGIN SSH SIGNATURE-----\n")
	for len(encoded) > 70 {
		_, _ = b.WriteString(encoded[:70])
		_ = b.WriteByte('\n')
		encoded = encoded[70:]
	}
	_, _ = b.WriteString(encoded)
	_, _ = b.WriteString("\n-----END SSH SIGNATURE-----\n")
	return b.String()
}
