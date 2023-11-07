import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import { Workspace } from "api/typesGenerated";
import * as Mocks from "testHelpers/entities";
import {
  deadlineExtensionMax,
  deadlineExtensionMin,
  extractTimezone,
  getMaxDeadline,
  getMaxDeadlineChange,
  getMinDeadline,
  stripTimezone,
  quietHoursDisplay,
} from "./schedule";

dayjs.extend(duration);
const now = dayjs();
const startTime = dayjs(Mocks.MockWorkspaceBuild.updated_at);

describe("util/schedule", () => {
  describe("stripTimezone", () => {
    it.each<[string, string]>([
      ["CRON_TZ=Canada/Eastern 30 9 1-5", "30 9 1-5"],
      ["CRON_TZ=America/Central 0 8 1,2,4,5", "0 8 1,2,4,5"],
      ["30 9 1-5", "30 9 1-5"],
    ])(`stripTimezone(%p) returns %p`, (input, expected) => {
      expect(stripTimezone(input)).toBe(expected);
    });
  });

  describe("extractTimezone", () => {
    it.each<[string, string]>([
      ["CRON_TZ=Canada/Eastern 30 9 1-5", "Canada/Eastern"],
      ["CRON_TZ=America/Central 0 8 1,2,4,5", "America/Central"],
      ["30 9 1-5", "UTC"],
    ])(`extractTimezone(%p) returns %p`, (input, expected) => {
      expect(extractTimezone(input)).toBe(expected);
    });
  });

  describe("maxDeadline", () => {
    const workspace: Workspace = {
      ...Mocks.MockWorkspace,
      latest_build: {
        ...Mocks.MockWorkspaceBuild,
        deadline: startTime.add(8, "hours").utc().format(),
      },
    };
    it("should be 24 hours from the workspace start time", () => {
      const delta = getMaxDeadline(workspace).diff(startTime);
      expect(delta).toEqual(deadlineExtensionMax.asMilliseconds());
    });
  });

  describe("minDeadline", () => {
    it("should never be less than 30 minutes", () => {
      const delta = getMinDeadline().diff(now);
      expect(delta).toBeGreaterThanOrEqual(
        deadlineExtensionMin.asMilliseconds(),
      );
    });
  });

  describe("getMaxDeadlineChange", () => {
    it("should return the number of hours you can add before hitting the max deadline", () => {
      const deadline = dayjs();
      const maxDeadline = dayjs().add(1, "hour").add(40, "minutes");
      // you can only add one hour even though the max is 1:40 away
      expect(getMaxDeadlineChange(deadline, maxDeadline)).toEqual(1);
    });

    it("should return the number of hours you can subtract before hitting the min deadline", () => {
      const deadline = dayjs().add(2, "hours").add(40, "minutes");
      const minDeadline = dayjs();
      // you can only subtract 2 hours even though the min is 2:40 less
      expect(getMaxDeadlineChange(deadline, minDeadline)).toEqual(2);
    });
  });

  describe("quietHoursDisplay", () => {
    const quietHoursStart = quietHoursDisplay(
      "00:00",
      "Australia/Sydney",
      new Date("2023-09-06T15:00:00.000+10:00"),
    );

    expect(quietHoursStart).toBe(
      "12:00AM tomorrow (in 9 hours) in Australia/Sydney",
    );
  });
});
