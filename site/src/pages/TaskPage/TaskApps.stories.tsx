import {
	MockPrimaryWorkspaceProxy,
	MockTasks,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceProxies,
} from "testHelpers/entities";
import { withAuthProvider, withProxyProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceApp } from "api/typesGenerated";
import { getPreferredProxy } from "contexts/ProxyContext";
import kebabCase from "lodash/kebabCase";
import type { Task } from "modules/tasks/tasks";
import { TaskApps } from "./TaskApps";

const mockExternalApp: WorkspaceApp = {
	...MockWorkspaceApp,
	external: true,
	health: "healthy",
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
		task: mockTask([]),
	},
};

export const WithExternalAppsOnly: Story = {
	args: {
		task: mockTask([mockExternalApp]),
	},
};

export const WithEmbeddedApps: Story = {
	args: {
		task: mockTask([mockEmbeddedApp()]),
	},
};

export const WithMixedApps: Story = {
	args: {
		task: mockTask([mockEmbeddedApp(), mockExternalApp]),
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
		task: mockTask([
			{
				...mockEmbeddedApp(),
				subdomain: true,
			},
		]),
	},
};

export const WithManyEmbeddedApps: Story = {
	args: {
		task: mockTask([
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
	};
}

function mockTask(apps: WorkspaceApp[]): Task {
	return {
		...MockTasks[0],
		workspace: {
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
		},
	};
}
