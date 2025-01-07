import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import { chromatic } from "testHelpers/chromatic";
import { WorkspaceTimings } from "./WorkspaceTimings";
import { WorkspaceTimingsResponse } from "./storybookData";

const meta: Meta<typeof WorkspaceTimings> = {
	title: "modules/workspaces/WorkspaceTimings",
	component: WorkspaceTimings,
	args: {
		defaultIsOpen: true,
		provisionerTimings: WorkspaceTimingsResponse.provisioner_timings,
		agentScriptTimings: WorkspaceTimingsResponse.agent_script_timings,
		agentConnectionTimings: WorkspaceTimingsResponse.agent_connection_timings,
	},
	parameters: {
		chromatic,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceTimings>;

export const Open: Story = {};

export const Close: Story = {
	args: {
		defaultIsOpen: false,
	},
};

export const Loading: Story = {
	args: {
		provisionerTimings: undefined,
		agentScriptTimings: undefined,
		agentConnectionTimings: undefined,
	},
};

export const ClickToOpen: Story = {
	args: {
		defaultIsOpen: false,
	},
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await user.click(canvas.getByText("Build timeline", { exact: false }));
		await canvas.findByText("provisioning");
	},
};

export const ClickToClose: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await canvas.findByText("provisioning");
		await user.click(canvas.getByText("Build timeline", { exact: false }));
		await waitFor(() =>
			expect(canvas.queryByText("workspace boot")).not.toBeInTheDocument(),
		);
	},
};

const [first, ...others] = WorkspaceTimingsResponse.agent_script_timings;
export const FailedScript: Story = {
	args: {
		agentScriptTimings: [
			{ ...first, status: "exit_failure", exit_code: 1 },
			...others,
		],
	},
};

// Navigate into a provisioning stage
export const NavigateToPlanStage: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const detailsButton = canvas.getByRole("button", {
			name: "View plan details",
		});
		await user.click(detailsButton);
		await canvas.findByText(
			"module.dotfiles.data.coder_parameter.dotfiles_uri[0]",
		);
	},
};

// Navigating into a workspace boot stage
export const NavigateToStartStage: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const detailsButton = canvas.getByRole("button", {
			name: "View run startup scripts details",
		});
		await user.click(detailsButton);
		await canvas.findByText("Startup Script");
	},
};

// Test case for https://github.com/coder/coder/issues/15413
export const DuplicatedScriptTiming: Story = {
	args: {
		agentScriptTimings: [
			WorkspaceTimingsResponse.agent_script_timings[0],
			{
				...WorkspaceTimingsResponse.agent_script_timings[0],
				started_at: "2021-09-01T00:00:00Z",
				ended_at: "2021-09-01T00:00:00Z",
			},
		],
	},
};

// Loading when agent script timings are empty
// Test case for https://github.com/coder/coder/issues/15273
export const LoadingWhenAgentScriptTimingsAreEmpty: Story = {
	args: {
		agentScriptTimings: undefined,
	},
};

export const MemoryLeak: Story = {
	args: {
		provisionerTimings: [
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:14.819777Z",
				ended_at: "2025-01-05T12:47:18.654211Z",
				stage: "init",
				source: "terraform",
				action: "initializing terraform",
				resource: "state file",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.003863Z",
				ended_at: "2025-01-05T12:47:19.004863Z",
				stage: "plan",
				source: "coder",
				action: "read",
				resource: "data.coder_workspace.me",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.004077Z",
				ended_at: "2025-01-05T12:47:19.004819Z",
				stage: "plan",
				source: "coder",
				action: "read",
				resource: "data.coder_provisioner.me",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.004141Z",
				ended_at: "2025-01-05T12:47:19.004989Z",
				stage: "plan",
				source: "coder",
				action: "read",
				resource: "data.coder_workspace_owner.me",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.00933Z",
				ended_at: "2025-01-05T12:47:19.030038Z",
				stage: "plan",
				source: "docker",
				action: "state refresh",
				resource: "docker_image.main",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.01011Z",
				ended_at: "2025-01-05T12:47:19.013026Z",
				stage: "plan",
				source: "coder",
				action: "state refresh",
				resource: "coder_agent.main",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.019742Z",
				ended_at: "2025-01-05T12:47:19.020578Z",
				stage: "plan",
				source: "coder",
				action: "state refresh",
				resource: "coder_app.code-server",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.056489Z",
				ended_at: "2025-01-05T12:47:19.494507Z",
				stage: "graph",
				source: "terraform",
				action: "building terraform dependency graph",
				resource: "state file",
			},
			{
				job_id: "49fe19b4-19e9-4320-8e17-1a63164453da",
				started_at: "2025-01-05T12:47:19.815179Z",
				ended_at: "2025-01-05T12:47:20.238378Z",
				stage: "apply",
				source: "docker",
				action: "create",
				resource: "docker_container.workspace[0]",
			},
		],
		agentScriptTimings: [],
		agentConnectionTimings: [
			{
				started_at: "2025-01-05T12:47:20.782132Z",
				ended_at: "2025-01-05T12:47:21.05562Z",
				stage: "connect",
				workspace_agent_id: "27941bd8-2f3b-4c0a-ad1d-46ea90cca242",
				workspace_agent_name: "main",
			},
		],
	},
};
