import type { Meta, StoryObj } from "@storybook/react";
import TasksPage, { data } from "./TasksPage";
import { spyOn } from "@storybook/test";
import {
	mockApiError,
	MockProxyLatencies,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAppStatus,
} from "testHelpers/entities";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";

const meta: Meta<typeof TasksPage> = {
	title: "pages/TasksPage",
	component: TasksPage,
};

export default meta;
type Story = StoryObj<typeof TasksPage>;

export const LoadingAITemplates: Story = {
	beforeEach: () => {
		spyOn(data, "fetchAITemplates").mockImplementation(
			() => new Promise((res) => 1000 * 60 * 60),
		);
	},
};

export const LoadingAITemplatesError: Story = {
	beforeEach: () => {
		spyOn(data, "fetchAITemplates").mockRejectedValue(
			mockApiError({
				message: "Failed to load AI templates",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const EmptyAITemplates: Story = {
	beforeEach: () => {
		spyOn(data, "fetchAITemplates").mockResolvedValue([]);
	},
};

export const LoadingTasks: Story = {
	beforeEach: () => {
		spyOn(data, "fetchAITemplates").mockResolvedValue([MockTemplate]);
		spyOn(data, "fetchTasks").mockImplementation(
			() => new Promise((res) => 1000 * 60 * 60),
		);
	},
};

export const LoadingTasksError: Story = {
	beforeEach: () => {
		spyOn(data, "fetchAITemplates").mockResolvedValue([MockTemplate]);
		spyOn(data, "fetchTasks").mockRejectedValue(
			mockApiError({
				message: "Failed to load tasks",
			}),
		);
	},
};

export const EmptyTasks: Story = {
	beforeEach: () => {
		spyOn(data, "fetchAITemplates").mockResolvedValue([MockTemplate]);
		spyOn(data, "fetchTasks").mockResolvedValue([]);
	},
};

export const LoadedTasks: Story = {
	decorators: [
		(Story) => (
			<ProxyContext.Provider
				value={{
					proxyLatencies: MockProxyLatencies,
					proxy: getPreferredProxy([], undefined),
					proxies: [],
					isLoading: false,
					isFetched: true,
					clearProxy: () => {
						return;
					},
					setProxy: () => {
						return;
					},
					refetchProxyLatencies: (): Date => {
						return new Date();
					},
				}}
			>
				<Story />
			</ProxyContext.Provider>
		),
	],
	beforeEach: () => {
		spyOn(data, "fetchAITemplates").mockResolvedValue([MockTemplate]);
		spyOn(data, "fetchTasks").mockResolvedValue([
			{
				workspace: {
					...MockWorkspace,
					latest_app_status: MockWorkspaceAppStatus,
				},
				prompt: "Create competitors page",
			},
			{
				workspace: {
					...MockWorkspace,
					id: "workspace-2",
					latest_app_status: {
						...MockWorkspaceAppStatus,
						message: "Avatar size fixed!",
					},
				},
				prompt: "Fix user avatar size",
			},
			{
				workspace: {
					...MockWorkspace,
					id: "workspace-3",
					latest_app_status: {
						...MockWorkspaceAppStatus,
						message: "Accessibility issues fixed!",
					},
				},
				prompt: "Fix accessibility issues",
			},
		]);
	},
};
