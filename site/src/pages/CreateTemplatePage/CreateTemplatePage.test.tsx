import {
  MockOrganization,
  MockProvisionerJob,
  MockTemplate,
  MockTemplateExample,
  MockTemplateVersion,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersionVariable3,
  renderWithAuth,
} from "testHelpers/renderHelpers"
import CreateTemplatePage from "./CreateTemplatePage"
import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"

const renderPage = async () => {
  // Render with the example ID so we don't need to upload a file
  const result = renderWithAuth(<CreateTemplatePage />, {
    route: `/templates/new?exampleId=${MockTemplateExample.id}`,
    path: "/templates/new",
    // We need this because after creation, the user will be redirected to here
    extraRoutes: [{ path: "templates/:template", element: <></> }],
  })
  // It is lazy loaded, so we have to wait for it to be rendered to not get an
  // act error
  await screen.findByLabelText("Icon", undefined, { timeout: 5000 })
  return result
}

test("Create template with variables", async () => {
  // Return pending when creating the first template version
  jest.spyOn(API, "createTemplateVersion").mockResolvedValueOnce({
    ...MockTemplateVersion,
    job: {
      ...MockTemplateVersion.job,
      status: "pending",
    },
  })
  // Return an error requesting for template variables
  jest.spyOn(API, "getTemplateVersion").mockResolvedValue({
    ...MockTemplateVersion,
    job: {
      ...MockTemplateVersion.job,
      status: "failed",
      error_code: "REQUIRED_TEMPLATE_VARIABLES",
    },
  })
  // Return the template variables
  jest
    .spyOn(API, "getTemplateVersionVariables")
    .mockResolvedValue([
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
      MockTemplateVersionVariable3,
    ])

  // Render page, fill the name and submit
  const { router, container } = await renderPage()
  const form = container.querySelector("form") as HTMLFormElement
  await userEvent.type(screen.getByLabelText(/Name/), "my-template")
  await userEvent.click(
    within(form).getByRole("button", { name: /create template/i }),
  )

  // Wait for the variables form to be rendered and fill it
  await screen.findByText(/Variables/)

  // Type first variable
  await userEvent.clear(screen.getByLabelText(/var.first_variable/))
  await userEvent.type(
    screen.getByLabelText(/var.first_variable/),
    "First value",
  )
  // Type second variable
  await userEvent.clear(screen.getByLabelText(/var.second_variable/))
  await userEvent.type(screen.getByLabelText(/var.second_variable/), "2")
  // Select third variable on radio
  await userEvent.click(screen.getByLabelText(/True/))
  // Setup the mock for the second template version creation before submit the form
  jest.clearAllMocks()
  jest
    .spyOn(API, "createTemplateVersion")
    .mockResolvedValue(MockTemplateVersion)
  jest.spyOn(API, "createTemplate").mockResolvedValue(MockTemplate)
  await userEvent.click(
    within(form).getByRole("button", { name: /create template/i }),
  )
  await waitFor(() => expect(API.createTemplate).toBeCalledTimes(1))
  expect(router.state.location.pathname).toEqual(
    `/templates/${MockTemplate.name}`,
  )
  expect(API.createTemplateVersion).toHaveBeenCalledWith(MockOrganization.id, {
    file_id: MockProvisionerJob.file_id,
    parameter_values: [],
    provisioner: "terraform",
    storage_method: "file",
    tags: {},
    user_variable_values: [
      { name: "first_variable", value: "First value" },
      { name: "second_variable", value: "2" },
      { name: "third_variable", value: "true" },
    ],
  })
})
