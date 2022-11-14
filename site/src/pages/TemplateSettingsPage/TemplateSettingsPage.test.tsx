import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter"
import { MockTemplate } from "../../testHelpers/entities"
import { renderWithAuth } from "../../testHelpers/renderHelpers"
import {
  Language as FormLanguage,
  validationSchema,
} from "./TemplateSettingsForm"
import { TemplateSettingsPage } from "./TemplateSettingsPage"
import i18next from "i18next"

const renderTemplateSettingsPage = async () => {
  const renderResult = renderWithAuth(<TemplateSettingsPage />, {
    route: `/templates/${MockTemplate.name}/settings`,
    path: `/templates/:templateId/settings`,
  })
  // Wait the form to be rendered
  await screen.findAllByLabelText(FormLanguage.nameLabel)
  return renderResult
}

const validFormValues = {
  name: "Name",
  display_name: "Test Template",
  description: "A description",
  icon: "A string",
  default_ttl_ms: 1,
}

const fillAndSubmitForm = async ({
  name,
  description,
  default_ttl_ms,
  icon,
}: Required<UpdateTemplateMeta>) => {
  const nameField = await screen.findByLabelText(FormLanguage.nameLabel)
  await userEvent.clear(nameField)
  await userEvent.type(nameField, name)

  const descriptionField = await screen.findByLabelText(
    FormLanguage.descriptionLabel,
  )
  await userEvent.clear(descriptionField)
  await userEvent.type(descriptionField, description)

  const iconField = await screen.findByLabelText(FormLanguage.iconLabel)
  await userEvent.clear(iconField)
  await userEvent.type(iconField, icon)

  const maxTtlField = await screen.findByLabelText(FormLanguage.defaultTtlLabel)
  await userEvent.clear(maxTtlField)
  await userEvent.type(maxTtlField, default_ttl_ms.toString())

  const submitButton = await screen.findByText(
    FooterFormLanguage.defaultSubmitLabel,
  )
  await userEvent.click(submitButton)
}

describe("TemplateSettingsPage", () => {
  it("renders", async () => {
    const { t } = i18next
    const pageTitle = t("templateSettings.title", {
      ns: "templatePage",
    })
    await renderTemplateSettingsPage()
    const element = await screen.findByText(pageTitle)
    expect(element).toBeDefined()
  })

  it("allows an admin to delete a template", async () => {
    const { t } = i18next
    await renderTemplateSettingsPage()
    const deleteCta = t("templateSettings.dangerZone.deleteCta", {
      ns: "templatePage",
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
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("allows ttl of 0", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      default_ttl_ms: 0,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a ttl of 7 days + 1 hour", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      default_ttl_ms: 24 * 7 + 1,
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(FormLanguage.ttlMaxError)
  })

  it("allows a description of 128 chars", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      description:
        "Nam quis nulla. Integer malesuada. In in enim a arcu imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus molestie, port",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).not.toThrowError()
  })

  it("disallows a description of 128 + 1 chars", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      description:
        "Nam quis nulla. Integer malesuada. In in enim a arcu imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus molestie, port a",
    }
    const validate = () => validationSchema.validateSync(values)
    expect(validate).toThrowError(FormLanguage.descriptionMaxError)
  })
})
