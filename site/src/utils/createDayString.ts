import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

dayjs.extend(relativeTime);

/**
 * Returns a human-readable string describing the passing of time
 * Broken into its own module for testing purposes
 */
export function createDayString(time: string): string {
  return dayjs().to(dayjs(time));
}
