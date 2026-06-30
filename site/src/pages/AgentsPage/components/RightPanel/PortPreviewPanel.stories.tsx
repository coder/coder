import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { PortPreviewPanel } from "./PortPreviewPanel";

const previewTab: Extract<UserRightPanelTab, { kind: "port" }> = {
	id: "port-3000",
	kind: "port",
	label: "Preview :3000",
	agentId: MockWorkspaceAgent.id,
	port: 3000,
	protocol: "http",
};

const meta = {
	title: "pages/AgentsPage/components/RightPanel/PortPreviewPanel",
	component: PortPreviewPanel,
	args: {
		workspace: MockWorkspace,
		agent: MockWorkspaceAgent,
		host: "*.apps.example.com",
		tab: previewTab,
	},
	parameters: {
		layout: "centered",
		pixel: { exclude: true },
	},
	decorators: [
		(Story) => (
			<div style={{ width: 480, height: 420 }}>
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof PortPreviewPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByTitle("Preview :3000")).toBeInTheDocument();
		await expect(canvas.getByLabelText("Open port in new tab")).toHaveAttribute(
			"href",
			expect.stringContaining("3000--"),
		);
	},
};

export const MissingWildcardHost: Story = {
	args: {
		host: "",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Port previews require a wildcard access URL."),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText("Open port in new tab")).toBeDisabled();
	},
};

export const AgentDisconnected: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "disconnected",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(
				"Port preview will be available once the workspace agent reconnects.",
			),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText("Open port in new tab")).toBeDisabled();
	},
};

export const InvalidWildcardHost: Story = {
	args: {
		// Chromium percent-encodes spaces in hosts instead of rejecting them,
		// so use a forbidden host code point that actually fails URL parsing.
		host: "bad^host",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(
				"The wildcard access URL produced an invalid preview URL. Check the deployment's wildcard access URL configuration.",
			),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText("Open port in new tab")).toBeDisabled();
	},
};
