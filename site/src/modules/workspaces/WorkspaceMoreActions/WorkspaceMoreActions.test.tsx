import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type { AuthorizationResponse } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockPrebuiltWorkspace,
	MockWorkspace,
	MockWorkspaceBuildDelete,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockReadOnlyWorkspacePermissions } from "#/testHelpers/workspacePermissions";
import { WorkspaceMoreActions } from "./WorkspaceMoreActions";

it("only dispatches row actions after their permissions load", async () => {
	const authorization = createDeferred<AuthorizationResponse>();
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) => {
		if ("stopWorkspace" in checks) return authorization.promise;
		return Object.fromEntries(Object.keys(checks).map((key) => [key, true]));
	});
	const onStop = vi.fn();
	const onRestart = vi.fn();
	const user = userEvent.setup();
	renderWithAuth(
		<WorkspaceMoreActions
			workspace={MockWorkspace}
			disabled={false}
			onStop={onStop}
			onRestart={onRestart}
		/>,
	);

	await user.click(
		await screen.findByRole("button", { name: "Workspace actions" }),
	);
	const stop = screen.getByRole("menuitem", { name: "Stop…" });
	const restart = screen.getByRole("menuitem", { name: "Restart…" });

	// A dispatched click reaches the item even when CSS suppresses pointer input.
	fireEvent.click(stop);
	fireEvent.click(restart);
	expect(onStop).not.toHaveBeenCalled();
	expect(onRestart).not.toHaveBeenCalled();

	authorization.resolve({
		...MockReadOnlyWorkspacePermissions,
		stopWorkspace: true,
		updateWorkspace: true,
	});
	await screen.findByRole("menuitem", { name: "Settings" });

	fireEvent.click(restart);
	expect(onRestart).not.toHaveBeenCalled();
	await user.click(stop);
	expect(onStop).toHaveBeenCalledOnce();
});

it("deletes a prebuild with the prebuild delete permission", async () => {
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(
			Object.keys(checks).map((key) => [key, key !== "deleteWorkspace"]),
		),
	);
	const deleteWorkspace = vi
		.spyOn(API, "deleteWorkspace")
		.mockResolvedValue(MockWorkspaceBuildDelete);
	const user = userEvent.setup();
	renderWithAuth(
		<WorkspaceMoreActions workspace={MockPrebuiltWorkspace} disabled={false} />,
	);

	await user.click(
		await screen.findByRole("button", { name: "Workspace actions" }),
	);
	await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
	const dialog = within(
		await screen.findByRole("dialog", { name: /delete workspace/i }),
	);
	await user.type(
		dialog.getByLabelText("Workspace name"),
		MockPrebuiltWorkspace.name,
	);
	await user.click(dialog.getByRole("button", { name: "Delete" }));

	expect(deleteWorkspace).toHaveBeenCalledWith(MockPrebuiltWorkspace.id, {
		orphan: false,
	});
});
