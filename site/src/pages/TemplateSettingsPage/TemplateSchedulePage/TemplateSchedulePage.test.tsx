import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter"
import {
  MockEntitlementsWithScheduling,
  MockTemplate,
} from "../../../testHelpers/entities"
import {
  renderWithTemplateSettingsLayout,
  waitForLoaderToBeRemoved,
} from "../../../testHelpers/renderHelpers"
import { getValidationSchema } from "./TemplateScheduleForm"
import TemplateSchedulePage from "./TemplateSchedulePage"
import i18next from "i18next"

const { t } = i18next

const validFormValues = {
  default_ttl_ms: 1,
  max_ttl_ms: 2,
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
}: {
  default_ttl_ms: number
  max_ttl_ms: number
}) => {
  const user = userEvent.setup()
  const defaultTtlLabel = t("defaultTtlLabel", { ns: "templateSettingsPage" })
  const defaultTtlField = await screen.findByLabelText(defaultTtlLabel)
  await user.clear(defaultTtlField)
  await user.type(defaultTtlField, default_ttl_ms.toString())

  const maxTtlLabel = t("maxTtlLabel", { ns: "templateSettingsPage" })
  const maxTtlField = await screen.findByLabelText(maxTtlLabel)
  await user.clear(maxTtlField)
  await user.type(maxTtlField, max_ttl_ms.toString())

  const submitButton = await screen.findByText(
    FooterFormLanguage.defaultSubmitLabel,
  )
  await user.click(submitButton)
}

describe("TemplateSchedulePage", () => {
  beforeEach(() => {
    jest
      .spyOn(API, "getEntitlements")
      .mockResolvedValue(MockEntitlementsWithScheduling)
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

  test("ttl is converted to and from hours", async () => {
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
          default_ttl_ms: validFormValues.default_ttl_ms * 3600000,
          max_ttl_ms: validFormValues.max_ttl_ms * 3600000,
        }),
      ),
    )
  })

  it("allows a ttl of 7 days", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      default_ttl_ms: 24 * 7,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows ttl of 0", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      default_ttl_ms: 0,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a ttl of 7 days + 1 hour", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      default_ttl_ms: 24 * 7 + 1,
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError(
      t("defaultTTLMaxError", { ns: "templateSettingsPage" }),
    )
  })
})
