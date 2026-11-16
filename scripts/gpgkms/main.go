// Command gpgkms creates OpenPGP signatures with an RSA key held in Google
// Cloud KMS, so the release signing private key never leaves KMS.
//
// The OpenPGP key is derived from the KMS public key and keyCreated. The
// creation time is part of the OpenPGP fingerprint, so it is pinned below and
// must stay the same for the lifetime of the key. RSA PKCS#1 v1.5 signatures are
// deterministic, so the exported public key (including its self-signature) is
// identical every time it is generated.
//
// Usage:
//
//	gpgkms public-key -key <version>
//	gpgkms sign -key <version> [-prepend <sig>] -out <file.asc> <file>
//
// <version> is a CryptoKeyVersion resource name with the algorithm
// RSA_SIGN_PKCS1_4096_SHA256, e.g.
// projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1.
// Credentials come from Application Default Credentials.
//
// -prepend takes an existing binary (unarmored) detached signature, such as
// one made by `gpg --detach-sign`, and writes its signature packets before the
// KMS signature. This lets a release be signed by the old and new keys at
// once, with the old key's signature first.
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"golang.org/x/xerrors"
	"google.golang.org/api/cloudkms/v1"

	"github.com/coder/coder/v2/coderd/util/ptr"
)

const (
	userIDName  = "Coder Release Signing Key"
	userIDEmail = "security@coder.com"
	keyLifetime = 4 * 365 * 24 * time.Hour

	kmsAlgorithm = "RSA_SIGN_PKCS1_4096_SHA256"
)

// keyCreated is the OpenPGP creation time of the release key (2026-11-16
// 00:00:00 UTC). It is part of the fingerprint: never change it.
var keyCreated = time.Unix(1794787200, 0)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "public-key":
		err = runPublicKey(ctx, os.Args[2:])
	case "sign":
		err = runSign(ctx, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpgkms: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: gpgkms public-key -key <version>")
	_, _ = fmt.Fprintln(os.Stderr, "       gpgkms sign -key <version> [-prepend <sig>] -out <file.asc> <file>")
}

type commonFlags struct {
	key string
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.key, "key", "", "Cloud KMS CryptoKeyVersion resource name")
}

func (c *commonFlags) entity(ctx context.Context) (*openpgp.Entity, error) {
	if c.key == "" {
		return nil, xerrors.New("-key is required")
	}
	signer, err := newKMSSigner(ctx, c.key)
	if err != nil {
		return nil, err
	}
	return newEntity(signer, keyCreated)
}

func runPublicKey(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("public-key", flag.ExitOnError)
	var c commonFlags
	c.register(fs)
	_ = fs.Parse(args)

	e, err := c.entity(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "fingerprint: %X\n", e.PrimaryKey.Fingerprint)
	return writePublicKey(os.Stdout, e)
}

func runSign(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	var c commonFlags
	c.register(fs)
	prependPath := fs.String("prepend", "", "binary detached signature whose packets are written before the KMS signature")
	outPath := fs.String("out", "", "path to write the armored detached signature to")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return xerrors.New("exactly one file to sign is required")
	}
	if *outPath == "" {
		return xerrors.New("-out is required")
	}

	var prepend []byte
	if *prependPath != "" {
		var err error
		prepend, err = os.ReadFile(*prependPath)
		if err != nil {
			return xerrors.Errorf("read prepend signature: %w", err)
		}
	}

	e, err := c.entity(ctx)
	if err != nil {
		return err
	}

	message, err := os.Open(fs.Arg(0))
	if err != nil {
		return xerrors.Errorf("open file to sign: %w", err)
	}
	defer message.Close()

	var out bytes.Buffer
	if err := sign(&out, e, message, prepend, time.Now()); err != nil {
		return err
	}
	// #nosec G306 -- detached signatures are public.
	if err := os.WriteFile(*outPath, out.Bytes(), 0o644); err != nil {
		return xerrors.Errorf("write signature: %w", err)
	}
	return nil
}

// newEntity builds the OpenPGP key for signer. The result only depends on the
// signer's public key and created, so it is the same on every call.
func newEntity(signer crypto.Signer, created time.Time) (*openpgp.Entity, error) {
	rsaPub, ok := signer.Public().(*rsa.PublicKey)
	if !ok {
		return nil, xerrors.Errorf("unsupported public key type %T, want RSA", signer.Public())
	}

	// packet.NewSignerPrivateKey only accepts concrete key types, so build the
	// private key by hand. go-crypto signs RSA keys through crypto.Signer.
	pub := packet.NewRSAPublicKey(created, rsaPub)
	priv := &packet.PrivateKey{PublicKey: *pub, PrivateKey: signer}
	e := &openpgp.Entity{
		PrimaryKey: pub,
		PrivateKey: priv,
		Identities: map[string]*openpgp.Identity{},
	}
	err := e.AddUserId(userIDName, "", userIDEmail, &packet.Config{
		DefaultHash:     crypto.SHA256,
		Time:            func() time.Time { return created },
		KeyLifetimeSecs: uint32(keyLifetime / time.Second),
		// go-crypto adds a random salt notation to v4 signatures by default,
		// which would change the exported key every time.
		NonDeterministicSignaturesViaNotation: ptr.Ref(false),
	})
	if err != nil {
		return nil, xerrors.Errorf("self-sign user ID: %w", err)
	}
	return e, nil
}

