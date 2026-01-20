import type { WorkspaceAgentMetadata } from "api/typesGenerated";

export const ZERO_TIME_ISO = "0001-01-01T00:00:00Z";

const getYear = (iso: string): number => {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) {
		return Number.NaN;
	}
	return d.getUTCFullYear();
};

export const isValidAgentMetadataSample = (
	metadata: readonly WorkspaceAgentMetadata[],
): boolean => {
	if (metadata.length === 0) {
		return false;
	}

	// Treat as valid if we have at least one item that has a real collected_at
	// timestamp (i.e. not zero-time and not obviously invalid).
	return metadata.some((item) => {
		if (item.result.collected_at === ZERO_TIME_ISO) {
			return false;
		}
		const year = getYear(item.result.collected_at);
		return year > 1970 && !Number.isNaN(year);
	});
};

export const isInvalidAgentMetadataSample = (
	metadata: readonly WorkspaceAgentMetadata[],
): boolean => {
	// Consider "invalid" the specific failure mode we observed:
	// - collected_at is zero time or looks uninitialized for all entries.
	if (metadata.length === 0) {
		return true;
	}
	const allZeroTime = metadata.every((m) => {
		if (m.result.collected_at === ZERO_TIME_ISO) {
			return true;
		}
		const year = getYear(m.result.collected_at);
		return year <= 1970 || Number.isNaN(year);
	});
	return allZeroTime;
};

