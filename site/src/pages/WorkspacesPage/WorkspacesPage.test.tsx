import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import * as API from "api/api";
import type { Workspace } from "api/typesGenerated";
import {
  MockStoppedWorkspace,
  MockWorkspace,
  MockDormantWorkspace,
  MockDormantOutdatedWorkspace,
  MockOutdatedWorkspace,
  MockRunningOutdatedWorkspace,
  MockWorkspacesResponse,
} from "testHelpers/entities";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import * as CreateDayString from "utils/createDayString";
import WorkspacesPage from "./WorkspacesPage";

describe("WorkspacesPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString");
    mock.mockImplementation(() => "a minute ago");
  });

  it("renders an empty workspaces page", async () => {
    // Given
    server.use(
      http.get("/api/v2/workspaces", async () => {
        return HttpResponse.json({ workspaces: [], count: 0 });
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

    // The button changes its text, and advances the content of the modal,
    // but it is technically the same button being clicked 3 times.
    const confirmButton = await screen.findByTestId("confirm-button");
    await user.click(confirmButton);
    await user.click(confirmButton);
    await user.click(confirmButton);

    await waitFor(() => {
      expect(deleteWorkspace).toHaveBeenCalledTimes(2);
    });
    expect(deleteWorkspace).toHaveBeenCalledWith(workspaces[0].id);
    expect(deleteWorkspace).toHaveBeenCalledWith(workspaces[1].id);
  });

  describe("batch update", () => {
    it("ignores up-to-date workspaces", async () => {
      const workspaces = [
        { ...MockWorkspace, id: "1" }, // running, not outdated. no warning.
        { ...MockDormantWorkspace, id: "2" }, // dormant, not outdated. no warning.
        { ...MockOutdatedWorkspace, id: "3" },
        { ...MockOutdatedWorkspace, id: "4" },
      ];
      jest
        .spyOn(API, "getWorkspaces")
        .mockResolvedValue({ workspaces, count: workspaces.length });
      const updateWorkspace = jest.spyOn(API, "updateWorkspace");
      const user = userEvent.setup();
      renderWithAuth(<WorkspacesPage />);
      await waitForLoaderToBeRemoved();

      for (const workspace of workspaces) {
        await user.click(getWorkspaceCheckbox(workspace));
      }

      await user.click(screen.getByRole("button", { name: /actions/i }));
      const updateButton = await screen.findByText(/update/i);
      await user.click(updateButton);

      // One click: no running workspaces warning, no dormant workspaces warning.
      // There is a running workspace and a dormant workspace selected, but they
      // are not outdated.
      const confirmButton = await screen.findByTestId("confirm-button");
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent(/used by/i);
      await user.click(confirmButton);

      // `workspaces[0]` was up-to-date, and running
      // `workspaces[1]` was dormant
      await waitFor(() => {
        expect(updateWorkspace).toHaveBeenCalledTimes(2);
      });
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[2]);
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[3]);
    });

    it("warns about and updates running workspaces", async () => {
      const workspaces = [
        { ...MockRunningOutdatedWorkspace, id: "1" },
        { ...MockOutdatedWorkspace, id: "2" },
        { ...MockOutdatedWorkspace, id: "3" },
      ];
      jest
        .spyOn(API, "getWorkspaces")
        .mockResolvedValue({ workspaces, count: workspaces.length });
      const updateWorkspace = jest.spyOn(API, "updateWorkspace");
      const user = userEvent.setup();
      renderWithAuth(<WorkspacesPage />);
      await waitForLoaderToBeRemoved();

      for (const workspace of workspaces) {
        await user.click(getWorkspaceCheckbox(workspace));
      }

      await user.click(screen.getByRole("button", { name: /actions/i }));
      const updateButton = await screen.findByText(/update/i);
      await user.click(updateButton);

      // Two clicks: 1 running workspace, no dormant workspaces warning.
      const confirmButton = await screen.findByTestId("confirm-button");
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent(/1 running workspace/i);
      await user.click(confirmButton);
      expect(dialog).toHaveTextContent(/used by/i);
      await user.click(confirmButton);

      await waitFor(() => {
        expect(updateWorkspace).toHaveBeenCalledTimes(3);
      });
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[0]);
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[1]);
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[2]);
    });

    it("warns about and ignores dormant workspaces", async () => {
      const workspaces = [
        { ...MockDormantOutdatedWorkspace, id: "1" },
        { ...MockOutdatedWorkspace, id: "2" },
        { ...MockOutdatedWorkspace, id: "3" },
      ];
      jest
        .spyOn(API, "getWorkspaces")
        .mockResolvedValue({ workspaces, count: workspaces.length });
      const updateWorkspace = jest.spyOn(API, "updateWorkspace");
      const user = userEvent.setup();
      renderWithAuth(<WorkspacesPage />);
      await waitForLoaderToBeRemoved();

      for (const workspace of workspaces) {
        await user.click(getWorkspaceCheckbox(workspace));
      }

      await user.click(screen.getByRole("button", { name: /actions/i }));
      const updateButton = await screen.findByText(/update/i);
      await user.click(updateButton);

      // Two clicks: no running workspaces warning, 1 dormant workspace.
      const confirmButton = await screen.findByTestId("confirm-button");
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent(/dormant/i);
      await user.click(confirmButton);
      expect(dialog).toHaveTextContent(/used by/i);
      await user.click(confirmButton);

      // `workspaces[0]` was dormant
      await waitFor(() => {
        expect(updateWorkspace).toHaveBeenCalledTimes(2);
      });
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[1]);
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[2]);
    });

    it("warns about running workspaces and then dormant workspaces", async () => {
      const workspaces = [
        { ...MockRunningOutdatedWorkspace, id: "1" },
        { ...MockDormantOutdatedWorkspace, id: "2" },
        { ...MockOutdatedWorkspace, id: "3" },
        { ...MockOutdatedWorkspace, id: "4" },
        { ...MockWorkspace, id: "5" },
      ];
      jest
        .spyOn(API, "getWorkspaces")
        .mockResolvedValue({ workspaces, count: workspaces.length });
      const updateWorkspace = jest.spyOn(API, "updateWorkspace");
      const user = userEvent.setup();
      renderWithAuth(<WorkspacesPage />);
      await waitForLoaderToBeRemoved();

      for (const workspace of workspaces) {
        await user.click(getWorkspaceCheckbox(workspace));
      }

      await user.click(screen.getByRole("button", { name: /actions/i }));
      const updateButton = await screen.findByText(/update/i);
      await user.click(updateButton);

      // Three clicks: 1 running workspace, 1 dormant workspace.
      const confirmButton = await screen.findByTestId("confirm-button");
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent(/1 running workspace/i);
      await user.click(confirmButton);
      expect(dialog).toHaveTextContent(/dormant/i);
      await user.click(confirmButton);
      expect(dialog).toHaveTextContent(/used by/i);
      await user.click(confirmButton);

      // `workspaces[1]` was dormant, and `workspaces[4]` was up-to-date
      await waitFor(() => {
        expect(updateWorkspace).toHaveBeenCalledTimes(3);
      });
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[0]);
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[2]);
      expect(updateWorkspace).toHaveBeenCalledWith(workspaces[3]);
    });
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
