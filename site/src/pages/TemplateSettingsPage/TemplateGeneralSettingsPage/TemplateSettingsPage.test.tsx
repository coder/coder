import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter"
import { MockTemplate } from "../../../testHelpers/entities"
import {
  renderWithTemplateSettingsLayout,
  waitForLoaderToBeRemoved,
} from "../../../testHelpers/renderHelpers"
import { getValidationSchema } from "./TemplateSettingsForm"
import { TemplateSettingsPage } from "./TemplateSettingsPage"
import i18next from "i18next"

const { t } = i18next

type FormValues = Required<
  Omit<UpdateTemplateMeta, "default_ttl_ms" | "max_ttl_ms">
>

const validFormValues: FormValues = {
  name: "Name",
  display_name: "A display name",
  description: "A description",
  icon: "vscode.png",
  allow_user_cancel_workspace_jobs: false,
  allow_user_autostart: false,
  allow_user_autostop: false,
  restart_requirement: {
    days_of_week: [],
    weeks: 1,
  },
  failure_ttl_ms: 0,
  inactivity_ttl_ms: 0,
  locked_ttl_ms: 0,
  update_workspace_last_used_at: false,
  update_workspace_locked_at: false,
}

const renderTemplateSettingsPage = async () => {
  renderWithTemplateSettingsLayout(<TemplateSettingsPage />, {
    route: `/templates/${MockTemplate.name}/settings`,
    path: `/templates/:template/settings`,
  })
  await waitForLoaderToBeRemoved()
}

const fillAndSubmitForm = async ({
  name,
  display_name,
  description,
  icon,
  allow_user_cancel_workspace_jobs,
}: FormValues) => {
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
  it("succeeds", async () => {
    await renderTemplateSettingsPage()
    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    })
    await fillAndSubmitForm(validFormValues)
    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1))
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
      t("descriptionMaxError", { ns: "templateSettingsPage" }).toString(),
    )
  })
})
