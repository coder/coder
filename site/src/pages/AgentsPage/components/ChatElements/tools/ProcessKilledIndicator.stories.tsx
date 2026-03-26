import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Tool } from "./Tool";

const PROCESS_ID = "376b2458-e318-4442-8b87-51a0f9727f0e";

const meta: Meta<typeof Tool> = {
	title: "components/ai-elements/tool/ProcessKilledIndicator",
	component: Tool,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof Tool>;

// ---------------------------------------------------------------------------
// Execute tool — killed indicator via killedBySignal prop
// ---------------------------------------------------------------------------

export const ExecuteKilled: Story = {
	args: {
		name: "execute",
		status: "completed",
		killedBySignal: "kill",
		args: { command: "make pre-push 2>&1" },
		result: {
			success: true,
			output: "pre-push (/tmp/coder-pre-push.CZ6K9A)\ntest + build site:",
			exit_code: -1,
			wall_duration_ms: 45000,
			background_process_id: PROCESS_ID,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("make pre-push 2>&1")).toBeInTheDocument();
	},
};

export const ExecuteTerminated: Story = {
	args: {
		name: "execute",
		status: "completed",
		killedBySignal: "terminate",
		args: { command: "npm start" },
		result: {
			success: true,
			output: "Starting dev server...",
			exit_code: 0,
			wall_duration_ms: 2000,
			background_process_id: PROCESS_ID,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("npm start")).toBeInTheDocument();
	},
};

/** Execute NOT signaled — no indicator. */
export const ExecuteNotSignaled: Story = {
	args: {
		name: "execute",
		status: "completed",
		args: { command: "echo hello" },
		result: {
			success: true,
			output: "hello",
			exit_code: 0,
			wall_duration_ms: 100,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("echo hello")).toBeInTheDocument();
	},
};

/** Running execute — killed indicator should NOT appear yet. */
export const ExecuteRunningNotYetKilled: Story = {
	args: {
		name: "execute",
		status: "running",
		killedBySignal: "kill",
		args: { command: "make pre-push 2>&1" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("make pre-push 2>&1")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// ProcessOutput tool — killed indicator
// ---------------------------------------------------------------------------

export const ProcessOutputKilled: Story = {
	args: {
		name: "process_output",
		status: "completed",
		killedBySignal: "kill",
		args: { process_id: PROCESS_ID },
		result: {
			output: "pre-push (/tmp/coder-pre-push.CZ6K9A)\ntest + build site:",
			exit_code: null,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/pre-push/)).toBeInTheDocument();
	},
};

export const ProcessOutputTerminated: Story = {
	args: {
		name: "process_output",
		status: "completed",
		killedBySignal: "terminate",
		args: { process_id: PROCESS_ID },
		result: {
			output: "server output",
			exit_code: null,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("server output")).toBeInTheDocument();
	},
};

/** ProcessOutput with no output and killed — indicator in empty state. */
export const ProcessOutputKilledNoOutput: Story = {
	args: {
		name: "process_output",
		status: "completed",
		killedBySignal: "kill",
		args: { process_id: PROCESS_ID },
		result: {
			output: "",
			exit_code: null,
		},
	},
};

/** ProcessOutput NOT signaled. */
export const ProcessOutputNotSignaled: Story = {
	args: {
		name: "process_output",
		status: "completed",
		args: { process_id: PROCESS_ID },
		result: {
			output: "some output",
			exit_code: 0,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("some output")).toBeInTheDocument();
	},
};
