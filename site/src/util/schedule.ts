/**
 * @fileoverview Client-side counterpart of the coderd/autostart/schedule Go
 * package. This package is a variation on crontab that uses minute, hour and
 * day of week.
 */

/**
 * DEFAULT_TIMEZONE is the default timezone that crontab assumes unless one is
 * specified.
 */
const DEFAULT_TIMEZONE = "UTC"

/**
 * stripTimezone strips a leading timezone from a schedule string
 */
export const stripTimezone = (raw: string): string => {
  return raw.replace(/CRON_TZ=\S*\s/, "")
}

/**
 * extractTimezone returns a leading timezone from a schedule string if one is
 * specified; otherwise DEFAULT_TIMEZONE
 */
export const extractTimezone = (raw: string): string => {
  const matches = raw.match(/CRON_TZ=\S*\s/g)

  if (matches && matches.length) {
    return matches[0].replace(/CRON_TZ=/, "").trim()
  } else {
    return DEFAULT_TIMEZONE
  }
}

/**
 * expandScheduleCronString ensures a Schedule is expanded to a valid 5-value
 * cron string by inserting '*' in month and day positions. If there is a
 * leading timezone, it is removed.
 *
 * @example
 * expandScheduleCronString("30 9 1-5") // -> "30 9 * * 1-5"
 */
export const expandScheduleCronString = (schedule: string): string => {
  const prepared = stripTimezone(schedule).trim()

  const parts = prepared.split(" ")

  while (parts.length < 5) {
    // insert '*' in the second to last position
    // ie [a, b, c] --> [a, b, *, c]
    parts.splice(parts.length - 1, 0, "*")
  }

  return parts.join(" ")
}
