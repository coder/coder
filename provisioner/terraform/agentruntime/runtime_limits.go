package agentruntime

import "golang.org/x/xerrors"

const (
	maxRuntimeReferenceCount         = 100_000
	maxRuntimeReferenceBytes         = 16 << 20
	maxRuntimeCandidateChecks        = 1_000_000
	maxRuntimeCandidateMatchBytes    = 256 << 20
	maxRuntimeReferenceCacheEntries  = 100_000
	maxRuntimeReferenceCacheKeyBytes = 16 << 20
	maxRuntimeDiagnosticCandidates   = 20
)

type resolverLimits struct {
	referenceCount         int
	referenceBytes         int
	candidateChecks        int
	candidateMatchBytes    int
	referenceCacheEntries  int
	referenceCacheKeyBytes int
}

func defaultResolverLimits() resolverLimits {
	return resolverLimits{
		referenceCount:         maxRuntimeReferenceCount,
		referenceBytes:         maxRuntimeReferenceBytes,
		candidateChecks:        maxRuntimeCandidateChecks,
		candidateMatchBytes:    maxRuntimeCandidateMatchBytes,
		referenceCacheEntries:  maxRuntimeReferenceCacheEntries,
		referenceCacheKeyBytes: maxRuntimeReferenceCacheKeyBytes,
	}
}

type resolutionBudget struct {
	candidateChecks     int
	candidateMatchBytes int
}

func exceedsLimit(used, additional, limit int) bool {
	return used > limit || additional > limit-used
}

func (b *resolutionBudget) consumeCandidateChecks(count, limit int) error {
	if exceedsLimit(b.candidateChecks, count, limit) {
		return xerrors.Errorf(
			"agent runtime resolution exceeds the limit of %d agent runtime candidate checks",
			limit,
		)
	}
	b.candidateChecks += count
	return nil
}

func (b *resolutionBudget) consumeCandidateMatchBytes(count, limit int) error {
	if exceedsLimit(b.candidateMatchBytes, count, limit) {
		return xerrors.Errorf(
			"agent runtime resolution exceeds the limit of %d agent runtime candidate match bytes",
			limit,
		)
	}
	b.candidateMatchBytes += count
	return nil
}
