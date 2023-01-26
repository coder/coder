import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter"
import { MockTemplate } from "../../testHelpers/entities"
import { renderWithAuth } from "../../testHelpers/renderHelpers"
import { getValidationSchema } from "./TemplateSettingsForm"
import { TemplateSettingsPage } from "./TemplateSettingsPage"
import i18next from "i18next"

const { t } = i18next

const renderTemplateSettingsPage = async () => {
  const renderResult = renderWithAuth(<TemplateSettingsPage />, {
    route: `/templates/${MockTemplate.name}/settings`,
    path: `/templates/:templateId/settings`,
  })
  // Wait the form to be rendered
  const label = t("nameLabel", { ns: "templateSettingsPage" })
  await screen.findAllByLabelText(label)
  return renderResult
}

const validFormValues = {
  name: "Name",
  display_name: "A display name",
  description: "A description",
  icon: "vscode.png",
  default_ttl_ms: 1,
  allow_user_cancel_workspace_jobs: false,
}

const fillAndSubmitForm = async ({
  name,
  display_name,
  description,
  default_ttl_ms,
  icon,
  allow_user_cancel_workspace_jobs,
}: Required<UpdateTemplateMeta>) => {
  const label = t("nameLabel", { ns: "templateSettingsPage" })
  const nameField = await screen.findByLabelText(label)
  await userEvent.clear(nameField)
  await userEvent.type(nameField, name)

  const displayNameLabel = t("displayNameLabel", { ns: "templateSettingsPage" })

  const displayNameField = await screen.findByLabelText(displayNameLabel)
  await userEvent.clear(displayNameField)
  await userEvent.type(displayNameField, display_name)

  const descriptionLabel = t("descriptionLabel", { ns: "templateSettingsPage" })
  const descriptionField = await screen.findByLabelText(descriptionLabel)
  await userEvent.clear(descriptionField)
  await userEvent.type(descriptionField, description)

  const iconLabel = t("iconLabel", { ns: "templateSettingsPage" })
  const iconField = await screen.findByLabelText(iconLabel)
  await userEvent.clear(iconField)
  await userEvent.type(iconField, icon)

  const defaultTtlLabel = t("defaultTtlLabel", { ns: "templateSettingsPage" })
  const maxTtlField = await screen.findByLabelText(defaultTtlLabel)
  await userEvent.clear(maxTtlField)
  await userEvent.type(maxTtlField, default_ttl_ms.toString())

  const allowCancelJobsField = screen.getByRole("checkbox")
  // checkbox is checked by default, so it must be clicked to get unchecked
  if (!allow_user_cancel_workspace_jobs) {
    await userEvent.click(allowCancelJobsField)
  }

  const submitButton = await screen.findByText(
    FooterFormLanguage.defaultSubmitLabel,
  )
  await userEvent.click(submitButton)
}

describe("TemplateSettingsPage", () => {
  it("renders", async () => {
    const { t } = i18next
    const pageTitle = t("title", {
      ns: "templateSettingsPage",
    })
    await renderTemplateSettingsPage()
    const element = await screen.findByText(pageTitle)
    expect(element).toBeDefined()
  })

  it("allows an admin to delete a template", async () => {
    const { t } = i18next
    await renderTemplateSettingsPage()
    const deleteCta = t("dangerZone.deleteCta", {
      ns: "templateSettingsPage",
    })
    const deleteButton = await screen.findByText(deleteCta)
    expect(deleteButton).toBeDefined()
  })

  it("succeeds", async () => {
    await renderTemplateSettingsPage()

    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    })
    await fillAndSubmitForm(validFormValues)

    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))
  })

  test("ttl is converted to and from hours", async () => {
    await renderTemplateSettingsPage()

    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    })

    await fillAndSubmitForm(validFormValues)
    expect(screen.getByDisplayValue(1)).toBeInTheDocument() // the default_ttl_ms
    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))

    await waitFor(() =>
      expect(API.updateTemplateMeta).toBeCalledWith(
        "test-template",
        expect.objectContaining({
          ...validFormValues,
          default_ttl_ms: 3600000, // the default_ttl_ms to ms
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
      t("ttlMaxError", { ns: "templateSettingsPage" }),
    )
  })

  it("allows a description of 128 chars", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      description:
        "Nam quis nulla. Integer malesuada. In in enim a arcu imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus molestie, port",
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a description of 128 + 1 chars", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      description:
        "Nam quis nulla. Integer malesuada. In in enim a arcu imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus molestie, port a",
    }
    const validate = () => getValidationSchema().validateSync(values)
    expect(validate).toThrowError(
      t("descriptionMaxError", { ns: "templateSettingsPage" }),
    )
  })
})
