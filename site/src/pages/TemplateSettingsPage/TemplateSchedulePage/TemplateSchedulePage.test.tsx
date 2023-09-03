import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter"
import {
  MockEntitlementsWithScheduling,
  MockTemplate,
} from "testHelpers/entities"
import {
  renderWithTemplateSettingsLayout,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers"
import {
  TemplateScheduleFormValues,
  getValidationSchema,
} from "./TemplateScheduleForm/formHelpers"
import TemplateSchedulePage from "./TemplateSchedulePage"
import i18next from "i18next"

const { t } = i18next

const validFormValues: TemplateScheduleFormValues = {
  default_ttl_ms: 1,
  max_ttl_ms: 2,
  failure_ttl_ms: 7,
  time_til_dormant_ms: 180,
  time_til_dormant_autodelete_ms: 30,
  update_workspace_last_used_at: false,
  update_workspace_dormant_at: false,
  autostop_requirement_days_of_week: "off",
  autostop_requirement_weeks: 1,
  failure_cleanup_enabled: false,
  inactivity_cleanup_enabled: false,
  dormant_autodeletion_cleanup_enabled: false,
}

const renderTemplateSchedulePage = async () => {
  renderWithTemplateSettingsLayout(<TemplateSchedulePage />, {
    route: `/templates/${MockTemplate.name}/settings/schedule`,
    path: `/templates/:template/settings/schedule`,
  })
  await waitForLoaderToBeRemoved()
}

const fillAndSubmitForm = async ({
  default_ttl_ms,
  max_ttl_ms,
  failure_ttl_ms,
  time_til_dormant_ms,
  time_til_dormant_autodelete_ms,
}: {
  default_ttl_ms?: number
  max_ttl_ms?: number
  failure_ttl_ms?: number
  time_til_dormant_ms?: number
  time_til_dormant_autodelete_ms?: number
}) => {
  const user = userEvent.setup()

  if (default_ttl_ms) {
    const defaultTtlLabel = t("defaultTtlLabel", { ns: "templateSettingsPage" })
    const defaultTtlField = await screen.findByLabelText(defaultTtlLabel)
    await user.clear(defaultTtlField)
    await user.type(defaultTtlField, default_ttl_ms.toString())
  }

  if (max_ttl_ms) {
    const maxTtlLabel = t("maxTtlLabel", { ns: "templateSettingsPage" })
    const maxTtlField = await screen.findByLabelText(maxTtlLabel)
    await user.clear(maxTtlField)
    await user.type(maxTtlField, max_ttl_ms.toString())
  }

  if (failure_ttl_ms) {
    const failureTtlField = screen.getByRole("checkbox", {
      name: /Failure Cleanup/i,
    })
    await user.type(failureTtlField, failure_ttl_ms.toString())
  }

  if (time_til_dormant_ms) {
    const inactivityTtlField = screen.getByRole("checkbox", {
      name: /Dormancy Threshold/i,
    })
    await user.type(inactivityTtlField, time_til_dormant_ms.toString())
  }

  if (time_til_dormant_autodelete_ms) {
    const dormancyAutoDeletionField = screen.getByRole("checkbox", {
      name: /Dormancy Auto-Deletion/i,
    })
    await user.type(
      dormancyAutoDeletionField,
      time_til_dormant_autodelete_ms.toString(),
    )
  }

  const submitButton = await screen.findByText(
    FooterFormLanguage.defaultSubmitLabel,
  )
  await user.click(submitButton)

  // User needs to confirm dormancy and autodeletion fields.
  const confirmButton = await screen.findByTestId("confirm-button")
  await user.click(confirmButton)
}

describe("TemplateSchedulePage", () => {
  beforeEach(() => {
    jest
      .spyOn(API, "getEntitlements")
      .mockResolvedValue(MockEntitlementsWithScheduling)

    // remove when https://github.com/coder/coder/milestone/19 is completed.
    jest.spyOn(API, "getExperiments").mockResolvedValue(["workspace_actions"])
  })

  it("succeeds", async () => {
    await renderTemplateSchedulePage()
    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    })
    await fillAndSubmitForm(validFormValues)
    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))
  })

  test("default and max ttl is converted to and from hours", async () => {
    await renderTemplateSchedulePage()

    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    })

    await fillAndSubmitForm(validFormValues)
    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))
    await waitFor(() =>
      expect(API.updateTemplateMeta).toBeCalledWith(
        "test-template",
        expect.objectContaining({
          default_ttl_ms: (validFormValues.default_ttl_ms || 0) * 3600000,
          max_ttl_ms: (validFormValues.max_ttl_ms || 0) * 3600000,
        }),
      ),
    )
  })

  test("failure, dormancy, and dormancy auto-deletion converted to and from days", async () => {
    await renderTemplateSchedulePage()

    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    })

    await fillAndSubmitForm(validFormValues)
    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))
    await waitFor(() =>
      expect(API.updateTemplateMeta).toBeCalledWith(
        "test-template",
        expect.objectContaining({
          failure_ttl_ms: (validFormValues.failure_ttl_ms || 0) * 86400000,
          time_til_dormant_ms:
            (validFormValues.time_til_dormant_ms || 0) * 86400000,
          time_til_dormant_autodelete_ms:
            (validFormValues.time_til_dormant_autodelete_ms || 0) * 86400000,
        }),
      ),
    )
  })

  it("allows a default ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 24 * 7,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows default ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 0,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows a default ttl of 30 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 24 * 30,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a default ttl of 30 days + 1 hour", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 24 * 30 + 1,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError(
      t("defaultTTLMaxError", { ns: "templateSettingsPage" }).toString(),
    )
  })

  it("allows a failure ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      failure_ttl_ms: 86400000 * 7,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows failure ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      failure_ttl_ms: 0,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a negative failure ttl", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      failure_ttl_ms: -1,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError(
      "Failure cleanup days must not be less than 0.",
    )
  })

  it("allows an inactivity ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_ms: 86400000 * 7,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows an inactivity ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_ms: 0,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a negative inactivity ttl", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_ms: -1,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError(
      "Dormancy threshold days must not be less than 0.",
    )
  })

  it("allows a dormancy ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_autodelete_ms: 86400000 * 7,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows a dormancy ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_autodelete_ms: 0,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a negative inactivity ttl", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_autodelete_ms: -1,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError(
      "Dormancy auto-deletion days must not be less than 0.",
    )
  })

  it("allows an autostop requirement weeks of 1", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_days_of_week: "saturday",
      autostop_requirement_weeks: 1,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows a autostop requirement weeks of 16", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_weeks: 16,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a autostop requirement weeks of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_weeks: 0,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError()
  })

  it("disallows a autostop requirement weeks of 17", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_weeks: 17,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError()
  })
})
