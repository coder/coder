import dayjs from "dayjs"

/**
 * Returns a human-readable string describing when the workspace was created
 * Broken into its own module for testing purposes
 */
export function createDayString(createdAt: string): string {
  return dayjs().to(dayjs(createdAt))
}
