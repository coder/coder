import { formatDistance, parseISO } from "date-fns";

/**
 * Returns a human-readable string describing the passing of time
 * Broken into its own module for testing purposes
 */
export function createDayString(time: string): string {
	return formatDistance(parseISO(time), new Date(), { addSuffix: true });
}
