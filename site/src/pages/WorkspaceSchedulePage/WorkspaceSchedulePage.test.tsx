import {
  formValuesToAutoStartRequest,
  formValuesToTTLRequest,
} from "pages/WorkspaceSchedulePage/formToRequest"
import {
  AutoStart,
  scheduleToAutoStart,
} from "pages/WorkspaceSchedulePage/schedule"
import { AutoStop, ttlMsToAutoStop } from "pages/WorkspaceSchedulePage/ttl"
import * as TypesGen from "../../api/typesGenerated"
import { WorkspaceScheduleFormValues } from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"

const validValues: WorkspaceScheduleFormValues = {
  autoStartEnabled: true,
  sunday: false,
  monday: true,
  tuesday: true,
  wednesday: true,
  thursday: true,
  friday: true,
  saturday: false,
  startTime: "09:30",
  timezone: "Canada/Eastern",
  autoStopEnabled: true,
  ttl: 120,
}

describe("WorkspaceSchedulePage", () => {
  describe("formValuesToAutoStartRequest", () => {
    it.each<
      [WorkspaceScheduleFormValues, TypesGen.UpdateWorkspaceAutostartRequest]
    >([
      [
        // Empty case
        {
          autoStartEnabled: false,
          sunday: false,
          monday: false,
          tuesday: false,
          wednesday: false,
          thursday: false,
          friday: false,
          saturday: false,
          startTime: "",
          timezone: "",
          autoStopEnabled: false,
          ttl: 0,
        },
        {
          schedule: "",
        },
      ],
      [
        // Single day
        {
          autoStartEnabled: true,
          sunday: true,
          monday: false,
          tuesday: false,
          wednesday: false,
          thursday: false,
          friday: false,
          saturday: false,
          startTime: "16:20",
          timezone: "Canada/Eastern",
          autoStopEnabled: true,
          ttl: 120,
        },
        {
          schedule: "CRON_TZ=Canada/Eastern 20 16 * * 0",
        },
      ],
      [
        // Standard 1-5 case
        {
          autoStartEnabled: true,
          sunday: false,
          monday: true,
          tuesday: true,
          wednesday: true,
          thursday: true,
          friday: true,
          saturday: false,
          startTime: "09:30",
          timezone: "America/Central",
          autoStopEnabled: true,
          ttl: 120,
        },
        {
          schedule: "CRON_TZ=America/Central 30 09 * * 1-5",
        },
      ],
      [
        // Everyday
        {
          autoStartEnabled: true,
          sunday: true,
          monday: true,
          tuesday: true,
          wednesday: true,
          thursday: true,
          friday: true,
          saturday: true,
          startTime: "09:00",
          timezone: "",
          autoStopEnabled: true,
          ttl: 60 * 8,
        },
        {
          schedule: "00 09 * * *",
        },
      ],
      [
        // Mon, Wed, Fri Evenings
        {
          autoStartEnabled: true,
          sunday: false,
          monday: true,
          tuesday: false,
          wednesday: true,
          thursday: false,
          friday: true,
          saturday: false,
          startTime: "16:20",
          timezone: "",
          autoStopEnabled: true,
          ttl: 60 * 3,
        },
        {
          schedule: "20 16 * * 1,3,5",
        },
      ],
    ])(`formValuesToAutoStartRequest(%p) return %p`, (values, request) => {
      expect(formValuesToAutoStartRequest(values)).toEqual(request)
    })
  })

  describe("formValuesToTTLRequest", () => {
    it.each<[WorkspaceScheduleFormValues, TypesGen.UpdateWorkspaceTTLRequest]>([
      [
        // 0 case
        {
          ...validValues,
          ttl: 0,
        },
        {
          ttl_ms: undefined,
        },
      ],
      [
        // 2 Hours = 7.2e+12 case
        {
          ...validValues,
          ttl: 2,
        },
        {
          ttl_ms: 7_200_000,
        },
      ],
      [
        // 8 hours = 2.88e+13 case
        {
          ...validValues,
          ttl: 8,
        },
        {
          ttl_ms: 28_800_000,
        },
      ],
    ])(`formValuesToTTLRequest(%p) returns %p`, (values, request) => {
      expect(formValuesToTTLRequest(values)).toEqual(request)
    })
  })

  describe("scheduleToAutoStart", () => {
    it.each<[string | undefined, AutoStart]>([
      // Empty case
      [
        undefined,
        {
          autoStartEnabled: false,
          sunday: false,
          monday: false,
          tuesday: false,
          wednesday: false,
          thursday: false,
          friday: false,
          saturday: false,
          startTime: "",
          timezone: "",
        },
      ],

      // Basic case: 9:30 1-5 UTC
      [
        "CRON_TZ=UTC 30 9 * * 1-5",
        {
          autoStartEnabled: true,
          sunday: false,
          monday: true,
          tuesday: true,
          wednesday: true,
          thursday: true,
          friday: true,
          saturday: false,
          startTime: "09:30",
          timezone: "UTC",
        },
      ],

      // Complex case: 4:20 1 3-4 6 Canada/Eastern
      [
        "CRON_TZ=Canada/Eastern 20 16 * * 1,3-4,6",
        {
          autoStartEnabled: true,
          sunday: false,
          monday: true,
          tuesday: false,
          wednesday: true,
          thursday: true,
          friday: false,
          saturday: true,
          startTime: "16:20",
          timezone: "Canada/Eastern",
        },
      ],
    ])(`scheduleToAutoStart(%p) returns %p`, (schedule, autoStart) => {
      expect(scheduleToAutoStart(schedule)).toEqual(autoStart)
    })
  })

  describe("ttlMsToAutoStop", () => {
    it.each<[number | undefined, AutoStop]>([
      // empty case
      [undefined, { autoStopEnabled: false, ttl: 0 }],
      // zero
      [0, { autoStopEnabled: false, ttl: 0 }],
      // basic case
      [28_800_000, { autoStopEnabled: true, ttl: 8 }],
    ])(`ttlMsToAutoStop(%p) returns %p`, (ttlMs, autoStop) => {
      expect(ttlMsToAutoStop(ttlMs)).toEqual(autoStop)
    })
  })
})
