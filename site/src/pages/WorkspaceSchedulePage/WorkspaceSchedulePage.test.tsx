import * as TypesGen from "../../api/typesGenerated"
import { WorkspaceScheduleFormValues } from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import * as Mocks from "../../testHelpers/entities"
import { formValuesToAutoStartRequest, formValuesToTTLRequest, workspaceToInitialValues } from "./WorkspaceSchedulePage"

const validValues: WorkspaceScheduleFormValues = {
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

describe("WorkspaceSchedulePage", () => {
  describe("formValuesToAutoStartRequest", () => {
    it.each<[WorkspaceScheduleFormValues, TypesGen.UpdateWorkspaceAutostartRequest]>([
      [
        // Empty case
        {
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
        },
        {
          schedule: "",
        },
      ],
      [
        // Single day
        {
          sunday: true,
          monday: false,
          tuesday: false,
          wednesday: false,
          thursday: false,
          friday: false,
          saturday: false,
          startTime: "16:20",
          timezone: "Canada/Eastern",
          ttl: 120,
        },
        {
          schedule: "CRON_TZ=Canada/Eastern 20 16 * * 0",
        },
      ],
      [
        // Standard 1-5 case
        {
          sunday: false,
          monday: true,
          tuesday: true,
          wednesday: true,
          thursday: true,
          friday: true,
          saturday: false,
          startTime: "09:30",
          timezone: "America/Central",
          ttl: 120,
        },
        {
          schedule: "CRON_TZ=America/Central 30 09 * * 1-5",
        },
      ],
      [
        // Everyday
        {
          sunday: true,
          monday: true,
          tuesday: true,
          wednesday: true,
          thursday: true,
          friday: true,
          saturday: true,
          startTime: "09:00",
          timezone: "",
          ttl: 60 * 8,
        },
        {
          schedule: "00 09 * * *",
        },
      ],
      [
        // Mon, Wed, Fri Evenings
        {
          sunday: false,
          monday: true,
          tuesday: false,
          wednesday: true,
          thursday: false,
          friday: true,
          saturday: false,
          startTime: "16:20",
          timezone: "",
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
          ttl: undefined,
        },
      ],
      [
        // 2 Hours = 7.2e+12 case
        {
          ...validValues,
          ttl: 2,
        },
        {
          ttl: 7_200_000_000_000,
        },
      ],
      [
        // 8 hours = 2.88e+13 case
        {
          ...validValues,
          ttl: 8,
        },
        {
          ttl: 28_800_000_000_000,
        },
      ],
    ])(`formValuesToTTLRequest(%p) returns %p`, (values, request) => {
      expect(formValuesToTTLRequest(values)).toEqual(request)
    })
  })

  describe("workspaceToInitialValues", () => {
    it.each<[TypesGen.Workspace, WorkspaceScheduleFormValues]>([
      // Empty case
      [
        {
          ...Mocks.MockWorkspace,
          autostart_schedule: "",
          ttl: undefined,
        },
        {
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
        },
      ],

      // ttl-only case (2 hours)
      [
        {
          ...Mocks.MockWorkspace,
          autostart_schedule: "",
          ttl: 7_200_000_000_000,
        },
        {
          sunday: false,
          monday: false,
          tuesday: false,
          wednesday: false,
          thursday: false,
          friday: false,
          saturday: false,
          startTime: "",
          timezone: "",
          ttl: 2,
        },
      ],

      // Basic case: 9:30 1-5 UTC running for 2 hours
      //
      // NOTE: We have to set CRON_TZ here because otherwise this test will
      //       flake based off of where it runs!
      [
        {
          ...Mocks.MockWorkspace,
          autostart_schedule: "CRON_TZ=UTC 30 9 * * 1-5",
          ttl: 7_200_000_000_000,
        },
        {
          sunday: false,
          monday: true,
          tuesday: true,
          wednesday: true,
          thursday: true,
          friday: true,
          saturday: false,
          startTime: "09:30",
          timezone: "UTC",
          ttl: 2,
        },
      ],

      // Complex case: 4:20 1 3-4 6 Canada/Eastern for 8 hours
      [
        {
          ...Mocks.MockWorkspace,
          autostart_schedule: "CRON_TZ=Canada/Eastern 20 16 * * 1,3-4,6",
          ttl: 28_800_000_000_000,
        },
        {
          sunday: false,
          monday: true,
          tuesday: false,
          wednesday: true,
          thursday: true,
          friday: false,
          saturday: true,
          startTime: "16:20",
          timezone: "Canada/Eastern",
          ttl: 8,
        },
      ],
    ])(`workspaceToInitialValues(%p) returns %p`, (workspace, formValues) => {
      expect(workspaceToInitialValues(workspace)).toEqual(formValues)
    })
  })
})
