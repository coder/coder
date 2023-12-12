import { renderWithAuth } from "testHelpers/renderHelpers";
import TemplateVersionEditorPage from "./TemplateVersionEditorPage";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "api/api";
import {
  MockTemplate,
  MockTemplateVersion,
  MockWorkspaceBuildLogs,
} from "testHelpers/entities";
import { Language } from "./PublishTemplateVersionDialog";

// For some reason this component in Jest is throwing a MUI style warning so,
// since we don't need it for this test, we can mock it out
jest.mock("components/TemplateResourcesTable/TemplateResourcesTable", () => {
  return {
    TemplateResourcesTable: () => <div />,
  };
});

const renderTemplateEditorPage = () => {
  renderWithAuth(<TemplateVersionEditorPage />, {
    route: `/templates/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit`,
    path: "/templates/:template/versions/:version/edit",
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div />,
      },
    ],
  });
};

test("Use custom name, message and set it as active when publishing", async () => {
  const user = userEvent.setup();
  renderTemplateEditorPage();
  const topbar = await screen.findByTestId("topbar");

  // Build Template
  jest.spyOn(api, "uploadFile").mockResolvedValueOnce({ hash: "hash" });
  const newTemplateVersion = {
    ...MockTemplateVersion,
    id: "new-version-id",
    name: "new-version",
  };
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValue(newTemplateVersion);
  jest
    .spyOn(api, "getTemplateVersionByName")
    .mockResolvedValue(newTemplateVersion);
  jest
    .spyOn(api, "watchBuildLogsByTemplateVersionId")
    .mockImplementation((_, options) => {
      options.onMessage(MockWorkspaceBuildLogs[0]);
      options.onDone();
      return jest.fn() as never;
    });
  const buildButton = within(topbar).getByRole("button", {
    name: "Build",
  });
  await user.click(buildButton);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(newTemplateVersion);
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" });
  await within(topbar).findByText("Success");
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish",
  });
  await user.click(publishButton);
  const publishDialog = await screen.findByTestId("dialog");
  const nameField = within(publishDialog).getByLabelText("Version name");
  await user.clear(nameField);
  await user.type(nameField, "v1.0");
  const messageField = within(publishDialog).getByLabelText("Message");
  await user.clear(messageField);
  await user.type(messageField, "Informative message");
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  );
  await waitFor(() => {
    expect(patchTemplateVersion).toBeCalledWith("new-version-id", {
      name: "v1.0",
      message: "Informative message",
    });
  });
  expect(updateActiveTemplateVersion).toBeCalledWith("test-template", {
    id: "new-version-id",
  });
});

test("Do not mark as active if promote is not checked", async () => {
  const user = userEvent.setup();
  renderTemplateEditorPage();
  const topbar = await screen.findByTestId("topbar");

  // Build Template
  jest.spyOn(api, "uploadFile").mockResolvedValueOnce({ hash: "hash" });
  const newTemplateVersion = {
    ...MockTemplateVersion,
    id: "new-version-id",
    name: "new-version",
  };
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValue(newTemplateVersion);
  jest
    .spyOn(api, "getTemplateVersionByName")
    .mockResolvedValue(newTemplateVersion);
  jest
    .spyOn(api, "watchBuildLogsByTemplateVersionId")
    .mockImplementation((_, options) => {
      options.onMessage(MockWorkspaceBuildLogs[0]);
      options.onDone();
      return jest.fn() as never;
    });
  const buildButton = within(topbar).getByRole("button", {
    name: "Build",
  });
  await user.click(buildButton);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(newTemplateVersion);
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" });
  await within(topbar).findByText("Success");
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish",
  });
  await user.click(publishButton);
  const publishDialog = await screen.findByTestId("dialog");
  const nameField = within(publishDialog).getByLabelText("Version name");
  await user.clear(nameField);
  await user.type(nameField, "v1.0");
  await user.click(
    within(publishDialog).getByLabelText(Language.defaultCheckboxLabel),
  );
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  );
  await waitFor(() => {
    expect(patchTemplateVersion).toBeCalledWith("new-version-id", {
      name: "v1.0",
      message: "",
    });
  });
  expect(updateActiveTemplateVersion).toBeCalledTimes(0);
});

test("Patch request is not send when there are no changes", async () => {
  const newTemplateVersion = {
    ...MockTemplateVersion,
    id: "new-version-id",
    name: "new-version",
  };
  const MockTemplateVersionWithEmptyMessage = {
    ...newTemplateVersion,
    message: "",
  };
  const user = userEvent.setup();
  renderTemplateEditorPage();
  const topbar = await screen.findByTestId("topbar");

  // Build Template
  jest.spyOn(api, "uploadFile").mockResolvedValueOnce({ hash: "hash" });
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValue(MockTemplateVersionWithEmptyMessage);
  jest
    .spyOn(api, "getTemplateVersionByName")
    .mockResolvedValue(MockTemplateVersionWithEmptyMessage);
  jest
    .spyOn(api, "watchBuildLogsByTemplateVersionId")
    .mockImplementation((_, options) => {
      options.onMessage(MockWorkspaceBuildLogs[0]);
      options.onDone();
      return jest.fn() as never;
    });
  const buildButton = within(topbar).getByRole("button", {
    name: "Build",
  });
  await user.click(buildButton);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(MockTemplateVersionWithEmptyMessage);
  await within(topbar).findByText("Success");
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish",
  });
  await user.click(publishButton);
  const publishDialog = await screen.findByTestId("dialog");
  // It is using the name from the template
  const nameField = within(publishDialog).getByLabelText("Version name");
  expect(nameField).toHaveValue(MockTemplateVersionWithEmptyMessage.name);
  // Publish
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  );
  expect(patchTemplateVersion).toBeCalledTimes(0);
});
