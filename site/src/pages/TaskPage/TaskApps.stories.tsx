import {
	MockTasks,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "testHelpers/entities";
import { withProxyProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceApp } from "api/typesGenerated";
import kebabCase from "lodash/kebabCase";
import { TaskApps } from "./TaskApps";

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

const mockAgentNoApps = {
	...MockWorkspaceAgent,
	apps: [],
};

const mockExternalApp: WorkspaceApp = {
	...MockWorkspaceApp,
	external: true,
};

const mockEmbeddedApp: WorkspaceApp = {
	...MockWorkspaceApp,
	external: false,
};

const taskWithNoApps = {
	...MockTasks[0],
	workspace: {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: [
				{
					...MockWorkspace.latest_build.resources[0],
					agents: [mockAgentNoApps],
				},
			],
		},
	},
};

export const NoEmbeddedApps: Story = {
	args: {
		task: taskWithNoApps,
	},
};

export const WithExternalAppsOnly: Story = {
	args: {
		task: {
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
									apps: [mockExternalApp],
								},
							],
						},
					],
				},
			},
		},
	},
};

export const WithEmbeddedApps: Story = {
	args: {
		task: {
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
									apps: [mockEmbeddedApp],
								},
							],
						},
					],
				},
			},
		},
	},
};

export const WithMixedApps: Story = {
	args: {
		task: {
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
									apps: [mockEmbeddedApp, mockExternalApp],
								},
							],
						},
					],
				},
			},
		},
	},
};

const createMockEmbeddedApp = (name: string): WorkspaceApp => ({
	...MockWorkspaceApp,
	id: window.crypto.randomUUID(),
	slug: kebabCase(name),
	display_name: name,
	external: false,
});

export const WithManyEmbeddedApps: Story = {
	args: {
		task: {
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
									apps: [
										createMockEmbeddedApp("Code Server"),
										createMockEmbeddedApp("Jupyter Notebook"),
										createMockEmbeddedApp("Web Terminal"),
										createMockEmbeddedApp("Database Client"),
										createMockEmbeddedApp("API Documentation"),
										createMockEmbeddedApp("Monitoring Dashboard"),
										createMockEmbeddedApp("Task Manager"),
										createMockEmbeddedApp("File Manager"),
										createMockEmbeddedApp("Test Runner"),
										createMockEmbeddedApp("Build Pipeline"),
									],
								},
							],
						},
					],
				},
			},
		},
	},
};
