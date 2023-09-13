import { renderWithAuth } from "testHelpers/renderHelpers";
import CreateTemplatePage from "./CreateTemplatePage";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as API from "api/api";
import {
  MockTemplateExample,
  MockTemplateVersion,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersionVariable3,
  MockTemplate,
  MockOrganization,
  MockProvisionerJob,
} from "testHelpers/entities";

const renderPage = async (searchParams: URLSearchParams) => {
  // Render with the example ID so we don't need to upload a file
  const view = renderWithAuth(<CreateTemplatePage />, {
    route: `/templates/new?${searchParams.toString()}`,
    path: "/templates/new",
    // We need this because after creation, the user will be redirected to here
    extraRoutes: [{ path: "templates/:template", element: <></> }],
  });
  // It is lazy loaded, so we have to wait for it to be rendered to not get an
  // act error
  await screen.findByLabelText("Icon", undefined, { timeout: 5000 });
  return view;
};

test("Create template with variables", async () => {
  // Return pending when creating the first template version
  jest.spyOn(API, "createTemplateVersion").mockResolvedValueOnce({
    ...MockTemplateVersion,
    job: {
      ...MockTemplateVersion.job,
      status: "pending",
    },
  });
  // Return an error requesting for template variables
  jest.spyOn(API, "getTemplateVersion").mockResolvedValue({
    ...MockTemplateVersion,
    job: {
      ...MockTemplateVersion.job,
      status: "failed",
      error_code: "REQUIRED_TEMPLATE_VARIABLES",
    },
  });
  // Return the template variables
  jest
    .spyOn(API, "getTemplateVersionVariables")
    .mockResolvedValue([
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
      MockTemplateVersionVariable3,
    ]);

  // Render page, fill the name and submit
  const searchParams = new URLSearchParams({
    exampleId: MockTemplateExample.id,
  });
  const { router, container } = await renderPage(searchParams);
  const form = container.querySelector("form") as HTMLFormElement;
  await userEvent.type(screen.getByLabelText(/Name/), "my-template");
  await userEvent.click(
    within(form).getByRole("button", { name: /create template/i }),
  );

  // Wait for the variables form to be rendered and fill it
  await screen.findByText(/Variables/);

  // Type first variable
  await userEvent.clear(screen.getByLabelText(/var.first_variable/));
  await userEvent.type(
    screen.getByLabelText(/var.first_variable/),
    "First value",
  );
  // Type second variable
  await userEvent.clear(screen.getByLabelText(/var.second_variable/));
  await userEvent.type(screen.getByLabelText(/var.second_variable/), "2");
  // Select third variable on radio
  await userEvent.click(screen.getByLabelText(/True/));
  // Setup the mock for the second template version creation before submit the form
  jest.clearAllMocks();
  jest
    .spyOn(API, "createTemplateVersion")
    .mockResolvedValue(MockTemplateVersion);
  jest.spyOn(API, "createTemplate").mockResolvedValue(MockTemplate);
  await userEvent.click(
    within(form).getByRole("button", { name: /create template/i }),
  );
  await waitFor(() => expect(API.createTemplate).toBeCalledTimes(1));
  expect(router.state.location.pathname).toEqual(
    `/templates/${MockTemplate.name}`,
  );
  expect(API.createTemplateVersion).toHaveBeenCalledWith(MockOrganization.id, {
    file_id: MockProvisionerJob.file_id,
    provisioner: "terraform",
    storage_method: "file",
    tags: {},
    user_variable_values: [
      { name: "first_variable", value: "First value" },
      { name: "second_variable", value: "2" },
      { name: "third_variable", value: "true" },
    ],
  });
});

test("Create template from another template", async () => {
  const searchParams = new URLSearchParams({
    fromTemplate: MockTemplate.name,
  });
  const { router } = await renderPage(searchParams);
  // Name and display name are using copy prefixes
  expect(screen.getByLabelText(/Name/)).toHaveValue(
    `${MockTemplate.name}-copy`,
  );
  expect(screen.getByLabelText(/Display name/)).toHaveValue(
    `Copy of ${MockTemplate.display_name}`,
  );
  // Variables are using the same values
  expect(
    screen.getByLabelText(MockTemplateVersionVariable1.description, {
      exact: false,
    }),
  ).toHaveValue(MockTemplateVersionVariable1.value);
  // Create template
  jest
    .spyOn(API, "createTemplateVersion")
    .mockResolvedValue(MockTemplateVersion);
  jest.spyOn(API, "createTemplate").mockResolvedValue(MockTemplate);
  await userEvent.click(
    screen.getByRole("button", { name: /create template/i }),
  );
  expect(router.state.location.pathname).toEqual(
    `/templates/${MockTemplate.name}`,
  );
});
