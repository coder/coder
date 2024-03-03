import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import TemplateVersionEditorPage from "./TemplateVersionEditorPage";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent, { UserEvent } from "@testing-library/user-event";
import * as api from "api/api";
import {
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockWorkspaceBuildLogs,
} from "testHelpers/entities";
import { Language } from "./PublishTemplateVersionDialog";
import { QueryClient } from "react-query";
import { templateVersionVariablesKey } from "api/queries/templates";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { server } from "testHelpers/server";
import { rest } from "msw";
import { AppProviders } from "App";
import { TemplateVersion } from "api/typesGenerated";

// For some reason this component in Jest is throwing a MUI style warning so,
// since we don't need it for this test, we can mock it out
jest.mock(
  "modules/templates/TemplateResourcesTable/TemplateResourcesTable",
  () => ({
    TemplateResourcesTable: () => <div></div>,
  }),
);

// Occasionally, Jest encounters HTML5 canvas errors. As the MonacoEditor is not
// required for these tests, we can safely mock it.
jest.mock("pages/TemplateVersionEditorPage/MonacoEditor", () => ({
  MonacoEditor: () => <div />,
}));

const renderTemplateEditorPage = () => {
  renderWithAuth(<TemplateVersionEditorPage />, {
    route: `/templates/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit`,
    path: "/templates/:template/versions/:version/edit",
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div></div>,
      },
    ],
  });
};

const buildTemplateVersion = async (
  templateVersion: TemplateVersion,
  user: UserEvent,
  topbar: HTMLElement,
) => {
  jest.spyOn(api, "uploadFile").mockResolvedValueOnce({ hash: "hash" });
  jest.spyOn(api, "createTemplateVersion").mockResolvedValue({
    ...templateVersion,
    job: {
      ...templateVersion.job,
      status: "running",
    },
  });
  jest
    .spyOn(api, "getTemplateVersionByName")
    .mockResolvedValue(templateVersion);
  jest
    .spyOn(api, "watchBuildLogsByTemplateVersionId")
    .mockImplementation((_, options) => {
      options.onMessage(MockWorkspaceBuildLogs[0]);
      options.onDone?.();
      const wsMock = {
        close: jest.fn(),
      } as unknown;
      return wsMock as WebSocket;
    });
  const buildButton = within(topbar).getByRole("button", {
    name: "Build",
  });
  await user.click(buildButton);
  await within(topbar).findByText("Success");
};

test("Use custom name, message and set it as active when publishing", async () => {
  const user = userEvent.setup();
  renderTemplateEditorPage();
  const topbar = await screen.findByTestId("topbar");

  const newTemplateVersion: TemplateVersion = {
    ...MockTemplateVersion,
    id: "new-version-id",
    name: "new-version",
  };
  await buildTemplateVersion(newTemplateVersion, user, topbar);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(newTemplateVersion);
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" });
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

  const newTemplateVersion = {
    ...MockTemplateVersion,
    id: "new-version-id",
    name: "new-version",
  };
  await buildTemplateVersion(newTemplateVersion, user, topbar);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(newTemplateVersion);
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" });
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
  const user = userEvent.setup();
  renderTemplateEditorPage();
  const topbar = await screen.findByTestId("topbar");

  const newTemplateVersion = {
    ...MockTemplateVersion,
    id: "new-version-id",
    name: "new-version",
    message: "",
  };
  await buildTemplateVersion(newTemplateVersion, user, topbar);

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(newTemplateVersion);
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish",
  });
  await user.click(publishButton);
  const publishDialog = await screen.findByTestId("dialog");
  // It is using the name from the template
  const nameField = within(publishDialog).getByLabelText("Version name");
  expect(nameField).toHaveValue(newTemplateVersion.name);
  // Publish
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  );
  expect(patchTemplateVersion).toBeCalledTimes(0);
});

describe.each([
  {
    testName: "Do not ask when template version has no errors",
    initialVariables: undefined,
    loadedVariables: undefined,
    templateVersion: MockTemplateVersion,
    askForVariables: false,
  },
  {
    testName:
      "Do not ask when template version has no errors even when having previously loaded variables",
    initialVariables: [
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
    ],
    loadedVariables: undefined,
    templateVersion: MockTemplateVersion,
    askForVariables: false,
  },
  {
    testName: "Ask when template version has errors",
    initialVariables: undefined,
    templateVersion: {
      ...MockTemplateVersion,
      job: {
        ...MockTemplateVersion.job,
        error_code: "REQUIRED_TEMPLATE_VARIABLES",
      },
    },
    loadedVariables: [
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
    ],
    askForVariables: true,
  },
])(
  "Missing template variables",
  ({
    testName,
    initialVariables,
    loadedVariables,
    templateVersion,
    askForVariables,
  }) => {
    it(testName, async () => {
      jest.resetAllMocks();
      const queryClient = new QueryClient();
      queryClient.setQueryData(
        templateVersionVariablesKey(MockTemplateVersion.id),
        initialVariables,
      );

      server.use(
        rest.get(
          "/api/v2/organizations/:org/templates/:template/versions/:version",
          (req, res, ctx) => {
            return res(ctx.json(templateVersion));
          },
        ),
      );

      if (loadedVariables) {
        server.use(
          rest.get(
            "/api/v2/templateversions/:version/variables",
            (req, res, ctx) => {
              return res(ctx.json(loadedVariables));
            },
          ),
        );
      }

      render(
        <AppProviders queryClient={queryClient}>
          <RouterProvider
            router={createMemoryRouter(
              [
                {
                  element: <RequireAuth />,
                  children: [
                    {
                      element: <TemplateVersionEditorPage />,
                      path: "/templates/:template/versions/:version/edit",
                    },
                  ],
                },
              ],
              {
                initialEntries: [
                  `/templates/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit`,
                ],
              },
            )}
          />
        </AppProviders>,
      );
      await waitForLoaderToBeRemoved();

      const dialogSelector = /template variables/i;
      if (askForVariables) {
        await screen.findByText(dialogSelector);
      } else {
        expect(screen.queryByText(dialogSelector)).not.toBeInTheDocument();
      }
    });
  },
);
