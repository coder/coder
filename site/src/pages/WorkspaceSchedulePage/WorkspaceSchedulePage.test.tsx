import {
  MockUser,
  MockWorkspace,
  renderWithAuth,
} from "testHelpers/renderHelpers"
import userEvent from "@testing-library/user-event"
import { screen } from "@testing-library/react"
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
import {
  WorkspaceScheduleFormValues,
  Language as FormLanguage,
} from "components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { WorkspaceSchedulePage } from "./WorkspaceSchedulePage"
import i18next from "i18next"
import { server } from "testHelpers/server"
import { rest } from "msw"

const { t } = i18next

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

  describe("autoStop change dialog", () => {
    it("shows if autoStop is changed", async () => {
      renderWithAuth(<WorkspaceSchedulePage />, {
        route: `/@${MockUser.username}/${MockWorkspace.name}/schedule`,
        path: "/@:username/:workspace/schedule",
      })
      const user = userEvent.setup()
      const autoStopToggle = await screen.findByLabelText(
        FormLanguage.stopSwitch,
      )
      await user.click(autoStopToggle)
      const submitButton = await screen.findByRole("button", {
        name: /submit/i,
      })
      await user.click(submitButton)
      const title = t("dialogTitle", { ns: "workspaceSchedulePage" })
      const dialog = await screen.findByText(title)
      expect(dialog).toBeInTheDocument()
    })

    it("doesn't show if autoStop is not changed", async () => {
      renderWithAuth(<WorkspaceSchedulePage />, {
        route: `/@${MockUser.username}/${MockWorkspace.name}/schedule`,
        path: "/@:username/:workspace/schedule",
      })
      const user = userEvent.setup()
      const autoStartToggle = await screen.findByLabelText(
        FormLanguage.startSwitch,
      )
      await user.click(autoStartToggle)
      const submitButton = await screen.findByRole("button", {
        name: /submit/i,
      })
      await user.click(submitButton)
      const title = t("dialogTitle", { ns: "workspaceSchedulePage" })
      const dialog = screen.queryByText(title)
      expect(dialog).not.toBeInTheDocument()
    })
  })

  describe("autostop", () => {
    it("uses template default ttl when first enabled", async () => {
      // have auto-stop disabled
      server.use(
        rest.get(
          "/api/v2/users/:userId/workspace/:workspaceName",
          (req, res, ctx) => {
            return res(
              ctx.status(200),
              ctx.json({ ...MockWorkspace, ttl_ms: 0 }),
            )
          },
        ),
      )
      renderWithAuth(<WorkspaceSchedulePage />, {
        route: `/@${MockUser.username}/${MockWorkspace.name}/schedule`,
        path: "/@:username/:workspace/schedule",
      })
      const user = userEvent.setup()
      const autoStopToggle = await screen.findByLabelText(
        FormLanguage.stopSwitch,
      )
      // enable auto-stop
      await user.click(autoStopToggle)
      // find helper text that describes the mock template's 24 hour default
      const autoStopHelperText = await screen.findByText(
        "Your workspace will shut down a day after",
        { exact: false },
      )
      expect(autoStopHelperText).toBeDefined()
    })
  })
})
