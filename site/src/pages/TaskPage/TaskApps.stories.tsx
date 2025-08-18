import {
	MockTasks,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "testHelpers/entities";
import { withProxyProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceApp } from "api/typesGenerated";
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
