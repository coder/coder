import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { acpSession, acpSessionPath } from "#/api/queries/acp";
import { chat } from "#/api/queries/chats";
import { MockACPSession } from "#/testHelpers/acp";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import ACPChatPage from "./ACPChatPage";

const session = MockACPSession;
const path = acpSessionPath(
	session.parent_chat_id,
	session.workspace_agent_id,
	session.session_id,
);
const meta: Meta<typeof ACPChatPage> = {
	title: "pages/AgentsPage/ACPChatPage",
	component: ACPChatPage,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withWebSocket,
		(Story) => (
			<div className="flex h-screen">
				<Story />
			</div>
		),
	],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		webSocket: [{ event: "message", data: JSON.stringify(session) }],
		reactRouter: reactRouterParameters({
			routing: { path: "/agents/:agentId/acp/:workspaceAgentId/:sessionId" },
			location: {
				pathParams: {
					agentId: session.parent_chat_id,
					workspaceAgentId: session.workspace_agent_id,
					sessionId: session.session_id,
				},
			},
		}),
		queries: [
			{ key: acpSession(path).queryKey, data: session },
			{
				key: chat(session.parent_chat_id).queryKey,
				data: { ...MockChat, id: session.parent_chat_id, title: "Fix tests" },
			},
		],
	},
};
export default meta;
type Story = StoryObj<typeof ACPChatPage>;
export const Running: Story = {};
export const Idle: Story = {
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify({
					...session,
					version: 4,
					status: "waiting",
					entries: session.entries.map((entry) =>
						entry.kind === "tool"
							? { ...entry, status: "completed", text: "Tests passed" }
							: entry,
					),
				}),
			},
		],
	},
};
export const Failed: Story = {
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify({
					...session,
					version: 4,
					status: "error",
					error: "Adapter exited. Spawn a new agent to continue.",
				}),
			},
		],
	},
};
export const Expired: Story = {
	parameters: {
		webSocket: [],
		queries: [
			{ key: acpSession(path).queryKey, data: null },
			{
				key: chat(session.parent_chat_id).queryKey,
				data: { ...MockChat, id: session.parent_chat_id, title: "Fix tests" },
			},
		],
	},
};
