package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/xerrors"
)

// prepareRelease computes the next release version, then creates and
// pushes the annotated tag and (optionally) the release branch.
// It emits the same JSON as calculateNextVersion so the workflow
// can consume it identically.
func prepareRelease(exec CommandExecutor, releaseType, ref, commitSHA string) (calculateResult, error) {
	result, err := calculateNextVersion(exec, releaseType, ref, commitSHA)
	if err != nil {
		return nil, err
	}

	switch v := result.(type) {
	case CreateBranchRequest:
		if err := createAndPushTag(exec, v.Version, v.TargetRef); err != nil {
			return nil, err
		}
		if err := createAndPushBranch(exec, v.BranchName, v.TargetRef); err != nil {
			return nil, err
		}
	case ReleaseRequest:
		if err := createAndPushTag(exec, v.Version, v.TargetRef); err != nil {
			return nil, err
		}
	default:
		return nil, xerrors.Errorf("unexpected result type %T", result)
	}

	return result, nil
}

// createAndPushTag creates an annotated tag at targetRef and pushes
// it. If the tag already exists at the correct commit, it is a
// no-op. If it exists at a different commit, it returns an error.
func createAndPushTag(exec CommandExecutor, versionTag, targetRef string) error {
	// Check if the tag already exists locally. Dereference the tag
	// object to the underlying commit with ^{}.
	existing, err := gitOutput(exec, "rev-parse", "--verify", fmt.Sprintf("refs/tags/%s^{}", versionTag))
	if err == nil {
		if existing == targetRef {
			_, _ = fmt.Fprintf(os.Stderr, "tag %s already exists at %s, skipping\n", versionTag, targetRef)
			return nil
		}
		return xerrors.Errorf("tag %s already exists at %s, expected %s", versionTag, existing, targetRef)
	}

	// Create annotated tag.
	if err := gitMutate(exec, "tag", "-a", versionTag, "-m", fmt.Sprintf("Release %s", versionTag), targetRef); err != nil {
		return xerrors.Errorf("create tag %s: %w", versionTag, err)
	}

	// Push tag using explicit refspec.
	refspec := fmt.Sprintf("refs/tags/%s:refs/tags/%s", versionTag, versionTag)
	if err := gitMutate(exec, "push", "origin", refspec); err != nil {
		return xerrors.Errorf("push tag %s: %w", versionTag, err)
	}

	return nil
}

// createAndPushBranch creates a branch at targetRef and pushes it.
// If the branch already exists at the correct commit on the remote,
// it is a no-op. If it exists at a different commit, it returns an
// error.
func createAndPushBranch(exec CommandExecutor, branchName, targetRef string) error {
	// Check if the branch already exists on the remote.
	existing, err := gitOutput(exec, "ls-remote", "--exit-code", "origin", fmt.Sprintf("refs/heads/%s", branchName))
	if err == nil && existing != "" {
		// ls-remote output format: "<sha>\trefs/heads/<branch>"
		remoteSHA, _, _ := strings.Cut(existing, "\t")
		if remoteSHA == targetRef {
			_, _ = fmt.Fprintf(os.Stderr, "branch %s already exists at %s, skipping\n", branchName, targetRef)
			return nil
		}
		return xerrors.Errorf("branch %s already exists at %s, expected %s", branchName, remoteSHA, targetRef)
	}

	// Push the commit directly to create the remote branch, without
	// needing a local branch.
	refspec := fmt.Sprintf("%s:refs/heads/%s", targetRef, branchName)
	if err := gitMutate(exec, "push", "origin", refspec); err != nil {
		return xerrors.Errorf("push branch %s: %w", branchName, err)
	}

	return nil
}
