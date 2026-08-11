import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { API } from "#/api/api";
import {
	MockAISandboxNetworkEvents,
	MockAISandboxSession,
	MockWorkspace,
} from "#/testHelpers/entities";
import { WorkspaceAIEgressActivity } from "./WorkspaceAIEgressActivity";

const meta: Meta<typeof WorkspaceAIEgressActivity> = {
	title: "pages/WorkspacePage/WorkspaceAIEgressActivity",
	component: WorkspaceAIEgressActivity,
	args: {
		workspaceId: MockWorkspace.id,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceAIEgressActivity>;

export const NoSessions: Story = {
	beforeEach: () => {
		API.getWorkspaceAISandboxSessions = async () => [];
	},
};

export const WithAllowedAndDeniedEvents: Story = {
	beforeEach: () => {
		API.getWorkspaceAISandboxSessions = async () => [MockAISandboxSession];
		API.getAISandboxSessionNetworkEvents = async (
			_workspaceId,
			_sessionId,
			afterId = 0,
		) => (afterId === 0 ? MockAISandboxNetworkEvents : []);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("pastebin.example.com:443"),
		).toBeVisible();
		await expect(await canvas.findByText("denied")).toBeVisible();
	},
};

export const AdvisoryAttestation: Story = {
	beforeEach: () => {
		API.getWorkspaceAISandboxSessions = async () => [
			{ ...MockAISandboxSession, egress_enforcement: "advisory" },
		];
		API.getAISandboxSessionNetworkEvents = async () => [];
	},
};
