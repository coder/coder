import dayjs from "dayjs"

/**
 * Returns a human-readable string describing the passing of time
 * Broken into its own module for testing purposes
 */
export function createDayString(time: string): string {
  return dayjs().to(dayjs(time))
}
