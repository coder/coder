import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, spyOn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { chatModels } from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import {
	MockDefaultOrganization,
	MockNoOrganizationPermissions,
	MockNoPermissions,
	MockUserMember,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { AISettingsIndexRedirect } from "./AISettingsIndexRedirect";

const meta: Meta<typeof AISettingsIndexRedirect> = {
	title: "pages/AISettingsPage/AISettingsIndexRedirect",
	component: AISettingsIndexRedirect,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		user: MockUserMember,
		permissions: MockNoPermissions,
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings" },
			routing: [
				{ path: "/ai/settings", useStoryElement: true },
				{
					path: "/ai/settings/mcp-servers",
					element: <h1>Organization MCP servers</h1>,
				},
				{ path: "/ai/settings/providers", element: <h1>AI providers</h1> },
				{
					path: "/ai/settings/mcp-servers/add",
					element: <h1>Add MCP server</h1>,
				},
				{ path: "/ai/settings/templates", element: <h1>Templates</h1> },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AISettingsIndexRedirect>;

export const OrganizationMCPSharerRedirectsToMCPServers: Story = {
	parameters: {
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [] },
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: {
						...MockNoOrganizationPermissions,
						shareMCPServerConfig: true,
					},
				},
			},
		],
	},
	play: async () => {
		await expect(
			await screen.findByRole("heading", {
				name: "Organization MCP servers",
			}),
		).toBeInTheDocument();
	},
};

export const MemberWithoutMCPSharingFallsBack: Story = {
	parameters: {
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [] },
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockNoOrganizationPermissions,
				},
			},
		],
	},
	play: async () => {
		await expect(
			await screen.findByRole("heading", { name: "AI providers" }),
		).toBeInTheDocument();
	},
};

export const MCPSharingPermissionLookupFailureShowsError: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockRejectedValue(
			new Error("Unable to load organization permissions"),
		);
	},
	parameters: {
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [] },
			},
		],
	},
	play: async () => {
		await expect(
			await screen.findByText("Unable to load organization permissions"),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("heading", { name: "AI providers" }),
		).not.toBeInTheDocument();
	},
};

export const TemplateUpdateOnlyAdminRedirectsToTemplates: Story = {
	parameters: {
		permissions: {
			...MockNoPermissions,
			updateAnyTemplate: true,
		},
	},
	play: async () => {
		await expect(
			await screen.findByRole("heading", { name: "Templates" }),
		).toBeInTheDocument();
	},
};

export const MCPUpdateOnlyAdminRedirectsToMCPServers: Story = {
	parameters: {
		permissions: {
			...MockNoPermissions,
			updateAnyMCPServerConfig: true,
		},
	},
	play: async () => {
		await expect(
			await screen.findByRole("heading", { name: "Organization MCP servers" }),
		).toBeInTheDocument();
	},
};

export const MCPDeleteOnlyAdminRedirectsToMCPServers: Story = {
	parameters: {
		permissions: {
			...MockNoPermissions,
			deleteAnyMCPServerConfig: true,
		},
	},
	play: async () => {
		await expect(
			await screen.findByRole("heading", { name: "Organization MCP servers" }),
		).toBeInTheDocument();
	},
};

export const MCPCreateOnlyAdminRedirectsToAddMCPServer: Story = {
	parameters: {
		permissions: {
			...MockNoPermissions,
			createAnyMCPServerConfig: true,
		},
	},
	play: async () => {
		await expect(
			await screen.findByRole("heading", { name: "Add MCP server" }),
		).toBeInTheDocument();
	},
};
