import userEvent from "@testing-library/user-event";
import {
  renderWithWorkspaceSettingsLayout,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import WorkspaceSettingsPage from "./WorkspaceSettingsPage";
import { screen, waitFor, within } from "@testing-library/react";
import * as api from "api/api";
import { MockWorkspace } from "testHelpers/entities";

test("Submit the workspace settings page successfully", async () => {
  // Mock the API calls that loads data
  jest
    .spyOn(api, "getWorkspaceByOwnerAndName")
    .mockResolvedValueOnce(MockWorkspace);
  // Mock the API calls that submit data
  const patchWorkspaceSpy = jest
    .spyOn(api, "patchWorkspace")
    .mockResolvedValue();
  // Setup event and rendering
  const user = userEvent.setup();
  renderWithWorkspaceSettingsLayout(<WorkspaceSettingsPage />, {
    route: "/@test-user/test-workspace/settings",
    path: "/:username/:workspace/settings",
    // Need this because after submit the user is redirected
    extraRoutes: [{ path: "/:username/:workspace", element: <div /> }],
  });
  await waitForLoaderToBeRemoved();
  // Fill the form and submit
  const form = screen.getByTestId("form");
  const name = within(form).getByLabelText("Name");
  await user.clear(name);
  await user.type(within(form).getByLabelText("Name"), "new-name");
  await user.click(within(form).getByRole("button", { name: "Submit" }));
  // Assert that the API calls were made with the correct data
  await waitFor(() => {
    expect(patchWorkspaceSpy).toHaveBeenCalledWith(MockWorkspace.id, {
      name: "new-name",
    });
  });
});
