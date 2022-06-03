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
 * specified; otherwise the specified defaultTZ
 */
export const extractTimezone = (raw: string, defaultTZ = DEFAULT_TIMEZONE): string => {
  const matches = raw.match(/CRON_TZ=\S*\s/g)

  if (matches && matches.length) {
    return matches[0].replace(/CRON_TZ=/, "").trim()
  } else {
    return defaultTZ
  }
}
