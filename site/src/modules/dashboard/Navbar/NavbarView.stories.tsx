import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type { TasksFilter } from "#/api/typesGenerated";
import { AuthProvider } from "#/contexts/auth/AuthProvider";
import { AISettingsIndexRedirect } from "#/router";
import {
	MockBuildInfo,
	MockNoPermissions,
	MockTasks,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { pixelWithDesktop, pixelWithTablet } from "#/testHelpers/pixel";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { NavbarView } from "./NavbarView";

const tasksFilter: TasksFilter = {
	owner: MockUserOwner.username,
};

const memberTasksFilter: TasksFilter = {
	owner: MockUserMember.username,
};

const meta: Meta<typeof NavbarView> = {
	title: "modules/dashboard/NavbarView",
	parameters: {
		pixel: { matrix: pixelWithTablet },
		layout: "fullscreen",
		queries: [
			{
				key: ["tasks", tasksFilter],
				data: [],
			},
		],
	},
	component: NavbarView,
	args: {
		user: MockUserOwner,
		adminPermissions: {
			canViewDeployment: true,
			canViewOrganizations: true,
			canViewAISettings: true,
			canViewAuditLog: true,
			canViewConnectionLog: true,
			canViewAIBridge: true,
			canViewHealth: true,
		},
		canCreateChat: true,
		supportLinks: [],
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof NavbarView>;

export const ForAdmin: Story = {
	parameters: { pixel: { matrix: pixelWithDesktop } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
	},
};

export const ForAuditor: Story = {
	parameters: { pixel: { matrix: pixelWithDesktop } },
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAuditLog: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
	},
};

export const ForOrgAdmin: Story = {
	parameters: { pixel: { matrix: pixelWithDesktop } },
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAuditLog: true,
			canViewOrganizations: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
	},
};

export const ForMCPUpdateOnlyAdmin: Story = {
	decorators: [withAuthProvider],
	parameters: {
		pixel: { matrix: pixelWithDesktop },
		queries: [{ key: ["tasks", memberTasksFilter], data: [] }],
		user: MockUserMember,
		permissions: {
			...MockNoPermissions,
			updateAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/" },
			routing: [
				{ path: "/", useStoryElement: true },
				{
					path: "/ai/settings",
					// Route elements render outside story decorators, so the
					// redirect needs its own AuthProvider; it reads the query
					// data seeded by withAuthProvider.
					element: (
						<AuthProvider>
							<AISettingsIndexRedirect />
						</AuthProvider>
					),
				},
				{
					path: "/ai/settings/mcp-servers",
					element: <h1>MCP servers</h1>,
				},
			],
		}),
	},
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAISettings: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
		const body = within(canvasElement.ownerDocument.body);
		const aiSettingsLink = body.getByRole("menuitem", { name: "AI" });
		await expect(aiSettingsLink).toHaveAttribute("href", "/ai/settings");
		await userEvent.click(aiSettingsLink);
		await expect(
			await canvas.findByRole("heading", { name: "MCP servers" }),
		).toBeInTheDocument();
	},
};

export const ForMCPDeleteOnlyAdmin: Story = {
	decorators: [withAuthProvider],
	parameters: {
		pixel: { matrix: pixelWithDesktop },
		queries: [{ key: ["tasks", memberTasksFilter], data: [] }],
		user: MockUserMember,
		permissions: {
			...MockNoPermissions,
			deleteAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/" },
			routing: [
				{ path: "/", useStoryElement: true },
				{
					path: "/ai/settings",
					element: (
						<AuthProvider>
							<AISettingsIndexRedirect />
						</AuthProvider>
					),
				},
				{
					path: "/ai/settings/mcp-servers",
					element: <h1>MCP servers</h1>,
				},
			],
		}),
	},
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAISettings: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByRole("menuitem", { name: "AI" }));
		await expect(
			await canvas.findByRole("heading", { name: "MCP servers" }),
		).toBeInTheDocument();
	},
};

export const ForMCPCreateOnlyAdmin: Story = {
	decorators: [withAuthProvider],
	parameters: {
		pixel: { matrix: pixelWithDesktop },
		queries: [{ key: ["tasks", memberTasksFilter], data: [] }],
		user: MockUserMember,
		permissions: {
			...MockNoPermissions,
			createAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/" },
			routing: [
				{ path: "/", useStoryElement: true },
				{
					path: "/ai/settings",
					// Route elements render outside story decorators, so the
					// redirect needs its own AuthProvider; it reads the query
					// data seeded by withAuthProvider.
					element: (
						<AuthProvider>
							<AISettingsIndexRedirect />
						</AuthProvider>
					),
				},
				{
					path: "/ai/settings/mcp-servers/add",
					element: <h1>Add MCP server</h1>,
				},
			],
		}),
	},
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAISettings: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByRole("menuitem", { name: "AI" }));
		await expect(
			await canvas.findByRole("heading", { name: "Add MCP server" }),
		).toBeInTheDocument();
	},
};

export const ForSingleOrgOSSAdmin: Story = {
	parameters: { pixel: { matrix: pixelWithDesktop } },
	args: {
		adminPermissions: {
			canViewDeployment: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
	},
};

export const ForMember: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {},
		canCreateChat: false,
	},
};

export const ForMemberWithAgentsAccess: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {},
		canCreateChat: true,
	},
};

export const IdleTasks: Story = {
	parameters: {
		queries: [
			{
				key: ["tasks", tasksFilter],
				data: MockTasks,
			},
		],
	},
};

export const SupportLinks: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {},
		supportLinks: [
			{
				name: "This is a bug",
				icon: "bug",
				target: "#",
			},
			{
				name: "This is a star",
				icon: "star",
				target: "#",
				location: "navbar",
			},
			{
				name: "This is a chat",
				icon: "chat",
				target: "#",
				location: "navbar",
			},
			{
				name: "No icon here",
				icon: "",
				target: "#",
				location: "navbar",
			},
			{
				name: "No icon here too",
				icon: "",
				target: "#",
			},
		],
	},
};

export const DefaultSupportLinks: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {},
		supportLinks: [
			{ icon: "docs", name: "Documentation", target: "" },
			{ icon: "bug", name: "Report a bug", target: "" },
			{
				icon: "chat",
				name: "Join the Coder Discord",
				target: "",
				location: "navbar",
			},
			{ icon: "star", name: "Star the Repo", target: "" },
		],
	},
};

export const DevelBuild: Story = {
	args: {
		buildInfo: {
			...MockBuildInfo,
			version: "v2.21.0-devel+abc123",
			external_url: "https://github.com/coder/coder/commit/abc123",
		},
	},
};

export const RcBuild: Story = {
	args: {
		buildInfo: {
			...MockBuildInfo,
			version: "v2.21.0-rc.1+def456",
			external_url: "https://github.com/coder/coder/releases/tag/v2.21.0-rc.1",
		},
	},
};

export const RcDevelBuild: Story = {
	args: {
		buildInfo: {
			...MockBuildInfo,
			version: "v2.33.0-rc.1-devel+727ec00f7",
			external_url: "https://github.com/coder/coder/commit/727ec00f7",
		},
	},
};
