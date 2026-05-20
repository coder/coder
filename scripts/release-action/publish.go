package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// publishRelease creates a GitHub release with the given assets,
// generates checksums, and optionally GPG-signs them.
func publishRelease(versionTag, channel, notesFile string, assets []string, gpgKeyBase64 string) error {
	if len(assets) == 0 {
		return xerrors.New("no assets provided")
	}

	// Validate all asset files exist.
	for _, f := range assets {
		if _, err := os.Stat(f); err != nil {
			return xerrors.Errorf("asset not found: %s", f)
		}
	}

	// Verify we're checked out on the expected tag.
	described, err := gitOutput("describe", "--always")
	if err != nil {
		return xerrors.Errorf("git describe: %w", err)
	}
	if described != versionTag {
		return xerrors.Errorf("checked-out ref %q does not match release tag %q", described, versionTag)
	}

	// Create a temp directory with symlinks to all assets.
	tempDir, err := os.MkdirTemp("", "release-publish-*")
	if err != nil {
		return xerrors.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	for _, f := range assets {
		abs, err := filepath.Abs(f)
		if err != nil {
			return xerrors.Errorf("abs path for %s: %w", f, err)
		}
		if err := os.Symlink(abs, filepath.Join(tempDir, filepath.Base(f))); err != nil {
			return xerrors.Errorf("symlink %s: %w", f, err)
		}
	}

	// Generate checksums file.
	version := strings.TrimPrefix(versionTag, "v")
	checksumFile := fmt.Sprintf("coder_%s_checksums.txt", version)
	checksumPath := filepath.Join(tempDir, checksumFile)
	if err := generateChecksums(tempDir, checksumPath); err != nil {
		return xerrors.Errorf("generate checksums: %w", err)
	}

	// GPG-sign the checksums file if a key is provided.
	if gpgKeyBase64 != "" {
		if err := gpgSignFile(checksumPath, gpgKeyBase64); err != nil {
			return xerrors.Errorf("gpg sign checksums: %w", err)
		}
	}

	// Determine target commitish from release branch.
	targetCommitish := "main"
	branchRef, err := gitOutput("branch", "--remotes", "--contains", versionTag, "--format", "%(refname)", "*/release/*")
	if err == nil && branchRef != "" {
		// refs/remotes/origin/release/2.9 -> release/2.9
		if idx := strings.Index(branchRef, "release/"); idx >= 0 {
			targetCommitish = branchRef[idx:]
		}
	}

	// Build gh release create arguments.
	ghArgs := []string{
		"release", "create",
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--title", versionTag,
		"--target", targetCommitish,
		"--notes-file", notesFile,
	}

	switch channel {
	case "stable":
		ghArgs = append(ghArgs, "--latest=true")
	case "rc":
		ghArgs = append(ghArgs, "--prerelease", "--latest=false")
	default:
		ghArgs = append(ghArgs, "--latest=false")
	}

	ghArgs = append(ghArgs, versionTag)

	// Add all files from the temp directory.
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return xerrors.Errorf("read temp dir: %w", err)
	}
	for _, e := range entries {
		ghArgs = append(ghArgs, filepath.Join(tempDir, e.Name()))
	}

	cmd := exec.Command("gh", ghArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader("") // prevent interactive prompts
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("gh release create: %w", err)
	}

	return nil
}

// generateChecksums writes SHA256 checksums for all files in dir
// (excluding the output file itself) to outPath.
func generateChecksums(dir, outPath string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		hash, err := sha256File(path)
		if err != nil {
			return xerrors.Errorf("hash %s: %w", e.Name(), err)
		}
		lines = append(lines, fmt.Sprintf("%s  %s", hash, e.Name()))
	}

	return os.WriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// sha256File returns the hex-encoded SHA256 hash of a file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// gpgSignFile signs the file at path using the base64-encoded GPG
// private key and writes the detached signature to path+".asc".
func gpgSignFile(path, keyBase64 string) error {
	keyBytes, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return xerrors.Errorf("decode GPG key: %w", err)
	}

	// Create a temporary GPG home directory.
	gpgHome, err := os.MkdirTemp("", "gpg-sign-*")
	if err != nil {
		return xerrors.Errorf("create gpg home: %w", err)
	}
	defer os.RemoveAll(gpgHome)

	// Import the key.
	importCmd := exec.Command("gpg", "--homedir", gpgHome, "--import")
	importCmd.Stdin = strings.NewReader(string(keyBytes))
	importCmd.Stderr = os.Stderr
	if err := importCmd.Run(); err != nil {
		return xerrors.Errorf("gpg import: %w", err)
	}

	// Get the fingerprint and mark as trusted.
	fpOut, err := exec.Command("gpg", "--homedir", gpgHome, "--with-colons", "--fingerprint").Output()
	if err != nil {
		return xerrors.Errorf("gpg fingerprint: %w", err)
	}
	var fingerprint string
	for _, line := range strings.Split(string(fpOut), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) >= 10 && parts[0] == "fpr" {
			fingerprint = parts[9]
			break
		}
	}
	if fingerprint == "" {
		return xerrors.New("could not determine GPG key fingerprint")
	}

	trustCmd := exec.Command("gpg", "--homedir", gpgHome, "--import-ownertrust")
	trustCmd.Stdin = strings.NewReader(fingerprint + ":6:\n")
	trustCmd.Stderr = os.Stderr
	if err := trustCmd.Run(); err != nil {
		return xerrors.Errorf("gpg trust: %w", err)
	}

	// Sign the file.
	signCmd := exec.Command("gpg", "--homedir", gpgHome, "--detach-sign", "--armor", path)
	signCmd.Stdin = strings.NewReader("") // prevent interactive prompts
	signCmd.Stderr = os.Stderr
	if err := signCmd.Run(); err != nil {
		return xerrors.Errorf("gpg sign: %w", err)
	}

	// Verify the signature.
	verifyCmd := exec.Command("gpg", "--homedir", gpgHome, "--verify", path+".asc", path)
	verifyCmd.Stderr = os.Stderr
	if err := verifyCmd.Run(); err != nil {
		return xerrors.Errorf("gpg signature verification failed: %w", err)
	}

	return nil
}
