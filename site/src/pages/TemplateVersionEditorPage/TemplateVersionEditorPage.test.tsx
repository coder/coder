import { renderWithAuth } from "testHelpers/renderHelpers";
import TemplateVersionEditorPage from "./TemplateVersionEditorPage";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "api/api";
import {
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

test("Use custom name, message and set it as active when publishing", async () => {
  const user = userEvent.setup();
  renderWithAuth(<TemplateVersionEditorPage />, {
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div />,
      },
    ],
  });
  const topbar = await screen.findByTestId("topbar");

  // Build Template
  jest.spyOn(api, "uploadTemplateFile").mockResolvedValueOnce({ hash: "hash" });
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValueOnce(MockTemplateVersion);
  jest
    .spyOn(api, "getTemplateVersion")
    .mockResolvedValue({ ...MockTemplateVersion, id: "new-version-id" });
  jest
    .spyOn(api, "watchBuildLogsByTemplateVersionId")
    .mockImplementation((_, options) => {
      options.onMessage(MockWorkspaceBuildLogs[0]);
      options.onDone();
      return jest.fn() as never;
    });
  const buildButton = within(topbar).getByRole("button", {
    name: "Build template",
  });
  await user.click(buildButton);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(MockTemplateVersion);
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" });
  await within(topbar).findByText("Success");
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish version",
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
  renderWithAuth(<TemplateVersionEditorPage />, {
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div />,
      },
    ],
  });
  const topbar = await screen.findByTestId("topbar");

  // Build Template
  jest.spyOn(api, "uploadTemplateFile").mockResolvedValueOnce({ hash: "hash" });
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValueOnce(MockTemplateVersion);
  jest
    .spyOn(api, "getTemplateVersion")
    .mockResolvedValue({ ...MockTemplateVersion, id: "new-version-id" });
  jest
    .spyOn(api, "watchBuildLogsByTemplateVersionId")
    .mockImplementation((_, options) => {
      options.onMessage(MockWorkspaceBuildLogs[0]);
      options.onDone();
      return jest.fn() as never;
    });
  const buildButton = within(topbar).getByRole("button", {
    name: "Build template",
  });
  await user.click(buildButton);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(MockTemplateVersion);
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" });
  await within(topbar).findByText("Success");
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish version",
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
  const MockTemplateVersionWithEmptyMessage = {
    ...MockTemplateVersion,
    message: "",
  };
  const user = userEvent.setup();
  renderWithAuth(<TemplateVersionEditorPage />, {
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div />,
      },
    ],
  });
  const topbar = await screen.findByTestId("topbar");

  // Build Template
  jest.spyOn(api, "uploadTemplateFile").mockResolvedValueOnce({ hash: "hash" });
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValueOnce(MockTemplateVersionWithEmptyMessage);
  jest.spyOn(api, "getTemplateVersion").mockResolvedValue({
    ...MockTemplateVersionWithEmptyMessage,
    id: "new-version-id",
  });
  jest
    .spyOn(api, "watchBuildLogsByTemplateVersionId")
    .mockImplementation((_, options) => {
      options.onMessage(MockWorkspaceBuildLogs[0]);
      options.onDone();
      return jest.fn() as never;
    });
  const buildButton = within(topbar).getByRole("button", {
    name: "Build template",
  });
  await user.click(buildButton);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(MockTemplateVersionWithEmptyMessage);
  await within(topbar).findByText("Success");
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish version",
  });
  await user.click(publishButton);
  const publishDialog = await screen.findByTestId("dialog");
  // It is using the name from the template version
  const nameField = within(publishDialog).getByLabelText("Version name");
  expect(nameField).toHaveValue(MockTemplateVersionWithEmptyMessage.name);
  // Publish
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  );
  expect(patchTemplateVersion).toBeCalledTimes(0);
});
