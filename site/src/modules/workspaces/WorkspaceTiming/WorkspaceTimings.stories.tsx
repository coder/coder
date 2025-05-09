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

export const LongTimeRange = {
	args: {
		provisionerTimings: [
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				started_at: "2021-09-01T00:00:00Z",
				ended_at: "2021-09-01T00:10:00Z",
			},
		],
		agentConnectionTimings: [
			{
				...WorkspaceTimingsResponse.agent_connection_timings[0],
				started_at: "2021-09-01T00:10:00Z",
				ended_at: "2021-09-01T00:35:00Z",
			},
		],
		agentScriptTimings: [
			{
				...WorkspaceTimingsResponse.agent_script_timings[0],
				started_at: "2021-09-01T00:35:00Z",
				ended_at: "2021-09-01T01:00:00Z",
			},
		],
	},
};

// We want to gracefully handle the case when the action is added in the BE but
// not in the FE. This is a temporary fix until we can have strongly provisioner
// timing action types in the BE.
export const MissedAction: Story = {
	args: {
		agentConnectionTimings: [
			{
				ended_at: "2025-03-12T18:15:13.651163Z",
				stage: "connect",
				started_at: "2025-03-12T18:15:10.249068Z",
				workspace_agent_id: "41ab4fd4-44f8-4f3a-bb69-262ae85fba0b",
				workspace_agent_name: "Interface",
			},
		],
		agentScriptTimings: [
			{
				display_name: "Startup Script",
				ended_at: "2025-03-12T18:16:44.771508Z",
				exit_code: 0,
				stage: "start",
				started_at: "2025-03-12T18:15:13.847336Z",
				status: "ok",
				workspace_agent_id: "41ab4fd4-44f8-4f3a-bb69-262ae85fba0b",
				workspace_agent_name: "Interface",
			},
		],
		provisionerTimings: [
			{
				action: "create",
				ended_at: "2025-03-12T18:08:07.402358Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "coder_agent.Interface",
				source: "coder",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.194957Z",
			},
			{
				action: "create",
				ended_at: "2025-03-12T18:08:08.029908Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "null_resource.validate_url",
				source: "null",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.399387Z",
			},
			{
				action: "create",
				ended_at: "2025-03-12T18:08:07.440785Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "module.emu_host.random_id.emulator_host_id",
				source: "random",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.403171Z",
			},
			{
				action: "missed action",
				ended_at: "2025-03-12T18:08:08.029752Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "null_resource.validate_url",
				source: "null",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.410219Z",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const applyButton = canvas.getByRole("button", {
			name: "View apply details",
		});
		await user.click(applyButton);
		await canvas.findByText("missed action");
	},
};
