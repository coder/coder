import { screen, waitFor, within } from "@testing-library/react";
import { rest } from "msw";
import * as CreateDayString from "utils/createDayString";
import {
  MockStoppedWorkspace,
  MockWorkspace,
  MockWorkspacesResponse,
} from "testHelpers/entities";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import WorkspacesPage from "./WorkspacesPage";
import userEvent from "@testing-library/user-event";
import * as API from "api/api";
import { Workspace } from "api/typesGenerated";

describe("WorkspacesPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString");
    mock.mockImplementation(() => "a minute ago");
  });

  it("renders an empty workspaces page", async () => {
    // Given
    server.use(
      rest.get("/api/v2/workspaces", async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json({ workspaces: [], count: 0 }));
      }),
    );

    // When
    renderWithAuth(<WorkspacesPage />);

    // Then
    await screen.findByText("Create a workspace");
  });

  it("renders a filled workspaces page", async () => {
    renderWithAuth(<WorkspacesPage />);
    await screen.findByText(`${MockWorkspace.name}1`);
    const templateDisplayNames = await screen.findAllByText(
      `${MockWorkspace.template_display_name}`,
    );
    expect(templateDisplayNames).toHaveLength(MockWorkspacesResponse.count);
  });

  it("deletes only the selected workspaces", async () => {
    const workspaces = [
      { ...MockWorkspace, id: "1" },
      { ...MockWorkspace, id: "2" },
      { ...MockWorkspace, id: "3" },
    ];
    jest
      .spyOn(API, "getWorkspaces")
      .mockResolvedValue({ workspaces, count: workspaces.length });
    const deleteWorkspace = jest.spyOn(API, "deleteWorkspace");
    const user = userEvent.setup();
    renderWithAuth(<WorkspacesPage />);
    await waitForLoaderToBeRemoved();

    await user.click(getWorkspaceCheckbox(workspaces[0]));
    await user.click(getWorkspaceCheckbox(workspaces[1]));
    await user.click(screen.getByRole("button", { name: /actions/i }));
    const deleteButton = await screen.findByText(/delete/i);
    await user.click(deleteButton);
    await user.type(screen.getByLabelText(/type delete to confirm/i), "DELETE");
    await user.click(screen.getByTestId("confirm-button"));

    await waitFor(() => {
      expect(deleteWorkspace).toHaveBeenCalledTimes(2);
    });
    expect(deleteWorkspace).toHaveBeenCalledWith(workspaces[0].id);
    expect(deleteWorkspace).toHaveBeenCalledWith(workspaces[1].id);
  });

  it("stops only the running and selected workspaces", async () => {
    const workspaces = [
      { ...MockWorkspace, id: "1" },
      { ...MockWorkspace, id: "2" },
      { ...MockWorkspace, id: "3" },
    ];
    jest
      .spyOn(API, "getWorkspaces")
      .mockResolvedValue({ workspaces, count: workspaces.length });
    const stopWorkspace = jest.spyOn(API, "stopWorkspace");
    const user = userEvent.setup();
    renderWithAuth(<WorkspacesPage />);
    await waitForLoaderToBeRemoved();

    await user.click(getWorkspaceCheckbox(workspaces[0]));
    await user.click(getWorkspaceCheckbox(workspaces[1]));
    await user.click(screen.getByRole("button", { name: /actions/i }));
    const stopButton = await screen.findByText(/stop/i);
    await user.click(stopButton);

    await waitFor(() => {
      expect(stopWorkspace).toHaveBeenCalledTimes(2);
    });
    expect(stopWorkspace).toHaveBeenCalledWith(workspaces[0].id);
    expect(stopWorkspace).toHaveBeenCalledWith(workspaces[1].id);
  });

  it("starts only the stopped and selected workspaces", async () => {
    const workspaces = [
      { ...MockStoppedWorkspace, id: "1" },
      { ...MockStoppedWorkspace, id: "2" },
      { ...MockStoppedWorkspace, id: "3" },
    ];
    jest
      .spyOn(API, "getWorkspaces")
      .mockResolvedValue({ workspaces, count: workspaces.length });
    const startWorkspace = jest.spyOn(API, "startWorkspace");
    const user = userEvent.setup();
    renderWithAuth(<WorkspacesPage />);
    await waitForLoaderToBeRemoved();

    await user.click(getWorkspaceCheckbox(workspaces[0]));
    await user.click(getWorkspaceCheckbox(workspaces[1]));
    await user.click(screen.getByRole("button", { name: /actions/i }));
    const startButton = await screen.findByText(/start/i);
    await user.click(startButton);

    await waitFor(() => {
      expect(startWorkspace).toHaveBeenCalledTimes(2);
    });
    expect(startWorkspace).toHaveBeenCalledWith(
      workspaces[0].id,
      MockStoppedWorkspace.latest_build.template_version_id,
    );
    expect(startWorkspace).toHaveBeenCalledWith(
      workspaces[1].id,
      MockStoppedWorkspace.latest_build.template_version_id,
    );
  });
});

const getWorkspaceCheckbox = (workspace: Workspace) => {
  return within(screen.getByTestId(`checkbox-${workspace.id}`)).getByRole(
    "checkbox",
  );
};
