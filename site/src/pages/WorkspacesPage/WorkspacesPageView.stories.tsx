import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import uniqueId from "lodash/uniqueId";
import { expect, within } from "storybook/test";
import {
	type Workspace,
	type WorkspaceStatus,
	WorkspaceStatuses,
} from "#/api/typesGenerated";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { getDefaultFilterProps } from "#/components/Filter/storyHelpers";
import { DEFAULT_RECORDS_PER_PAGE } from "#/components/PaginationWidget/utils";
import {
	MockBuildInfo,
	MockOrganization,
	MockPendingProvisionerJob,
	MockTaskWorkspace,
	MockTemplate,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceSubAgent,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import { WorkspacesPageView } from "./WorkspacesPageView";

const createWorkspace = (
	name: string,
	status: WorkspaceStatus,
	outdated = false,
	lastUsedAt = "0001-01-01",
	dormantAt?: string,
	deletingAt?: string,
): Workspace => {
	return {
		...MockWorkspace,
		id: uniqueId("workspace"),
		name: name,
		outdated,
		latest_build: {
			...MockWorkspace.latest_build,
			status,
			job:
				status === "pending"
					? MockPendingProvisionerJob
					: MockWorkspace.latest_build.job,
		},
		last_used_at: lastUsedAt,
		dormant_at: dormantAt || null,
		deleting_at: deletingAt || null,
	};
};

// This is type restricted to prevent future statuses from slipping
// through the cracks unchecked!
const workspaces = WorkspaceStatuses.map((status) =>
	createWorkspace(status, status),
);

// Additional Workspaces depending on time
const additionalWorkspaces: Record<string, Workspace> = {
	today: createWorkspace(
		"running-outdated",
		"running",
		true,
		dayjs().subtract(3, "hour").toString(),
	),
	old: createWorkspace(
		"old-outdated",
		"running",
		true,
		dayjs().subtract(1, "week").toString(),
	),
	oldStopped: createWorkspace(
		"old-stopped-outdated",
		"stopped",
		true,
		dayjs().subtract(1, "week").toString(),
	),
	oldRequireActiveVersion: {
		...createWorkspace(
			"old-require-active-version-outdated",
			"running",
			true,
			dayjs().subtract(1, "week").toString(),
		),
		template_require_active_version: true,
	},
	oldStoppedRequireActiveVersion: {
		...createWorkspace(
			"old-stopped-require-active-version-outdated",
			"stopped",
			true,
			dayjs().subtract(1, "week").toString(),
		),
		template_require_active_version: true,
	},
	veryOld: createWorkspace(
		"very-old-running-outdated",
		"running",
		true,
		dayjs().subtract(1, "month").subtract(4, "day").toString(),
	),
};

const dormantWorkspaces: Record<string, Workspace> = {
	dormantNoDelete: createWorkspace(
		"dormant-no-delete",
		"stopped",
		false,
		dayjs().subtract(1, "month").toString(),
		dayjs().subtract(1, "month").toString(),
	),
	dormantAutoDelete: createWorkspace(
		"dormant-auto-delete",
		"stopped",
		false,
		dayjs().subtract(1, "month").toString(),
		dayjs().subtract(1, "month").toString(),
		dayjs().add(29, "day").toString(),
	),
};

const allWorkspaces = [
	...Object.values(workspaces),
	...Object.values(additionalWorkspaces),
];

const defaultFilter = getDefaultFilterProps<{ filter: UseFilterResult }>({
	query: "owner:me",
	values: {
		owner: MockUserOwner.username,
		template: undefined,
		status: undefined,
	},
}).filter;

const mockTemplates = [
	MockTemplate,
	...[1, 2, 3, 4].map((num) => {
		return {
			...MockTemplate,
			active_user_count: Math.floor(Math.random() * 10) * num,
			display_name: `Extra Template ${num}`,
			description: "Auto-Generated template",
			icon: num % 2 === 0 ? "" : "/icon/goland.svg",
		};
	}),
];

const meta: Meta<typeof WorkspacesPageView> = {
	title: "pages/WorkspacesPage",
	component: WorkspacesPageView,
	args: {
		limit: DEFAULT_RECORDS_PER_PAGE,
		filter: defaultFilter,
		checkedWorkspaces: [],
		templates: mockTemplates,
		templatesFetchStatus: "success",
		canCreateWorkspace: true,
		count: 13,
		page: 1,
	},
	parameters: {
		queries: [
			{
				key: ["buildInfo"],
				data: MockBuildInfo,
			},
		],
		user: MockUserOwner,
	},
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
};

export default meta;
type Story = StoryObj<typeof WorkspacesPageView>;

export const CannotCreateWorkspace: Story = {
	args: {
		workspaces: [],
		count: 0,
		canCreateWorkspace: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("button", { name: /new workspace/i })).toBeNull();
		await canvas.findByText(/don't have permission to create workspaces/i);
	},
};

export const CannotCreateWorkspaceWithWorkspaces: Story = {
	args: {
		workspaces: allWorkspaces,
		count: allWorkspaces.length,
		canCreateWorkspace: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(allWorkspaces[0].name);
		expect(canvas.queryByRole("button", { name: /new workspace/i })).toBeNull();
	},
};

export const CannotCreateWorkspaceWithFilter: Story = {
	args: {
		workspaces: [],
		count: 0,
		canCreateWorkspace: false,
		filter: { ...defaultFilter, used: true },
	},
	// The filter empty state takes priority: an active filter that matched
	// nothing shows "no results" regardless of create permission, since the
	// user may own workspaces the filter excluded.
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(/no results matched your search/i);
		expect(
			canvas.queryByText(/don't have permission to create workspaces/i),
		).toBeNull();
		expect(canvas.queryByRole("button", { name: /new workspace/i })).toBeNull();
	},
};

export const AllStates: Story = {
	args: {
		workspaces: allWorkspaces,
		count: allWorkspaces.length,
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByText(allWorkspaces[0].name);
		const images = canvasElement.querySelectorAll("img");
		expect(images.length).toBeGreaterThan(0);
		for (const img of images) {
			expect(img).toHaveAttribute("alt");
		}
	},
};

export const Loading: Story = {
	args: {
		workspaces: undefined,
		count: undefined,
	},
};

export const AllStatesWithFavorites: Story = {
	args: {
		workspaces: allWorkspaces.map((workspace, i) => ({
			...workspace,
			// NOTE: testing sort order is not relevant here.
			favorite: i % 2 === 0,
		})),
		count: allWorkspaces.length,
	},
};

const icons = [
	"/icon/code.svg",
	"/icon/aws.svg",
	"/icon/docker-white.svg",
	"/icon/docker.svg",
	"",
	"/icon/doesntexist.svg",
];

export const Icons: Story = {
	args: {
		workspaces: allWorkspaces.map((workspace, i) => ({
			...workspace,
			template_icon: icons[i % icons.length],
		})),
		count: allWorkspaces.length,
	},
};

export const OwnerHasNoWorkspaces: Story = {
	args: {
		workspaces: [],
		count: 0,
		canCreateTemplate: true,
		canCreateWorkspace: true,
	},
};

export const OwnerHasNoWorkspacesAndNoTemplates: Story = {
	args: {
		workspaces: [],
		templates: [],
		count: 0,
		canCreateTemplate: true,
		canCreateWorkspace: true,
	},
};

export const UserHasNoWorkspaces: Story = {
	args: {
		workspaces: [],
		count: 0,
		canCreateTemplate: false,
		canCreateWorkspace: true,
	},
};

export const UserHasNoWorkspacesAndNoTemplates: Story = {
	args: {
		workspaces: [],
		templates: [],
		count: 0,
		canCreateTemplate: false,
		canCreateWorkspace: true,
	},
};

export const NoSearchResults: Story = {
	args: {
		workspaces: [],
		filter: {
			...defaultFilter,
			query: "searchwithnoresults",
			used: true,
		},
		count: 0,
	},
};

export const UnhealthyWorkspace: Story = {
	args: {
		workspaces: [
			{
				...createWorkspace("unhealthy", "running"),
				health: {
					healthy: false,
					failing_agents: [],
				},
			},
		],
	},
};

export const DormantWorkspaces: Story = {
	args: {
		workspaces: Object.values(dormantWorkspaces),
		count: Object.values(dormantWorkspaces).length,
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({ message: "Something went wrong" }),
	},
};

export const InvalidPageNumber: Story = {
	args: {
		workspaces: [],
		count: 200,
		limit: 25,
		page: 1000,
	},
};

export const MultipleApps: Story = {
	args: {
		workspaces: [
			{
				...MockWorkspace,
				name: "multiple-apps",
				latest_build: {
					...MockWorkspace.latest_build,
					resources: [
						{
							...MockWorkspace.latest_build.resources[0],
							agents: [
								{
									...MockWorkspaceAgent,
									apps: [
										{
											...MockWorkspaceAgent.apps[0],
											display_name: "App 1",
											id: "app-1",
										},
										{
											...MockWorkspaceAgent.apps[0],
											display_name: "App 2",
											id: "app-2",
										},
									],
								},
							],
						},
					],
				},
			},
		],
		count: allWorkspaces.length,
	},
};

// The shortcuts row only renders apps from the parent agent (the agent without
// a `parent_id`). Apps from sub-agents, such as those created by devcontainers,
// are excluded so the row stays deterministic regardless of agent ordering.
export const ParentAgentApps: Story = {
	args: {
		workspaces: [
			{
				...MockWorkspace,
				name: "parent-agent-apps",
				latest_build: {
					...MockWorkspace.latest_build,
					resources: [
						{
							...MockWorkspace.latest_build.resources[0],
							agents: [
								// Sub-agent is listed first to prove ordering does
								// not determine which apps are shown.
								{
									...MockWorkspaceSubAgent,
									display_apps: [],
									apps: [
										{
											...MockWorkspaceApp,
											id: "sub-agent-app",
											slug: "sub-agent-app",
											display_name: "Sub Agent App",
											health: "healthy",
										},
									],
								},
								{
									...MockWorkspaceAgent,
									display_apps: [],
									apps: [
										{
											...MockWorkspaceApp,
											id: "parent-agent-app",
											slug: "parent-agent-app",
											display_name: "Parent Agent App",
											health: "healthy",
										},
									],
								},
							],
						},
					],
				},
			},
		],
		count: allWorkspaces.length,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("link", { name: /Open Parent Agent App/i });
		expect(
			canvas.queryByRole("link", { name: /Open Sub Agent App/i }),
		).not.toBeInTheDocument();
	},
};

// An external app with an unparsable URL must not crash the table. Its icon
// renders as a non-navigating button with an explanatory label instead of a
// broken link.
export const InvalidAppUrl: Story = {
	args: {
		workspaces: [
			{
				...MockWorkspace,
				name: "invalid-app-url",
				latest_build: {
					...MockWorkspace.latest_build,
					resources: [
						{
							...MockWorkspace.latest_build.resources[0],
							agents: [
								{
									...MockWorkspaceAgent,
									display_apps: [],
									apps: [
										{
											...MockWorkspaceApp,
											id: "invalid-app",
											slug: "invalid-app",
											display_name: "Broken App",
											health: "healthy",
											external: true,
											// A bare string with no scheme is unparsable
											// by the URL constructor.
											url: "my-repo",
										},
									],
								},
							],
						},
					],
				},
			},
		],
		count: allWorkspaces.length,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The invalid app renders a non-navigating button, not a link.
		await canvas.findByRole("button", {
			name: /Broken App has an invalid URL/i,
		});
		expect(
			canvas.queryByRole("link", { name: /Broken App/i }),
		).not.toBeInTheDocument();
	},
};

export const ShowOrganizations: Story = {
	args: {
		workspaces: [
			{
				...MockWorkspace,
				name: "other-org-workspace",
				organization_name: "limbus-co",
			},
		],
	},

	parameters: {
		showOrganizations: true,
		organizations: [
			{
				...MockOrganization,
				name: "limbus-co",
				display_name: "Limbus Company, LLC",
			},
		],
	},

	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const accessibleTableCell = await canvas.findByRole("cell", {
			// The organization label is always visually hidden, but the test
			// makes sure that there's a screen reader hook to give the table
			// cell more structured output
			name: /organization: Limbus Company, LLC/i,
		});

		expect(accessibleTableCell).toBeDefined();
	},
};

