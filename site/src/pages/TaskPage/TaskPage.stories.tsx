import type { Meta, StoryObj } from "@storybook/react";
import { expect, spyOn, within } from "@storybook/test";
import {
	MockFailedWorkspace,
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
	MockWorkspaceResource,
	mockApiError,
} from "testHelpers/entities";
import { withProxyProvider } from "testHelpers/storybook";
import TaskPage, { data } from "./TaskPage";

const meta: Meta<typeof TaskPage> = {
	title: "pages/TaskPage",
	component: TaskPage,
	parameters: {
		layout: "fullscreen",
	},
};

export default meta;
type Story = StoryObj<typeof TaskPage>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockImplementation(
			() => new Promise((res) => 1000 * 60 * 60),
		);
	},
};

export const LoadingError: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockRejectedValue(
			mockApiError({
				message: "Failed to load task",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const WaitingOnBuild: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockStartingWorkspace,
		});
	},
};

export const WaitingOnStatus: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_app_status: null,
			},
		});
	},
};

export const FailedBuild: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockFailedWorkspace,
		});
	},
};

export const TerminatedBuild: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockStoppedWorkspace,
		});
	},
};

export const TerminatedBuildWithStatus: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockStoppedWorkspace,
				latest_app_status: MockWorkspaceAppStatus,
			},
		});
	},
};

export const Active: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					resources: [
						{
							...MockWorkspaceResource,
							agents: [
								{
									...MockWorkspaceAgent,
									apps: [
										{
											...MockWorkspaceApp,
											id: "claude-code",
											display_name: "Claude Code",
											slug: "claude-code",
											icon: "/icon/claude.svg",
											statuses: [
												MockWorkspaceAppStatus,
												{
													...MockWorkspaceAppStatus,
													id: "2",
													message: "Planning changes",
													state: "working",
												},
											],
										},
										{
											...MockWorkspaceApp,
											id: "vscode",
											slug: "vscode",
											display_name: "VS Code Web",
											icon: "/icon/code.svg",
										},
										{
											...MockWorkspaceApp,
											slug: "zed",
											id: "zed",
											display_name: "Zed",
											icon: "/icon/zed.svg",
										},
									],
								},
							],
						},
					],
				},
				latest_app_status: {
					...MockWorkspaceAppStatus,
					app_id: "claude-code",
				},
			},
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const vscodeIframe = await canvas.findByTitle("VS Code Web");
		const zedIframe = await canvas.findByTitle("Zed");
		const claudeIframe = await canvas.findByTitle("Claude Code");

		expect(vscodeIframe).not.toBeVisible();
		expect(zedIframe).not.toBeVisible();
		expect(claudeIframe).toBeVisible();
	},
};
