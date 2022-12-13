import { createDisplayDate } from "./TimelineDateRow"

describe("createDisplayDate", () => {
  it("returns correctly for Saturdays", () => {
    const now = new Date()
    const date = new Date(
      now.getFullYear(),
      now.getMonth(),
      // Previous Saturday, from now.
      now.getDate() - now.getDay() - 1,
    )
    expect(createDisplayDate(date)).toEqual("last Saturday")
  })
})
