import { getTimeSince } from "./time"

describe("util/time", () => {
  describe("getTimeSince", () => {
    it("should be at least a year", () => {
      const date = new Date()
      date.setFullYear(2000)
      const since = getTimeSince(date)
      expect(since).toContain("year")
    })
  })
})
