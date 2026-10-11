/**
 * Floored like the license usage card so these figures never read higher
 * than the license total.
 */
export const usedAgentHours = (ms: number): number =>
	Number.isFinite(ms) ? Math.floor(ms / 360_000) / 10 : 0;

export const formatUsedAgentHours = (ms: number): string =>
	usedAgentHours(ms).toLocaleString("en-US", {
		minimumFractionDigits: 1,
		maximumFractionDigits: 1,
	});
