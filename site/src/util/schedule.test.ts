import { dowToWeeklyFlag, extractTimezone, stripTimezone, WeeklyFlag } from "./schedule"

describe("util/schedule", () => {
  describe("stripTimezone", () => {
    it.each<[string, string]>([
      ["CRON_TZ=Canada/Eastern 30 9 1-5", "30 9 1-5"],
      ["CRON_TZ=America/Central 0 8 1,2,4,5", "0 8 1,2,4,5"],
      ["30 9 1-5", "30 9 1-5"],
    ])(`stripTimezone(%p) returns %p`, (input, expected) => {
      expect(stripTimezone(input)).toBe(expected)
    })
  })

  describe("extractTimezone", () => {
    it.each<[string, string]>([
      ["CRON_TZ=Canada/Eastern 30 9 1-5", "Canada/Eastern"],
      ["CRON_TZ=America/Central 0 8 1,2,4,5", "America/Central"],
      ["30 9 1-5", "UTC"],
    ])(`extractTimezone(%p) returns %p`, (input, expected) => {
      expect(extractTimezone(input)).toBe(expected)
    })
  })

  describe("dowToWeeklyFlag", () => {
    it.each<[string, WeeklyFlag]>([
      // All days
      ["*", [true, true, true, true, true, true, true]],
      ["0-6", [true, true, true, true, true, true, true]],
      ["1-7", [true, true, true, true, true, true, true]],

      // Single number modulo 7
      ["3", [false, false, false, true, false, false, false]],
      ["0", [true, false, false, false, false, false, false]],
      ["7", [true, false, false, false, false, false, false]],
      ["8", [false, true, false, false, false, false, false]],

      // Comma-separated Numbers, Ranges and Mixes
      ["1,3,5", [false, true, false, true, false, true, false]],
      ["1-2,4-5", [false, true, true, false, true, true, false]],
      ["1,3-4,6", [false, true, false, true, true, false, true]],
    ])(`dowToWeeklyFlag(%p) returns %p`, (dow, weeklyFlag) => {
      expect(dowToWeeklyFlag(dow)).toEqual(weeklyFlag)
    })
  })
})
