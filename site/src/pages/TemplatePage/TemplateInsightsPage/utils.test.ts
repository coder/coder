import { getDateRangeFilter } from "./utils";

describe("getDateRangeFilter", () => {
  it("returns the start time at the start of the day", () => {
    const date = new Date("2020-01-01T12:00:00.000Z");
    const { start_time } = getDateRangeFilter({
      startDate: date,
      endDate: date,
      now: date,
      isToday: () => false,
    });
    expect(start_time).toEqual("2020-01-01T00:00:00+00:00");
  });

  it("returns the end time at the start of the next day", () => {
    const date = new Date("2020-01-01T12:00:00.000Z");
    const { end_time } = getDateRangeFilter({
      startDate: date,
      endDate: date,
      now: date,
      isToday: () => false,
    });
    expect(end_time).toEqual("2020-01-02T00:00:00+00:00");
  });

  it("returns the end time at the start of the next hour if the end date is today", () => {
    const date = new Date("2020-01-01T12:00:00.000Z");
    const { end_time } = getDateRangeFilter({
      startDate: date,
      endDate: date,
      now: date,
      isToday: () => true,
    });
    expect(end_time).toEqual("2020-01-01T13:00:00+00:00");
  });
});
