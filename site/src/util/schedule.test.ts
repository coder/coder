import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import { emptySchedule } from "pages/WorkspaceSchedulePage/schedule"
import { emptyTTL } from "pages/WorkspaceSchedulePage/ttl"
import { Workspace } from "../api/typesGenerated"
import * as Mocks from "../testHelpers/entities"
import {
  deadlineExtensionMax,
  deadlineExtensionMin,
  extractTimezone,
  getMaxDeadline,
  getMaxDeadlineChange,
  getMinDeadline,
  stripTimezone,
  scheduleChanged,
} from "./schedule"

dayjs.extend(duration)
const now = dayjs()
const startTime = dayjs(Mocks.MockWorkspaceBuild.updated_at)

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
})

describe("maxDeadline", () => {
  const workspace: Workspace = {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: startTime.add(8, "hours").utc().format(),
    },
  }
  it("should be 24 hours from the workspace start time", () => {
    const delta = getMaxDeadline(workspace).diff(startTime)
    expect(delta).toEqual(deadlineExtensionMax.asMilliseconds())
  })
})

describe("minDeadline", () => {
  it("should never be less than 30 minutes", () => {
    const delta = getMinDeadline().diff(now)
    expect(delta).toBeGreaterThanOrEqual(deadlineExtensionMin.asMilliseconds())
  })
})

describe("getMaxDeadlineChange", () => {
  it("should return the number of hours you can add before hitting the max deadline", () => {
    const deadline = dayjs()
    const maxDeadline = dayjs().add(1, "hour").add(40, "minutes")
    // you can only add one hour even though the max is 1:40 away
    expect(getMaxDeadlineChange(deadline, maxDeadline)).toEqual(1)
  })

  it("should return the number of hours you can subtract before hitting the min deadline", () => {
    const deadline = dayjs().add(2, "hours").add(40, "minutes")
    const minDeadline = dayjs()
    // you can only subtract 2 hours even though the min is 2:40 less
    expect(getMaxDeadlineChange(deadline, minDeadline)).toEqual(2)
  })
})

describe("scheduleChanged", () => {
  describe("autoStart", () => {
    it("should be true if toggle values are different", () => {
      const autoStart = { autoStartEnabled: true, ...emptySchedule }
      const formValues = {
        autoStartEnabled: false,
        ...emptySchedule,
        autoStopEnabled: false,
        ttl: emptyTTL,
      }
      expect(scheduleChanged(autoStart, formValues)).toBe(true)
    })
    it("should be true if schedule values are different", () => {
      const autoStart = { autoStartEnabled: true, ...emptySchedule }
      const formValues = {
        autoStartEnabled: true,
        ...{ ...emptySchedule, monday: true, startTime: "09:00" },
        autoStopEnabled: false,
        ttl: emptyTTL,
      }
      expect(scheduleChanged(autoStart, formValues)).toBe(true)
    })
    it("should be false if all autostart values are the same", () => {
      const autoStart = { autoStartEnabled: true, ...emptySchedule }
      const formValues = {
        autoStartEnabled: true,
        ...emptySchedule,
        autoStopEnabled: false,
        ttl: emptyTTL,
      }
      expect(scheduleChanged(autoStart, formValues)).toBe(false)
    })
  })

  describe("autoStop", () => {
    it("should be true if toggle values are different", () => {
      const autoStop = { autoStopEnabled: true, ttl: 1000 }
      const formValues = {
        autoStartEnabled: false,
        ...emptySchedule,
        autoStopEnabled: false,
        ttl: 1000,
      }
      expect(scheduleChanged(autoStop, formValues)).toBe(true)
    })
    it("should be true if ttl values are different", () => {
      const autoStop = { autoStopEnabled: true, ttl: 1000 }
      const formValues = {
        autoStartEnabled: false,
        ...emptySchedule,
        autoStopEnabled: true,
        ttl: 2000,
      }
      expect(scheduleChanged(autoStop, formValues)).toBe(true)
    })
    it("should be false if all autostop values are the same", () => {
      const autoStop = { autoStopEnabled: true, ttl: 1000 }
      const formValues = {
        autoStartEnabled: false,
        ...emptySchedule,
        autoStopEnabled: true,
        ttl: 1000,
      }
      expect(scheduleChanged(autoStop, formValues)).toBe(false)
    })
  })
})
