import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { API } from "#/api/api";
import {
	workspaceAISandboxActivityKey,
	workspaceAISandboxSessionsKey,
} from "#/api/queries/workspaces";
import type { AISandboxNetworkEventView } from "#/api/typesGenerated";
import {
	MockAISandboxNetworkEvent,
	MockAISandboxNetworkEvents,
	MockAISandboxSession,
	MockWorkspace,
	mockApiError,
} from "#/testHelpers/entities";
import { withWebSocket } from "#/testHelpers/storybook";
import { WorkspaceAIEgressActivity } from "./WorkspaceAIEgressActivity";

const meta: Meta<typeof WorkspaceAIEgressActivity> = {
	title: "pages/WorkspacePage/WorkspaceAIEgressActivity",
	component: WorkspaceAIEgressActivity,
	args: {
		workspaceId: MockWorkspace.id,
	},
	decorators: [withWebSocket],
	parameters: {
		webSocket: [],
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceAIEgressActivity>;

export const NoSessions: Story = {
	beforeEach: () => {
		API.getWorkspaceAISandboxSessions = async () => [];
		API.getWorkspaceAISandboxNetworkEvents = async () => [];
	},
};

export const LoadError: Story = {
	beforeEach: () => {
		API.getWorkspaceAISandboxSessions = async () => [MockAISandboxSession];
		API.getWorkspaceAISandboxNetworkEvents = async () => {
			throw mockApiError({ message: "Failed to load AI egress activity." });
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("Failed to load AI egress activity."),
		).toBeVisible();
	},
};

export const WithAllowedAndDeniedEvents: Story = {
	beforeEach: () => {
		API.getWorkspaceAISandboxSessions = async () => [MockAISandboxSession];
		API.getWorkspaceAISandboxNetworkEvents = async (
			_workspaceId,
			beforeId = 0,
		) => (beforeId === 0 ? MockAISandboxNetworkEvents : []);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("pastebin.example.com:443"),
		).toBeVisible();
		await expect(await canvas.findByText("denied")).toBeVisible();
		const rows = await canvas.findAllByRole("row");
		await expect(
			within(rows[1]).getByText("pastebin.example.com:443"),
		).toBeVisible();
		await expect(within(rows[2]).getByText("github.com:443")).toBeVisible();
	},
};

export const MultipleSessionsNewestFirst: Story = {
	beforeEach: () => {
		const olderSession = {
			...MockAISandboxSession,
			id: "2f3e4d5c-6b7a-8901-a293-b4c5d6e7f809",
			started_at: "2024-05-28T11:00:00Z",
		};
		const newestEvent: AISandboxNetworkEventView = {
			...MockAISandboxNetworkEvent,
			id: 3,
			session_id: olderSession.id,
			occurred_at: "2024-05-28T12:00:12Z",
			host: "registry.example.com",
		};
		API.getWorkspaceAISandboxSessions = async () => [
			MockAISandboxSession,
			olderSession,
		];
		API.getWorkspaceAISandboxNetworkEvents = async () => [
			newestEvent,
			...MockAISandboxNetworkEvents,
		];
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("registry.example.com:443");
		await expect(canvas.getAllByRole("table")).toHaveLength(1);
		const rows = canvas.getAllByRole("row");
		await expect(
			within(rows[1]).getByText("registry.example.com:443"),
		).toBeVisible();
		await expect(
			canvas.queryByText("All egress is routed through the platform proxy."),
		).not.toBeInTheDocument();
	},
};

export const ReloadsFromWebSocket: Story = {
	parameters: {
		webSocket: [{ event: "message", data: "{}" }],
		queries: [
			{
				key: workspaceAISandboxSessionsKey(MockWorkspace.id),
				data: [MockAISandboxSession],
			},
			{
				key: workspaceAISandboxActivityKey(MockWorkspace.id),
				data: { pages: [MockAISandboxNetworkEvents], pageParams: [0] },
			},
		],
	},
	beforeEach: () => {
		const latestEvent: AISandboxNetworkEventView = {
			...MockAISandboxNetworkEvent,
			id: 3,
			occurred_at: "2024-05-28T12:00:12Z",
			host: "registry.example.com",
		};
		API.getWorkspaceAISandboxSessions = async () => [MockAISandboxSession];
		API.getWorkspaceAISandboxNetworkEvents = async (
			_workspaceId,
			beforeId = 0,
		) => {
			if (beforeId > 0) {
				return [];
			}
			return [latestEvent, ...MockAISandboxNetworkEvents];
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("registry.example.com:443"),
		).toBeVisible();
	},
};

export const AdvisoryAttestation: Story = {
	beforeEach: () => {
		API.getWorkspaceAISandboxSessions = async () => [
			{ ...MockAISandboxSession, egress_enforcement: "advisory" },
		];
		API.getWorkspaceAISandboxNetworkEvents = async () => [];
	},
};
