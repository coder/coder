import {
	MockPrimaryWorkspaceProxy,
	MockTask,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceProxies,
} from "testHelpers/entities";
import { withAuthProvider, withProxyProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Task, Workspace, WorkspaceApp } from "api/typesGenerated";
import { getPreferredProxy } from "contexts/ProxyContext";
import kebabCase from "lodash/kebabCase";
import { TaskApps } from "./TaskApps";

const mockExternalApp: WorkspaceApp = {
	...MockWorkspaceApp,
	external: true,
	health: "healthy",
};

const mockTask: Task = {
	...MockTask,
	workspace_app_id: null,
};

const meta: Meta<typeof TaskApps> = {
	title: "pages/TaskPage/TaskApps",
	component: TaskApps,
	decorators: [withProxyProvider()],
	parameters: {
		layout: "fullscreen",
	},
};

export default meta;
type Story = StoryObj<typeof TaskApps>;

export const NoEmbeddedApps: Story = {
	args: {
		task: mockTask,
		workspace: mockWorkspaceWithApps([]),
	},
};

export const WithExternalAppsOnly: Story = {
	args: {
		task: mockTask,
		workspace: mockWorkspaceWithApps([mockExternalApp]),
	},
};

export const WithEmbeddedApps: Story = {
	args: {
		task: mockTask,
		workspace: mockWorkspaceWithApps([mockEmbeddedApp()]),
	},
};

export const WithMixedApps: Story = {
	args: {
		task: mockTask,
		workspace: mockWorkspaceWithApps([mockEmbeddedApp(), mockExternalApp]),
	},
};

export const WithWildcardWarning: Story = {
	decorators: [
		withAuthProvider,
		withProxyProvider({
			proxy: {
				...getPreferredProxy(MockWorkspaceProxies, MockPrimaryWorkspaceProxy),
				preferredWildcardHostname: "",
			},
		}),
	],
	parameters: {
		user: MockUserOwner,
	},
	args: {
		task: mockTask,
		workspace: mockWorkspaceWithApps([
			{
				...mockEmbeddedApp(),
				subdomain: true,
			},
		]),
	},
};

export const WithManyEmbeddedApps: Story = {
	args: {
		task: mockTask,
		workspace: mockWorkspaceWithApps([
			mockEmbeddedApp("Code Server"),
			mockEmbeddedApp("Jupyter Notebook"),
			mockEmbeddedApp("Web Terminal"),
			mockEmbeddedApp("Database Client"),
			mockEmbeddedApp("API Documentation"),
			mockEmbeddedApp("Monitoring Dashboard"),
			mockEmbeddedApp("Task Manager"),
			mockEmbeddedApp("File Manager"),
			mockEmbeddedApp("Test Runner"),
			mockEmbeddedApp("Build Pipeline"),
		]),
	},
};

function mockEmbeddedApp(name = MockWorkspaceApp.display_name): WorkspaceApp {
	return {
		...MockWorkspaceApp,
		id: crypto.randomUUID(),
		slug: kebabCase(name),
		display_name: name,
		external: false,
		health: "healthy",
	};
}

function mockWorkspaceWithApps(apps: WorkspaceApp[]): Workspace {
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: [
				{
					...MockWorkspace.latest_build.resources[0],
					agents: [
						{
							...MockWorkspaceAgent,
							apps,
						},
					],
				},
			],
		},
	};
}
