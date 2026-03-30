import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Tool } from "./Tool";

const PROCESS_ID = "376b2458-e318-4442-8b87-51a0f9727f0e";

const meta: Meta<typeof Tool> = {
	title: "components/ai-elements/tool/ProcessSignal",
	component: Tool,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: { name: "process_signal" },
};
export default meta;
type Story = StoryObj<typeof Tool>;

// ---------------------------------------------------------------------------
// Running states
// ---------------------------------------------------------------------------

export const RunningKill: Story = {
	args: {
		status: "running",
		args: { process_id: PROCESS_ID, signal: "kill" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Killing process…")).toBeInTheDocument();
	},
};

export const RunningTerminate: Story = {
	args: {
		status: "running",
		args: { process_id: PROCESS_ID, signal: "terminate" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Terminating process…")).toBeInTheDocument();
	},
};

export const RunningUnknownSignal: Story = {
	args: {
		status: "running",
		args: { process_id: PROCESS_ID, signal: "" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Sending signal…")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Success states
// ---------------------------------------------------------------------------

export const SuccessKill: Story = {
	args: {
		status: "completed",
		args: { process_id: PROCESS_ID, signal: "kill" },
		result: {
			success: true,
			message: `signal "kill" sent to process ${PROCESS_ID}`,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Killed process 376b2458")).toBeInTheDocument();
	},
};

export const SuccessTerminate: Story = {
	args: {
		status: "completed",
		args: { process_id: PROCESS_ID, signal: "terminate" },
		result: {
			success: true,
			message: `signal "terminate" sent to process ${PROCESS_ID}`,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Terminated process 376b2458")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Failure states
// ---------------------------------------------------------------------------

/**
 * Backend errorResult() wraps failures in NewTextResponse (not
 * NewTextErrorResponse), so isError stays false. The renderer
 * detects this via the success=false field.
 */
export const SoftFailureKill: Story = {
	args: {
		status: "completed",
		args: { process_id: PROCESS_ID, signal: "kill" },
		result: {
			success: false,
			error: "signal process: process not found",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Failed to kill process 376b2458"),
		).toBeInTheDocument();
	},
};

export const SoftFailureTerminate: Story = {
	args: {
		status: "completed",
		args: { process_id: PROCESS_ID, signal: "terminate" },
		result: {
			success: false,
			error: "signal process: process not found",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Failed to terminate process 376b2458"),
		).toBeInTheDocument();
	},
};

/**
 * Protocol-level error from NewTextErrorResponse (e.g. missing
 * args). isError is true, result is a plain string.
 */
export const ProtocolError: Story = {
	args: {
		status: "completed",
		isError: true,
		args: { process_id: "", signal: "kill" },
		result: "process_id is required",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Failed to kill process")).toBeInTheDocument();
	},
};

/**
 * Protocol-level error with a structured result body.
 */
export const ProtocolErrorStructured: Story = {
	args: {
		status: "completed",
		isError: true,
		args: { process_id: PROCESS_ID, signal: "terminate" },
		result: {
			success: false,
			error: "workspace connection resolver is not configured",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Failed to terminate process 376b2458"),
		).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

/** No args parsed yet (streamed tool call with partial data). */
export const NoArgs: Story = {
	args: {
		status: "running",
		args: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Sending signal…")).toBeInTheDocument();
	},
};
