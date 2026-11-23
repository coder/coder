import { screen, waitFor } from "@testing-library/react";
import { API } from "#/api/api";
import type { AuthorizationResponse } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import { MockWorkspace } from "#/testHelpers/entities";
import { renderWithWorkspaceSettingsLayout } from "#/testHelpers/renderHelpers";
import WorkspaceParametersPage from "./WorkspaceParametersPage/WorkspaceParametersPage";
import WorkspaceSchedulePage from "./WorkspaceSchedulePage/WorkspaceSchedulePage";
import WorkspaceSharingPage from "./WorkspaceSharingPage/WorkspaceSharingPage";

const pages = [
	{
		name: "schedule",
		permission: "updateWorkspace",
		element: <WorkspaceSchedulePage />,
	},
	{
		name: "parameters",
		permission: "startWorkspace",
		element: <WorkspaceParametersPage />,
	},
	{
		name: "sharing",
		permission: "shareWorkspace",
		element: <WorkspaceSharingPage />,
	},
];

test.each(pages)(
	"denied $name deep links do not load restricted data",
	async ({ name, permission, element }) => {
		vi.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockWorkspace,
		);
		vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
			Object.fromEntries(
				Object.keys(checks).map((key) => [key, key !== permission]),
			),
		);
		const template = vi.spyOn(API, "getTemplateByName");
		const parameters = vi.spyOn(API, "getWorkspaceBuildParameters");
		const socket = vi.spyOn(API, "templateVersionDynamicParameters");
		const acl = vi.spyOn(API, "getWorkspaceACL");

		renderWithWorkspaceSettingsLayout(element, {
			route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/settings/${name}`,
			path: "/:username/:workspace/settings/:section",
		});
		await screen.findByRole("dialog", { name: /you don't have permission/i });
		expect(template).not.toHaveBeenCalled();
		expect(parameters).not.toHaveBeenCalled();
		expect(socket).not.toHaveBeenCalled();
		expect(acl).not.toHaveBeenCalled();
	},
);

test("waits for workspace authorization before mounting schedule queries", async () => {
	vi.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(MockWorkspace);
	const authorization = createDeferred<AuthorizationResponse>();
	const check = vi
		.spyOn(API, "checkAuthorization")
		.mockImplementation(async ({ checks }) => {
			if ("updateWorkspace" in checks) return authorization.promise;
			return Object.fromEntries(Object.keys(checks).map((key) => [key, true]));
		});
	const template = vi.spyOn(API, "getTemplateByName");
	renderWithWorkspaceSettingsLayout(<WorkspaceSchedulePage />, {
		route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/settings/schedule`,
		path: "/:username/:workspace/settings/schedule",
	});
	await waitFor(() =>
		expect(check).toHaveBeenCalledWith(
			expect.objectContaining({
				checks: expect.objectContaining({ updateWorkspace: expect.anything() }),
			}),
		),
	);
	expect(template).not.toHaveBeenCalled();
	authorization.resolve({ updateWorkspace: false });
	await screen.findByRole("dialog", { name: /you don't have permission/i });
	expect(template).not.toHaveBeenCalled();
});
