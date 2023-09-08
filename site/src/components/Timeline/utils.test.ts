import { createDisplayDate } from "./utils";

describe("createDisplayDate", () => {
  it("returns correctly for Saturdays", () => {
    const now = new Date(2020, 1, 7);
    const date = new Date(
      now.getFullYear(),
      now.getMonth(),
      // Previous Saturday, from now.
      now.getDate() - now.getDay() - 1,
    );
    expect(createDisplayDate(date, now)).toEqual("last Saturday");
  });
});