func writePublicKey(w io.Writer, e *openpgp.Entity) error {
	aw, err := armor.Encode(w, openpgp.PublicKeyType, nil)
	if err != nil {
		return err
	}
	if err := e.Serialize(aw); err != nil {
		return xerrors.Errorf("serialize public key: %w", err)
	}
	if err := aw.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w)
	return err
}

// sign writes an armored detached signature over message. Signature packets in
// prepend come first, followed by the signature from e.
func sign(w io.Writer, e *openpgp.Entity, message io.Reader, prepend []byte, now time.Time) error {
	if len(prepend) > 0 {
		if err := validateSignatures(prepend); err != nil {
			return xerrors.Errorf("prepend signature: %w", err)
		}
	}

	aw, err := armor.Encode(w, openpgp.SignatureType, nil)
	if err != nil {
		return err
	}
	if _, err := aw.Write(prepend); err != nil {
		return err
	}
	err = openpgp.DetachSign(aw, e, message, &packet.Config{
		DefaultHash: crypto.SHA256,
		Time:        func() time.Time { return now },
		// Keep release signatures to the standard subpackets for older verifiers.
		NonDeterministicSignaturesViaNotation: ptr.Ref(false),
	})
	if err != nil {
		return xerrors.Errorf("sign: %w", err)
	}
	if err := aw.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w)
	return err
}

// validateSignatures checks that b is a binary stream of one or more OpenPGP
// signature packets and nothing else.
func validateSignatures(b []byte) error {
	reader := packet.NewReader(bytes.NewReader(b))
	count := 0
	for {
		p, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return xerrors.Errorf("parse packet (the signature must be binary, not armored): %w", err)
		}
		if _, ok := p.(*packet.Signature); !ok {
			return xerrors.Errorf("unexpected %T packet, want only signatures", p)
		}
		count++
	}
	if count == 0 {
		return xerrors.New("no signature packets found")
	}
	return nil
}

// kmsSigner is a crypto.Signer backed by a Cloud KMS RSA PKCS#1 v1.5 SHA-256
// key.
type kmsSigner struct {
	ctx  context.Context
	svc  *cloudkms.ProjectsLocationsKeyRingsCryptoKeysCryptoKeyVersionsService
	name string
	pub  *rsa.PublicKey
}

func newKMSSigner(ctx context.Context, name string) (*kmsSigner, error) {
	svc, err := cloudkms.NewService(ctx)
	if err != nil {
		return nil, xerrors.Errorf("create Cloud KMS client: %w", err)
	}
	versions := svc.Projects.Locations.KeyRings.CryptoKeys.CryptoKeyVersions

	resp, err := versions.GetPublicKey(name).Context(ctx).Do()
	if err != nil {
		return nil, xerrors.Errorf("get public key for %q: %w", name, err)
	}
	if resp.Algorithm != kmsAlgorithm {
		return nil, xerrors.Errorf("key %q has algorithm %s, want %s", name, resp.Algorithm, kmsAlgorithm)
	}
	block, _ := pem.Decode([]byte(resp.Pem))
	if block == nil {
		return nil, xerrors.Errorf("decode public key PEM for %q", name)
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, xerrors.Errorf("parse public key for %q: %w", name, err)
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, xerrors.Errorf("key %q is %T, want RSA", name, parsed)
	}
	return &kmsSigner{ctx: ctx, svc: versions, name: name, pub: pub}, nil
}

func (s *kmsSigner) Public() crypto.PublicKey {
	return s.pub
}

func (s *kmsSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts.HashFunc() != crypto.SHA256 || len(digest) != sha256.Size {
		return nil, xerrors.Errorf("unsupported hash %v: %s only signs SHA-256 digests", opts.HashFunc(), kmsAlgorithm)
	}
	resp, err := s.svc.AsymmetricSign(s.name, &cloudkms.AsymmetricSignRequest{
		Digest: &cloudkms.Digest{Sha256: base64.StdEncoding.EncodeToString(digest)},
	}).Context(s.ctx).Do()
	if err != nil {
		return nil, xerrors.Errorf("Cloud KMS asymmetric sign: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(resp.Signature)
	if err != nil {
		return nil, xerrors.Errorf("decode Cloud KMS signature: %w", err)
	}
	// Check the signature locally so a corrupted response fails here rather
	// than producing an unverifiable release signature.
	if err := rsa.VerifyPKCS1v15(s.pub, crypto.SHA256, digest, sig); err != nil {
		return nil, xerrors.Errorf("verify Cloud KMS signature: %w", err)
	}
	return sig, nil
}
