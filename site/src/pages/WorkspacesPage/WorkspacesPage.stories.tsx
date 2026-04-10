import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fireEvent,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { workspacePermissionsByOrganization } from "#/api/queries/organizations";
import {
	getTemplatesQueryKey,
	templateVersionsQueryKey,
} from "#/api/queries/templates";
import { workspacesKey } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import { workspaceChecks } from "#/modules/workspaces/permissions";
import {
	MockDefaultOrganization,
	MockStoppedWorkspace,
	MockTemplate,
	MockTemplateVersion,
	MockUserOwner,
	MockWorkspace,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import WorkspacesPage from "./WorkspacesPage";

const workspace: Workspace = {
	...MockStoppedWorkspace,
	id: "workspace-1",
	name: "workspace-1",
	latest_build: {
		...MockStoppedWorkspace.latest_build,
		workspace_id: "workspace-1",
		workspace_name: "workspace-1",
		workspace_owner_name: MockStoppedWorkspace.owner_name,
		status: "stopped",
		updated_at: "2024-01-01T00:00:00.000Z",
	},
};

const deletingWorkspace: Workspace = {
	...workspace,
	latest_build: {
		...workspace.latest_build,
		id: "workspace-1-delete-build",
		transition: "delete",
		status: "deleting",
		updated_at: "2024-01-01T00:01:00.000Z",
	},
};

const meta = {
	title: "pages/WorkspacesPage/WorkspacesPage",
	component: WorkspacesPage,
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	parameters: {
		user: MockUserOwner,
		permissions: {
			viewDeploymentConfig: false,
		},
		queries: [
			{
				key: getTemplatesQueryKey(),
				data: [MockTemplate],
			},
			{
				key: workspacePermissionsByOrganization(
					[MockTemplate.organization_id],
					MockUserOwner.id,
				).queryKey,
				data: {
					[MockTemplate.organization_id]: {
						createWorkspaceForUserID: true,
					},
				},
			},
			{
				key: getAuthorizationKey({ checks: workspaceChecks(workspace) }),
				data: {
					readWorkspace: true,
					shareWorkspace: true,
					updateWorkspace: true,
					updateWorkspaceVersion: true,
					deleteFailedWorkspace: true,
				},
			},
			{
				key: templateVersionsQueryKey(workspace.template_id),
				data: [MockTemplateVersion],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) => {
			return Object.fromEntries(Object.keys(checks).map((key) => [key, true]));
		});
		spyOn(API, "getUsers").mockResolvedValue({
			users: [MockUserOwner],
			count: 1,
		});
		spyOn(API, "getOrganizations").mockResolvedValue([MockDefaultOrganization]);
		spyOn(API, "getWorkspaceBuildParameters").mockResolvedValue([]);
	},
} satisfies Meta<typeof WorkspacesPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const DeleteWorkspaceShowsDeletingStateImmediately: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces")
			.mockResolvedValueOnce({
				workspaces: [workspace],
				count: 1,
			})
			.mockResolvedValue({
				workspaces: [deletingWorkspace],
				count: 1,
			});
		spyOn(API, "deleteWorkspace").mockResolvedValue(
			deletingWorkspace.latest_build,
		);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await step("Open the delete dialog from the workspace row", async () => {
			const row = await canvas.findByTestId(`workspace-${workspace.id}`);
			await within(row).findByText("Stopped");
			await user.click(within(row).getByTestId("workspace-options-button"));
			await user.click(await body.findByRole("menuitem", { name: /delete/i }));
		});

		await step("Confirm deletion", async () => {
			const dialog = await body.findByRole("dialog");
			const confirmationInput = within(dialog).getByTestId(
				"delete-dialog-name-confirmation",
			);
			fireEvent.change(confirmationInput, {
				target: { value: workspace.name },
			});
			const confirmButton = within(dialog).getByTestId("confirm-button");
			await waitFor(() => {
				expect(confirmationInput).toHaveValue(workspace.name);
				expect(confirmButton).toBeEnabled();
			});
			await user.click(confirmButton);
			await waitFor(() => {
				expect(API.deleteWorkspace).toHaveBeenCalledWith(workspace.id, {
					orphan: false,
				});
			});
		});

		await step(
			"Show the workspace as deleting immediately after the mutation",
			async () => {
				const row = await canvas.findByTestId(`workspace-${workspace.id}`);
				await within(row).findByText("Deleting");
			},
		);
	},
};

const makePage = (prefix: string) =>
	Array.from({ length: 25 }, (_, i) => ({
		...MockWorkspace,
		id: `${prefix}-workspace-${i}`,
		name: `${prefix}-workspace-${i}`,
	}));

export const PaginationChangesQueryKey: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
		queries: [
			...meta.parameters.queries,
			{
				key: workspacesKey({ q: "owner:me", limit: 25, offset: 0 }),
				data: { workspaces: makePage("page1"), count: 50 },
			},
			{
				key: workspacesKey({ q: "owner:me", limit: 25, offset: 25 }),
				data: { workspaces: makePage("page2"), count: 50 },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await step("Page 1 renders from cache", async () => {
			await canvas.findByText("page1-workspace-0");
		});

		await step("Clicking next page shows page 2 data", async () => {
			const nextButton = await canvas.findByRole("button", {
				name: /next page/i,
			});
			await user.click(nextButton);

			await canvas.findByText("page2-workspace-0");
			await waitFor(() => {
				expect(canvas.queryByText("page1-workspace-0")).not.toBeInTheDocument();
			});
		});
	},
};
