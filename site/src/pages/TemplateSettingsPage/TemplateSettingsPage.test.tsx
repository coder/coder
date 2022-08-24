import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter"
import { MockTemplate } from "../../testHelpers/entities"
import { renderWithAuth } from "../../testHelpers/renderHelpers"
import { Language as FormLanguage, validationSchema } from "./TemplateSettingsForm"
import { TemplateSettingsPage } from "./TemplateSettingsPage"
import { Language as ViewLanguage } from "./TemplateSettingsPageView"

const renderTemplateSettingsPage = async () => {
  const renderResult = renderWithAuth(<TemplateSettingsPage />, {
    route: `/templates/${MockTemplate.name}/settings`,
    path: `/templates/:templateId/settings`,
  })
  // Wait the form to be rendered
  await screen.findAllByLabelText(FormLanguage.nameLabel)
  return renderResult
}

const validFormValues: UpdateTemplateMeta = {
  name: "A name",
  description: "A description",
  icon: "A string",
  max_ttl_ms: 24,
  min_autostart_interval_ms: 24,
}

const fillAndSubmitForm = async ({
  name,
  description,
  max_ttl_ms,
  icon,
}: Omit<Required<UpdateTemplateMeta>, "min_autostart_interval_ms">) => {
  const nameField = await screen.findByLabelText(FormLanguage.nameLabel)
  await userEvent.clear(nameField)
  await userEvent.type(nameField, name)

  const descriptionField = await screen.findByLabelText(FormLanguage.descriptionLabel)
  await userEvent.clear(descriptionField)
  await userEvent.type(descriptionField, description)

  const iconField = await screen.findByLabelText(FormLanguage.iconLabel)
  await userEvent.clear(iconField)
  await userEvent.type(iconField, icon)

  const maxTtlField = await screen.findByLabelText(FormLanguage.maxTtlLabel)
  await userEvent.clear(maxTtlField)
  await userEvent.type(maxTtlField, max_ttl_ms.toString())

  const submitButton = await screen.findByText(FooterFormLanguage.defaultSubmitLabel)
  await userEvent.click(submitButton)
}

describe("TemplateSettingsPage", () => {
  it("renders", async () => {
    await renderTemplateSettingsPage()
    const element = await screen.findByText(ViewLanguage.title)
    expect(element).toBeDefined()
  })

  it("succeeds", async () => {
    await renderTemplateSettingsPage()

    const newTemplateSettings = {
      name: "edited-template-name",
      description: "Edited description",
      max_ttl_ms: 4000,
      icon: "/icon/code.svg",
    }
    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...newTemplateSettings,
    })
    await fillAndSubmitForm(newTemplateSettings)

    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))
  })

  test("ttl is converted to and from hours", async () => {
    await renderTemplateSettingsPage()

    const newTemplateSettings = {
      name: "edited-template-name",
      description: "Edited description",
      max_ttl_ms: 1,
      icon: "/icon/code.svg",
    }

    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...newTemplateSettings,
    })

    await fillAndSubmitForm(newTemplateSettings)
    expect(screen.getByDisplayValue(1)).toBeInTheDocument()
    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))

    await waitFor(() =>
      expect(API.updateTemplateMeta).toBeCalledWith(
        "test-template",
        expect.objectContaining({
          ...newTemplateSettings,
          max_ttl_ms: 3600000,
        }),
      ),
    )
  })

  it("allows a ttl of 7 days", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      max_ttl_ms: 24 * 7,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a ttl of 7 days + 1 hour", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      max_ttl_ms: 24 * 7 + 1,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError("ttl must be less than or equal to 168")
  })
})
