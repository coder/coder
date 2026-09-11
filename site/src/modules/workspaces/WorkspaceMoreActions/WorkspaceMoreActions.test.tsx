import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { API } from "#/api/api";
import { MockWorkspace } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { WorkspaceMoreActions } from "./WorkspaceMoreActions";

describe("WorkspaceMoreActions", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});
	it("does not toast when deleting a workspace fails", async () => {
		const user = userEvent.setup({ delay: 0 });
		const toastError = vi.spyOn(toast, "error");
		const deleteWorkspaceMock = vi
			.spyOn(API, "deleteWorkspace")
			.mockRejectedValueOnce(new Error("Cannot delete workspace."));
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({
			readWorkspace: true,
			shareWorkspace: true,
			updateWorkspace: true,
			updateWorkspaceVersion: false,
			deleteFailedWorkspace: false,
		});
		vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValue([]);

		const onActionSuccess = vi.fn();
		renderWithAuth(
			<WorkspaceMoreActions
				workspace={MockWorkspace}
				disabled={false}
				onActionSuccess={onActionSuccess}
			/>,
		);

		await user.click(await screen.findByTestId("workspace-options-button"));
		await user.click(await screen.findByTestId("delete-button"));

		const dialog = await screen.findByTestId("dialog");
		await user.type(
			within(dialog).getByLabelText("Workspace name"),
			MockWorkspace.name,
		);
		await user.click(
			within(dialog).getByRole("button", {
				name: "Delete",
				hidden: false,
			}),
		);

		await waitFor(() => {
			expect(deleteWorkspaceMock).toHaveBeenCalled();
		});
		expect(toastError).not.toHaveBeenCalled();
		expect(onActionSuccess).not.toHaveBeenCalled();
	});
});
