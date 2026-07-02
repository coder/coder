package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// publishRelease creates a GitHub release with the given assets
// and generates checksums.
func publishRelease(exec CommandExecutor, versionTag string, stable bool, notesFile string, assets []string) error {
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
	described, err := gitOutput(exec, "describe", "--always")
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

	// Determine target commitish from release branch.
	targetCommitish := "main"
	branchRef, err := gitOutput(exec, "branch", "--remotes", "--contains", versionTag, "--format", "%(refname)", "*/release/*")
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

	// RC detection from the version tag.
	isRC := strings.Contains(versionTag, "-rc.")
	switch {
	case isRC:
		ghArgs = append(ghArgs, "--prerelease", "--latest=false")
	case stable:
		ghArgs = append(ghArgs, "--latest=true")
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

	if err := exec.RunMutationStdout(os.Stdout, os.Stderr, "gh", ghArgs...); err != nil {
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

	return os.WriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
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
