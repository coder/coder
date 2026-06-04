import type { Meta, StoryObj } from "@storybook/react-vite";
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
	source: "listening",
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
		chromatic: { disableSnapshot: true },
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

export const Ready: Story = {};

export const MissingWildcardHost: Story = {
	args: {
		host: "",
	},
};

export const AgentDisconnected: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "disconnected",
		},
	},
};

export const InvalidWildcardHost: Story = {
	args: {
		host: "bad host",
	},
};
