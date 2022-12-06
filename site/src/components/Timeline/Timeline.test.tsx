import { createDisplayDate } from "./TimelineDateRow"

describe("createDisplayDate", () => {
  it("returns correctly for Saturdays", () => {
    const date = new Date("Sat Dec 03 2022 00:00:00 GMT-0500")
    expect(createDisplayDate(date)).toEqual("last Saturday")
  })
})
