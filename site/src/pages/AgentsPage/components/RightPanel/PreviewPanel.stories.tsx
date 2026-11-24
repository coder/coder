import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import { MockPreviewAddressResponse } from "#/testHelpers/chatEntities";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	mockApiError,
} from "#/testHelpers/entities";
import { withProxyProvider } from "#/testHelpers/storybook";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { PreviewPanel } from "./PreviewPanel";

const mockPreviewTab: Extract<UserRightPanelTab, { kind: "preview" }> = {
	id: "preview-docs",
	kind: "preview",
	label: "Docs site",
	workspaceId: MockWorkspace.id,
	agentName: MockWorkspaceAgent.name,
	slug: "preview--3000",
};

const meta = {
	title: "pages/AgentsPage/components/RightPanel/PreviewPanel",
	component: PreviewPanel,
	args: {
		agent: MockWorkspaceAgent,
		tab: mockPreviewTab,
		reloadCount: 0,
	},
	parameters: {
		layout: "centered",
	},
	decorators: [
		withProxyProvider(),
		(Story) => (
			<div style={{ width: 480, height: 420 }}>
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		// A blank page keeps the screenshot independent of the network.
		spyOn(API.experimental, "getPreviewAddress").mockResolvedValue({
			...MockPreviewAddressResponse,
			url: "about:blank",
		});
	},
} satisfies Meta<typeof PreviewPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getPreviewAddress").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const LoadFailed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getPreviewAddress").mockRejectedValue(
			mockApiError({
				message:
					"Resource not found or you do not have access to this resource",
			}),
		);
	},
};

export const AgentDisconnected: Story = {
	args: {
		agent: { ...MockWorkspaceAgent, status: "disconnected" },
	},
};