export const ShowWorkspaceTasks: Story = {
	args: {
		workspaces: [
			{
				...MockWorkspace,
				name: "regular-user-workspace",
			},
			{
				...MockTaskWorkspace,
				name: "task-workspace",
			},
		],
	},
};

export const ShowWorkspaceChats: Story = {
	args: {
		workspaces: [
			{
				...MockWorkspace,
				name: "regular-workspace",
			},
			{
				...MockWorkspace,
				id: "ws-with-agent",
				name: "agent-workspace",
			},
		],
		chatsByWorkspace: { "ws-with-agent": "some-chat-id" },
	},
};

export const WithCheckedWorkspaces: Story = {
	args: {
		workspaces: allWorkspaces.slice(0, 5),
		checkedWorkspaces: allWorkspaces.slice(0, 2),
		count: 5,
	},
};

// An invalid filter query returns an API validation error. The page suppresses
// its ErrorAlert for validation errors, so the message must surface on the
// filter itself and the input must be marked invalid.
export const WithFilterError: Story = {
	args: {
		workspaces: [],
		count: 0,
		error: mockApiError({
			message: "Invalid filter query.",
			validations: [
				{ field: "q", detail: 'Query param "q" has an invalid value.' },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(/invalid value/i);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter workspaces…",
		});
		expect(input).toHaveAttribute("aria-invalid", "true");
		const alert = canvas.getByRole("alert");
		expect(input).toHaveAttribute("aria-errormessage", alert.id);
	},
};
