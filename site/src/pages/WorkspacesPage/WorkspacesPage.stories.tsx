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
import type { Workspace, WorkspaceAppHealth } from "#/api/typesGenerated";
import { workspaceChecks } from "#/modules/workspaces/permissions";
import {
	MockDefaultOrganization,
	MockDormantOutdatedWorkspace,
	MockDormantWorkspace,
	MockOutdatedStoppedWorkspaceAlwaysUpdate,
	MockOutdatedWorkspace,
	MockRunningOutdatedWorkspace,
	MockStoppedWorkspace,
	MockTemplate,
	MockTemplateVersion,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceBuild,
	MockWorkspacesResponse,
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
		pixel: { exclude: true },
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

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({ workspaces: [], count: 0 });
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: /Create a workspace/ });
	},
};

export const RendersWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue(MockWorkspacesResponse);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(`${MockWorkspace.name}1`);
		const templateDisplayNames = await canvas.findAllByText(
			MockWorkspace.template_display_name,
		);
		expect(templateDisplayNames).toHaveLength(MockWorkspacesResponse.count);
	},
};

const deletableWorkspaces: Workspace[] = [
	{ ...MockWorkspace, id: "1" },
	{ ...MockWorkspace, id: "2" },
	{ ...MockWorkspace, id: "3" },
];

export const DeletesOnlySelectedWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: deletableWorkspaces,
			count: deletableWorkspaces.length,
		});
		spyOn(API, "deleteWorkspace").mockResolvedValue(MockWorkspaceBuild);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2"]);
		await openBulkActions(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: /delete/i }));

		const confirmButton = await body.findByTestId("confirm-button");
		await user.click(confirmButton);
		await user.click(confirmButton);
		await user.click(confirmButton);

		await waitFor(() => expect(API.deleteWorkspace).toHaveBeenCalledTimes(2));
		expect(API.deleteWorkspace).toHaveBeenCalledWith("1");
		expect(API.deleteWorkspace).toHaveBeenCalledWith("2");
	},
};

const skipUpToDateWorkspaces: Workspace[] = [
	{ ...MockWorkspace, id: "1" },
	{ ...MockDormantWorkspace, id: "2" },
	{ ...MockOutdatedWorkspace, id: "3" },
	{
		...MockOutdatedWorkspace,
		id: "4",
		latest_build: { ...MockOutdatedWorkspace.latest_build, status: "running" },
	},
];

export const BatchUpdateSkipsUpToDateWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: skipUpToDateWorkspaces,
			count: skipUpToDateWorkspaces.length,
		});
		spyOn(API, "updateWorkspace").mockResolvedValue(MockWorkspaceBuild);
		spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2", "3", "4"]);
		await openBulkActions(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: /Update/ }));

		const modal = await body.findByRole("dialog", { name: /Review Updates/i });
		await user.click(
			within(modal).getByRole("checkbox", {
				name: /I acknowledge these risks\./,
			}),
		);
		await user.click(within(modal).getByRole("button", { name: /Update/ }));

		await waitFor(() => expect(API.updateWorkspace).toHaveBeenCalledTimes(2));
		expect(API.updateWorkspace).toHaveBeenCalledWith(
			skipUpToDateWorkspaces[2],
			[],
			false,
		);
		expect(API.updateWorkspace).toHaveBeenCalledWith(
			skipUpToDateWorkspaces[3],
			[],
			false,
		);
	},
};

const updateRunningWorkspaces: Workspace[] = [
	{ ...MockRunningOutdatedWorkspace, id: "1" },
	{ ...MockOutdatedWorkspace, id: "2" },
	{ ...MockOutdatedWorkspace, id: "3" },
];

