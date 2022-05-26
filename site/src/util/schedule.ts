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

/**
 * WeeklyFlag is an array representing which days of the week are set or flagged
 *
 * @remarks
 *
 * A WeeklyFlag has an array size of 7 and should never have its size modified.
 * The 0th index is Sunday
 * The 6th index is Saturday
 */
export type WeeklyFlag = [boolean, boolean, boolean, boolean, boolean, boolean, boolean]

/**
 * dowToWeeklyFlag converts a dow cron string to a WeeklyFlag array.
 *
 * @example
 *
 * dowToWeeklyFlag("1") // [false, true, false, false, false, false, false]
 * dowToWeeklyFlag("1-5") // [false, true, true, true, true, true, false]
 * dowToWeeklyFlag("1,3-4,6") // [false, true, false, true, true, false, true]
 */
export const dowToWeeklyFlag = (dow: string): WeeklyFlag => {
  if (dow === "*") {
    return [true, true, true, true, true, true, true]
  }

  const results: WeeklyFlag = [false, false, false, false, false, false, false]

  const commaSeparatedRangeOrNum = dow.split(",")

  for (const rangeOrNum of commaSeparatedRangeOrNum) {
    const flags = processRangeOrNum(rangeOrNum)

    flags.forEach((value, idx) => {
      if (value) {
        results[idx] = true
      }
    })
  }

  return results
}

/**
 * processRangeOrNum is a helper for dowToWeeklyFlag. It processes a range or
 * number (modulo 7) into a Weeklyflag boolean array.
 *
 * @example
 *
 * processRangeOrNum("1") // [false, true, false, false, false, false, false]
 * processRangeOrNum("1-5") // [false, true, true, true, true, true, false]
 */
const processRangeOrNum = (rangeOrNum: string): WeeklyFlag => {
  const result: WeeklyFlag = [false, false, false, false, false, false, false]

  const isRange = /^[0-9]-[0-9]$/.test(rangeOrNum)

  if (isRange) {
    const [first, last] = rangeOrNum.split("-")

    for (let i = Number(first); i <= Number(last); i++) {
      result[i % 7] = true
    }
  } else {
    result[Number(rangeOrNum) % 7] = true
  }

  return result
}
