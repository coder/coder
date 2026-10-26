import { preloadHighlighter } from "@pierre/diffs";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
	MockACPExpiredLoginResult,
	MockACPSession,
	MockACPSessionArgs,
	MockACPSpawnArgs,
	MockACPTranscriptDeltas,
	MockACPWaitResult,
} from "#/testHelpers/acp";
import { MockChatModel } from "#/testHelpers/chatModels";
import { ACPContext, buildACPSessionDescriptors } from "./ACPContext";
import { ACPToolNames } from "./acpToolNames";
import { mergeACPTranscriptDelta } from "./acpTranscript";
import { Tool } from "./Tool";

const mockLongWaitResult = {
	...MockACPWaitResult,
	messages: [
		{
			role: "assistant",
			content: [
				{
					type: "text",
					text: Array.from(
						{ length: 20 },
						(_, index) =>
							`Step ${index + 1}: Updated the response expectation and verified the affected tests pass.`,
					).join("\n\n"),
				},
			],
		},
	],
};

const mockEmptyWaitResult = { ...MockACPWaitResult, messages: [] };

const mockTimedOutWaitResult = {
	...MockACPWaitResult,
	timed_out: true,
	status: "running",
};

const mockMessageArgs = {
	...MockACPSessionArgs,
	message: "Also check the error handling.",
};

const mockInterruptedSession = {
	...MockACPSessionArgs,
	...MockACPSession,
	interrupted: true,
};

const mockFailedSession = {
	...MockACPSession,
	error: "Adapter exited. Check workspace credentials.",
};

const mockFailedWaitResult = {
	...MockACPWaitResult,
	error: "Workspace agent unavailable.",
};

const mockCustomSpawnArgs = {
	...MockACPSpawnArgs,
	agent: { harness_id: "custom-agent" },
};

const mockCustomSession = {
	...MockACPSession,
	harness_id: "custom-agent",
	harness_display_name: "Workspace Assistant",
};

const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/ACPTool",
	component: Tool,
	// Preload the fallback highlighter so StrictMode remounts preserve output.
	loaders: [
		async () => {
			await preloadHighlighter({
				themes: ["github-dark-high-contrast", "github-light"],
				langs: ["json"],
			});
		},
	],
	decorators: [
		(Story) => (
			<ACPContext
				value={buildACPSessionDescriptors([
					{
						id: "spawn",
						name: ACPToolNames.SpawnAgent,
						status: "completed",
						isError: false,
						args: MockACPSpawnArgs,
						result: MockACPSession,
					},
				])}
			>
				<Story />
			</ACPContext>
		),
	],
	args: {
		organizationId: MockChatModel.organization_id,
		mcpServers: [],
		subagentTitles: new Map(),
		subagentVariants: new Map(),
		shellToolDisplayMode: "auto",
		codeDiffDisplayMode: "auto",
		name: ACPToolNames.SpawnAgent,
		status: "completed",
		args: MockACPSpawnArgs,
		result: MockACPSession,
		isError: false,
	},
};
export default meta;
type Story = StoryObj<typeof Tool>;

export const Spawned: Story = {};
export const Spawning: Story = {
	args: { status: "running", result: undefined },
};
export const Connecting: Story = {
	args: {
		name: ACPToolNames.WaitAgent,
		status: "running",
		args: MockACPSessionArgs,
		result: undefined,
	},
};
export const Waiting: Story = {
	args: {
		...Connecting.args,
		result: mergeACPTranscriptDelta(
			undefined,
			undefined,
			MockACPTranscriptDeltas.slice(0, 1).join(""),
		).value,
	},
};
export const WaitingWithTools: Story = {
	args: {
		...Connecting.args,
		result: mergeACPTranscriptDelta(
			undefined,
			undefined,
			MockACPTranscriptDeltas.slice(0, 3).join(""),
		).value,
	},
};
export const WaitingWithCompletedTools: Story = {
	args: {
		...Connecting.args,
		result: mergeACPTranscriptDelta(
			undefined,
			undefined,
			MockACPTranscriptDeltas.join(""),
		).value,
	},
};
export const Completed: Story = {
	args: {
		name: ACPToolNames.WaitAgent,
		args: MockACPSessionArgs,
		result: MockACPWaitResult,
	},
};
export const CompletedLongOutput: Story = {
	args: {
		...Completed.args,
		result: mockLongWaitResult,
	},
};
export const NoActivity: Story = {
	args: { ...Completed.args, result: mockEmptyWaitResult },
};
export const TimedOut: Story = {
	args: {
		...Completed.args,
		result: mockTimedOutWaitResult,
	},
};
export const Messaged: Story = {
	args: {
		name: ACPToolNames.MessageAgent,
		args: mockMessageArgs,
	},
};
export const Interrupted: Story = {
	args: {
		name: ACPToolNames.InterruptAgent,
		args: MockACPSessionArgs,
		result: mockInterruptedSession,
	},
};
export const Failed: Story = {
	args: {
		status: "error",
		isError: true,
		result: mockFailedSession,
	},
};
export const FailedWait: Story = {
	args: {
		...Completed.args,
		status: "error",
		isError: true,
		result: mockFailedWaitResult,
	},
};

export const ExpiredLogin: Story = {
	args: {
		...Completed.args,
		status: "error",
		isError: true,
		result: MockACPExpiredLoginResult,
	},
};

export const CustomHarness: Story = {
	args: {
		args: mockCustomSpawnArgs,
		result: mockCustomSession,
	},
};
export const Mobile: Story = {
	...Completed,
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
export const SpawnedThenWaited: Story = {
	render: (args) => (
		<div className="flex flex-col gap-2">
			<Tool {...args} />
			<Tool
				{...args}
				name={ACPToolNames.WaitAgent}
				args={MockACPSessionArgs}
				result={MockACPWaitResult}
			/>
		</div>
	),
};
export const CollapsedSpawnedThenWaited: Story = {
	render: SpawnedThenWaited.render,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /^Spawned/ }));
		await userEvent.click(canvas.getByRole("button", { name: /^Waited for/ }));
	},
};
export const KeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: /^Spawned/ })
			.focus();
	},
};