export const BatchUpdateRunningWorkspace: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: updateRunningWorkspaces,
			count: updateRunningWorkspaces.length,
		});
		spyOn(API, "updateWorkspace").mockResolvedValue(MockWorkspaceBuild);
		spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2", "3"]);
		await openBulkActions(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: /Update/ }));

		const modal = await body.findByRole("dialog", { name: /Review Updates/i });
		await user.click(
			within(modal).getByRole("checkbox", {
				name: /I acknowledge these risks\./,
			}),
		);
		await user.click(within(modal).getByRole("button", { name: /Update/ }));

		await waitFor(() => expect(API.updateWorkspace).toHaveBeenCalledTimes(3));
		expect(API.updateWorkspace).toHaveBeenCalledWith(
			updateRunningWorkspaces[0],
			[],
			false,
		);
		expect(API.updateWorkspace).toHaveBeenCalledWith(
			updateRunningWorkspaces[1],
			[],
			false,
		);
		expect(API.updateWorkspace).toHaveBeenCalledWith(
			updateRunningWorkspaces[2],
			[],
			false,
		);
	},
};

const ignoreDormantWorkspaces: Workspace[] = [
	{ ...MockDormantOutdatedWorkspace, id: "1" },
	{ ...MockOutdatedWorkspace, id: "2" },
	{ ...MockOutdatedWorkspace, id: "3" },
];

export const BatchUpdateIgnoresDormantWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: ignoreDormantWorkspaces,
			count: ignoreDormantWorkspaces.length,
		});
		spyOn(API, "updateWorkspace").mockResolvedValue(MockWorkspaceBuild);
		spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2", "3"]);
		await openBulkActions(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: /Update/ }));

		const modal = await body.findByRole("dialog", { name: /Review Updates/i });
		await user.click(within(modal).getByRole("button", { name: /Update/ }));

		await waitFor(() => expect(API.updateWorkspace).toHaveBeenCalledTimes(2));
		expect(API.updateWorkspace).toHaveBeenCalledWith(
			ignoreDormantWorkspaces[1],
			[],
			false,
		);
		expect(API.updateWorkspace).toHaveBeenCalledWith(
			ignoreDormantWorkspaces[2],
			[],
			false,
		);
	},
};

const runningWorkspaces: Workspace[] = [
	{ ...MockWorkspace, id: "1" },
	{ ...MockWorkspace, id: "2" },
	{ ...MockWorkspace, id: "3" },
];

export const StopsOnlySelectedWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: runningWorkspaces,
			count: runningWorkspaces.length,
		});
		spyOn(API, "stopWorkspace").mockResolvedValue(MockWorkspaceBuild);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2"]);
		await openBulkActions(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: /stop/i }));

		await waitFor(() => expect(API.stopWorkspace).toHaveBeenCalledTimes(2));
		expect(API.stopWorkspace).toHaveBeenCalledWith("1");
		expect(API.stopWorkspace).toHaveBeenCalledWith("2");
	},
};

const stoppedWorkspaces: Workspace[] = [
	{ ...MockStoppedWorkspace, id: "1" },
	{ ...MockStoppedWorkspace, id: "2" },
	{ ...MockStoppedWorkspace, id: "3" },
];

const mixedStateWorkspaces: Workspace[] = [
	{ ...MockStoppedWorkspace, id: "1" },
	{ ...MockWorkspace, id: "2" },
	{ ...MockStoppedWorkspace, id: "3" },
];

export const StartsOnlySelectedWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: stoppedWorkspaces,
			count: stoppedWorkspaces.length,
		});
		spyOn(API, "startWorkspace").mockResolvedValue(MockWorkspaceBuild);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2"]);
		await openBulkActions(canvas, user);
		await user.click(await body.findByRole("menuitem", { name: /start/i }));

		await waitFor(() => expect(API.startWorkspace).toHaveBeenCalledTimes(2));
		expect(API.startWorkspace).toHaveBeenCalledWith(
			"1",
			MockStoppedWorkspace.latest_build.template_version_id,
		);
		expect(API.startWorkspace).toHaveBeenCalledWith(
			"2",
			MockStoppedWorkspace.latest_build.template_version_id,
		);
	},
};

export const StartIgnoresAlreadyRunningWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: mixedStateWorkspaces,
			count: mixedStateWorkspaces.length,
		});
		spyOn(API, "startWorkspace").mockResolvedValue(MockWorkspaceBuild);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2", "3"]);
		await openBulkActions(canvas, user);

		const startItem = await body.findByRole("menuitem", { name: /start/i });
		expect(startItem).not.toHaveAttribute("data-disabled");
		await user.click(startItem);

		await waitFor(() => expect(API.startWorkspace).toHaveBeenCalledTimes(2));
		expect(API.startWorkspace).toHaveBeenCalledWith(
			"1",
			MockStoppedWorkspace.latest_build.template_version_id,
		);
		expect(API.startWorkspace).toHaveBeenCalledWith(
			"3",
			MockStoppedWorkspace.latest_build.template_version_id,
		);
	},
};

export const StopIgnoresAlreadyStoppedWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: mixedStateWorkspaces,
			count: mixedStateWorkspaces.length,
		});
		spyOn(API, "stopWorkspace").mockResolvedValue(MockWorkspaceBuild);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2", "3"]);
		await openBulkActions(canvas, user);

		const stopItem = await body.findByRole("menuitem", { name: /stop/i });
		expect(stopItem).not.toHaveAttribute("data-disabled");
		await user.click(stopItem);

		await waitFor(() => expect(API.stopWorkspace).toHaveBeenCalledTimes(1));
		expect(API.stopWorkspace).toHaveBeenCalledWith("2");
	},
};

export const StartDisabledWhenNoWorkspacesAreStartable: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: runningWorkspaces,
			count: runningWorkspaces.length,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2", "3"]);
		await openBulkActions(canvas, user);

		const startItem = await body.findByRole("menuitem", { name: /start/i });
		expect(startItem).toHaveAttribute("data-disabled");
	},
};

export const StopDisabledWhenNoWorkspacesAreStoppable: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: stoppedWorkspaces,
			count: stoppedWorkspaces.length,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();

		await selectWorkspaces(canvas, user, ["1", "2", "3"]);
		await openBulkActions(canvas, user);

		const stopItem = await body.findByRole("menuitem", { name: /stop/i });
		expect(stopItem).toHaveAttribute("data-disabled");
	},
};

const appHealthStatuses: [WorkspaceAppHealth, boolean][] = [
	["healthy", true],
	["disabled", true],
	["unhealthy", false],
	["initializing", false],
];

const appsWorkspace: Workspace = {
	...MockWorkspace,
	latest_build: {
		...MockWorkspace.latest_build,
		status: "running",
		resources: [
			{
				...MockWorkspace.latest_build.resources[0],
				agents: [
					{
						...MockWorkspaceAgent,
						display_apps: [],
						apps: [
							...appHealthStatuses.map(([health]) => ({
								...MockWorkspaceApp,
								id: `${health}-app`,
								slug: `${health}-app`,
								display_name: `${health} App`,
								health,
								hidden: false,
							})),
							{
								...MockWorkspaceApp,
								id: "hidden-app",
								slug: "hidden-app",
								display_name: "Hidden App",
								health: "healthy",
								hidden: true,
							},
						],
					},
				],
			},
		],
	},
};

export const FiltersWorkspaceAppsByHealthAndVisibility: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [appsWorkspace],
			count: 1,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("link", {
			name: (name) => name.toLowerCase().includes("healthy app"),
		});

		for (const [health, shouldBeVisible] of appHealthStatuses) {
			const link = canvas.queryByRole("link", {
				name: (name) => name.toLowerCase().includes(`${health} app`),
			});
			if (shouldBeVisible) {
				expect(link).toBeInTheDocument();
			} else {
				expect(link).not.toBeInTheDocument();
			}
		}

		expect(
			canvas.queryByRole("link", {
				name: (name) => name.toLowerCase().includes("hidden app"),
			}),
		).not.toBeInTheDocument();
	},
};

export const OutdatedStoppedAlwaysUpdateHidesStartButton: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [MockOutdatedStoppedWorkspaceAlwaysUpdate],
			count: 1,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("button", { name: "Update and start workspace" });
		expect(
			canvas.queryByRole("button", { name: "Start workspace" }),
		).not.toBeInTheDocument();
	},
};

async function selectWorkspaces(
	canvas: ReturnType<typeof within>,
	user: ReturnType<typeof userEvent.setup>,
	ids: string[],
) {
	for (const id of ids) {
		await user.click(await canvas.findByTestId(`checkbox-${id}`));
	}
}

async function openBulkActions(
	canvas: ReturnType<typeof within>,
	user: ReturnType<typeof userEvent.setup>,
) {
	await user.click(
		await canvas.findByRole("button", { name: /bulk actions/i }),
	);
}
