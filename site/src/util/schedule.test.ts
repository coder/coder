import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import { Template, Workspace } from "../api/typesGenerated"
import * as Mocks from "../testHelpers/entities"
import {
  canExtendDeadline,
  canReduceDeadline,
  deadlineExtensionMax,
  deadlineExtensionMin,
  extractTimezone,
  getMaxDeadline,
  getMinDeadline,
  stripTimezone,
} from "./schedule"

dayjs.extend(duration)
const now = dayjs()

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
  // Given: a workspace built from a template with a max deadline equal to 25 hours which isn't really possible
  const workspace: Workspace = {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: now.add(8, "hours").utc().format(),
    },
  }
  describe("given a template with 25 hour max ttl", () => {
    it("should be never be greater than global max deadline", () => {
      const template: Template = {
        ...Mocks.MockTemplate,
        max_ttl_ms: 25 * 60 * 60 * 1000,
      }

      // Then: deadlineMinusDisabled should be falsy
      const delta = getMaxDeadline(workspace, template).diff(now)
      expect(delta).toBeLessThanOrEqual(deadlineExtensionMax.asMilliseconds())
    })
  })

  describe("given a template with 4 hour max ttl", () => {
    it("should be never be greater than global max deadline", () => {
      const template: Template = {
        ...Mocks.MockTemplate,
        max_ttl_ms: 4 * 60 * 60 * 1000,
      }

      // Then: deadlineMinusDisabled should be falsy
      const delta = getMaxDeadline(workspace, template).diff(now)
      expect(delta).toBeLessThanOrEqual(deadlineExtensionMax.asMilliseconds())
    })
  })
})

describe("minDeadline", () => {
  it("should never be less than 30 minutes", () => {
    const delta = getMinDeadline().diff(now)
    expect(delta).toBeGreaterThanOrEqual(deadlineExtensionMin.asMilliseconds())
  })
})

describe("canExtendDeadline", () => {
  it("should be falsy if the deadline is more than 24 hours in the future", () => {
    expect(
      canExtendDeadline(dayjs().add(25, "hours"), Mocks.MockWorkspace, Mocks.MockTemplate),
    ).toBeFalsy()
  })

  it("should be falsy if the deadline is more than the template max_ttl", () => {
    const tooFarAhead = dayjs().add(dayjs.duration(Mocks.MockTemplate.max_ttl_ms, "milliseconds"))
    expect(canExtendDeadline(tooFarAhead, Mocks.MockWorkspace, Mocks.MockTemplate)).toBeFalsy()
  })

  it("should be truth if the deadline is within the template max_ttl", () => {
    const okDeadline = dayjs().add(
      dayjs.duration(Mocks.MockTemplate.max_ttl_ms / 2, "milliseconds"),
    )
    expect(canExtendDeadline(okDeadline, Mocks.MockWorkspace, Mocks.MockTemplate)).toBeFalsy()
  })
})

describe("canReduceDeadline", () => {
  it("should be falsy if the deadline is 30 minutes or less in the future", () => {
    expect(canReduceDeadline(dayjs())).toBeFalsy()
    expect(canReduceDeadline(dayjs().add(1, "minutes"))).toBeFalsy()
    expect(canReduceDeadline(dayjs().add(29, "minutes"))).toBeFalsy()
    expect(canReduceDeadline(dayjs().add(30, "minutes"))).toBeFalsy()
  })

  it("should be truthy if the deadline is 30 minutes or more in the future", () => {
    expect(canReduceDeadline(dayjs().add(31, "minutes"))).toBeTruthy()
    expect(canReduceDeadline(dayjs().add(100, "years"))).toBeTruthy()
  })
})
