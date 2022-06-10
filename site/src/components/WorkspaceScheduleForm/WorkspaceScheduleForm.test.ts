import dayjs from "dayjs"
import { Workspace } from "../../api/typesGenerated"
import * as Mocks from "../../testHelpers/entities"
import { Language, ttlShutdownAt, validationSchema, WorkspaceScheduleFormValues } from "./WorkspaceScheduleForm"
import { zones } from "./zones"

const valid: WorkspaceScheduleFormValues = {
  sunday: false,
  monday: true,
  tuesday: true,
  wednesday: true,
  thursday: true,
  friday: true,
  saturday: false,

  startTime: "09:30",
  timezone: "Canada/Eastern",
  ttl: 120,
}

describe("validationSchema", () => {
  it("allows everything to be falsy", () => {
    const values: WorkspaceScheduleFormValues = {
      sunday: false,
      monday: false,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,

      startTime: "",
      timezone: "",
      ttl: 0,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrow()
  })

  it("disallows ttl to be negative", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: -1,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrow()
  })

  it("disallows all days-of-week to be false when startTime is set", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      sunday: false,
      monday: false,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorNoDayOfWeek)
  })

  it("disallows empty startTime when at least one day is set", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      sunday: false,
      monday: true,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,
      startTime: "",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorNoTime)
  })

  it("allows startTime 16:20", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "16:20",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrow()
  })

  it("disallows startTime to be H:mm", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "9:30",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows startTime to be HH:m", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "09:5",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows an invalid startTime 24:01", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "24:01",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows an invalid startTime 09:60", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      startTime: "09:60",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTime)
  })

  it("disallows an invalid timezone Canada/North", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      timezone: "Canada/North",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(Language.errorTimezone)
  })

  it.each<[string]>(zones.map((zone) => [zone]))(`validation passes for tz=%p`, (zone) => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      timezone: zone,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrow()
  })

  it("allows a ttl of 7 days", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: 24 * 7,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a ttl of 7 days + 1 hour", () => {
    const values: WorkspaceScheduleFormValues = {
      ...valid,
      ttl: 24 * 7 + 1,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError("ttl must be less than or equal to 168")
  })
})

describe("ttlShutdownAt", () => {
  it.each<[string, dayjs.Dayjs, Workspace, string, number, string]>([
    [
      "Workspace is stopped --> helper text",
      dayjs("2022-05-17T18:09:00Z"),
      Mocks.MockStoppedWorkspace,
      "America/Chicago",
      1,
      Language.ttlHelperText,
    ],
    [
      "TTL is not modified --> helper text",
      dayjs("2022-05-17T16:09:00Z"),
      {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: "2022-05-17T18:09:00Z",
        },
        ttl_ms: 2 * 60 * 60 * 1000, // 2 hours = shuts off at 18:09
      },
      "America/Chicago",
      2,
      Language.ttlHelperText,
    ],
    [
      "TTL becomes 0 --> manual helper text",
      dayjs("2022-05-17T18:09:00Z"),
      Mocks.MockWorkspace,
      "America/Chicago",
      0,
      Language.ttlCausesNoShutdownHelperText,
    ],
    [
      "Deadline of 18:09 becomes 17:09 at 17:09 --> immediate shutdown",
      dayjs("2022-05-17T17:09:00Z"),
      {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: "2022-05-17T18:09:00Z",
        },
        ttl_ms: 2 * 60 * 60 * 1000, // 2 hours = shuts off at 18:09
      },
      "America/Chicago",
      1,
      `⚠️ ${Language.ttlCausesShutdownHelperText} ${Language.ttlCausesShutdownImmediately} ⚠️`,
    ],
    [
      "Deadline of 18:09 becomes 17:09 at 16:39 --> display shutdown soon",
      dayjs("2022-05-17T16:39:00Z"),
      {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: "2022-05-17T18:09:00Z",
        },
        ttl_ms: 2 * 60 * 60 * 1000, // 2 hours = shuts off at 18:09
      },
      "America/Chicago",
      1,
      `⚠️ ${Language.ttlCausesShutdownHelperText} ${Language.ttlCausesShutdownSoon} ⚠️`,
    ],
    [
      "Deadline of 18:09 becomes 17:09 at 16:09 --> display 12:09 CDT",
      dayjs("2022-05-17T16:09:00Z"),
      {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: "2022-05-17T18:09:00Z",
        },
        ttl_ms: 2 * 60 * 60 * 1000, // 2 hours = shuts off at 18:09
      },
      "America/Chicago",
      1,
      `${Language.ttlCausesShutdownHelperText} ${Language.ttlCausesShutdownAt} May 17, 2022 12:09 PM.`,
    ],
    [
      "Manual workspace gets new deadline of 18:09 at 17:09 --> display 1:09 CDT",
      dayjs("2022-05-17T17:09:00Z"),
      {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: "0001-01-01T00:00:00Z",
        },
        ttl_ms: 0,
      },
      "America/Chicago",
      1,
      `${Language.ttlCausesShutdownHelperText} ${Language.ttlCausesShutdownAt} May 17, 2022 1:09 PM.`,
    ],
  ])("%p", (_, now, workspace, timezone, ttlHours, expected) => {
    expect(ttlShutdownAt(now, workspace, timezone, ttlHours)).toEqual(expected)
  })
})
