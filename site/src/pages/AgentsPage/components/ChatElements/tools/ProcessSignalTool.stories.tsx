import type { Meta, StoryObj } from "@storybook/react-vite";
import { Tool } from "./Tool";

const PROCESS_ID = "376b2458-e318-4442-8b87-51a0f9727f0e";

const meta: Meta<typeof Tool> = {
	title: "components/ai-elements/tool/ProcessSignal",
	component: Tool,
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
};

export const RunningTerminate: Story = {
	args: {
		status: "running",
		args: { process_id: PROCESS_ID, signal: "terminate" },
	},
};

export const RunningUnknownSignal: Story = {
	args: {
		status: "running",
		args: { process_id: PROCESS_ID, signal: "" },
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
};

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------
