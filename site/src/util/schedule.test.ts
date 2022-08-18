import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import { Template, Workspace } from "../api/typesGenerated"
import * as Mocks from "../testHelpers/entities"
import {
  deadlineExtensionMax,
  deadlineExtensionMin,
  extractTimezone,
  maxDeadline,
  minDeadline,
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
      const delta = maxDeadline(workspace, template).diff(now)
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
      const delta = maxDeadline(workspace, template).diff(now)
      expect(delta).toBeLessThanOrEqual(deadlineExtensionMax.asMilliseconds())
    })
  })
})

describe("minDeadline", () => {
  it("should never be less than 30 minutes", () => {
    const delta = minDeadline().diff(now)
    expect(delta).toBeGreaterThanOrEqual(deadlineExtensionMin.asMilliseconds())
  })
})
