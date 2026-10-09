import { preloadHighlighter } from "@pierre/diffs";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
	MockACPExpiredLoginResult,
	MockACPSession,
	MockACPSessionArgs,
	MockACPSpawnArgs,
	MockACPWaitingResult,
	MockACPWaitResult,
} from "#/testHelpers/acp";
import { MockChatModel } from "#/testHelpers/chatModels";
import { ACPContext, buildACPSessionDescriptors } from "./ACPContext";
import { ACPToolNames } from "./acpToolNames";
import { Tool } from "./Tool";

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
		result: {
			messages: [
				{
					role: "assistant",
					content: [
						{
							type: "reasoning",
							text: "I will compare the response with the expected value.",
						},
					],
				},
			],
		},
	},
};
export const WaitingWithTools: Story = {
	args: { ...Connecting.args, result: MockACPWaitingResult },
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
		result: {
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
		},
	},
};
export const NoActivity: Story = {
	args: { ...Completed.args, result: { ...MockACPWaitResult, messages: [] } },
};
export const TimedOut: Story = {
	args: {
		...Completed.args,
		result: { ...MockACPWaitResult, timed_out: true, status: "running" },
	},
};
export const IncompleteHistory: Story = {
	args: {
		...Completed.args,
		result: { ...MockACPWaitResult, history_complete: false },
	},
};
export const Messaged: Story = {
	args: {
		name: ACPToolNames.MessageAgent,
		args: {
			...MockACPSessionArgs,
			message: "Also check the error handling.",
		},
	},
};
export const Interrupted: Story = {
	args: {
		name: ACPToolNames.InterruptAgent,
		args: MockACPSessionArgs,
		result: { ...MockACPSessionArgs, ...MockACPSession, interrupted: true },
	},
};
export const Failed: Story = {
	args: {
		status: "error",
		isError: true,
		result: {
			...MockACPSession,
			error: "Adapter exited. Check workspace credentials.",
		},
	},
};
export const FailedWait: Story = {
	args: {
		...Completed.args,
		status: "error",
		isError: true,
		result: { ...MockACPWaitResult, error: "Workspace agent unavailable." },
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
		args: { ...MockACPSpawnArgs, agent: { harness_id: "custom-agent" } },
		result: {
			...MockACPSession,
			harness_id: "custom-agent",
			harness_display_name: "Workspace Assistant",
		},
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
