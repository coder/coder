import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { acpSession, acpSessionPath } from "#/api/queries/acp";
import { MockACPSession } from "#/testHelpers/acp";
import { withWebSocket } from "#/testHelpers/storybook";
import { ACPTool } from "./ACPTool";
import { Tool } from "./Tool";

const session = MockACPSession;
const path = acpSessionPath(
	session.parent_chat_id,
	session.workspace_agent_id,
	session.session_id,
);
const meta: Meta<typeof ACPTool> = {
	title: "pages/AgentsPage/ChatElements/tools/ACPTool",
	component: ACPTool,
	decorators: [withWebSocket],
	args: {
		ToolComponent: Tool,
		name: "acp_spawn_agent",
		status: "completed",
		args: {
			agent: "claude_code",
			prompt: "Find why the unit tests are failing.",
		},
		result: session,
		isError: false,
	},
	parameters: {
		webSocket: [{ event: "message", data: JSON.stringify(session) }],
		reactRouter: reactRouterParameters({
			routing: { path: "/agents/:agentId" },
			location: { pathParams: { agentId: session.parent_chat_id } },
		}),
		queries: [{ key: acpSession(path).queryKey, data: session }],
	},
};
export default meta;
type Story = StoryObj<typeof ACPTool>;
export const Spawned: Story = {
	args: {
		result: {
			...session,
			report: "I found a mismatch in the expected response.",
		},
	},
};
export const Waiting: Story = {
	args: {
		name: "acp_wait_agent",
		status: "running",
		args: {
			session_id: session.session_id,
			workspace_agent_id: session.workspace_agent_id,
		},
		result: undefined,
	},
};
export const Completed: Story = {
	args: {
		name: "acp_wait_agent",
		result: {
			...session,
			status: "waiting",
			entries: session.entries
				.filter((entry) => entry.role === "assistant")
				.map((entry) => ({ ...entry, status: "completed" })),
			report: "Updated the response expectation. All affected tests pass.",
		},
	},
};
export const CompletedLongOutput: Story = {
	args: {
		name: "acp_wait_agent",
		result: {
			...session,
			status: "waiting",
			report: Array.from(
				{ length: 20 },
				(_, index) =>
					`Step ${index + 1}: Updated the response expectation and verified the affected tests pass.`,
			).join("\n\n"),
		},
	},
};
export const Messaged: Story = {
	args: {
		name: "acp_message_agent",
		args: { ...session, message: "Also check the error handling." },
	},
};
export const Interrupted: Story = {
	args: { name: "acp_interrupt_agent" },
};
export const Failed: Story = {
	args: {
		status: "error",
		isError: true,
		result: {
			...session,
			error: "Adapter exited. Check workspace credentials.",
		},
	},
};
export const Expired: Story = {
	args: Waiting.args,
	parameters: { queries: [{ key: acpSession(path).queryKey, data: null }] },
};
export const Listed: Story = {
	args: {
		name: "acp_list_agents",
		result: {
			agents: [
				session,
				{
					...session,
					session_id: "second",
					agent: "codex",
					title: "Review changes",
				},
			],
		},
	},
};

export const SpawnedThenWaited: Story = {
	render: (args) => (
		<div className="flex flex-col gap-2">
			<ACPTool {...args} />
			<ACPTool
				{...args}
				name="acp_wait_agent"
				result={{ ...session, report: "All affected tests pass." }}
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
