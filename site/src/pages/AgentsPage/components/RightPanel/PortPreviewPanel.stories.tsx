import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { AnnotatorToHostMessage } from "#/annotator/protocol";
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
		// Live port previews embed a cross-origin iframe that Pixel cannot capture
		// deterministically; submission behavior is covered in PortPreviewPanel.test.tsx.
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

export const Ready: Story = {};

export const MissingWildcardHost: Story = {
	args: { host: "" },
	parameters: { pixel: { exclude: false } },
};

export const AgentDisconnected: Story = {
	args: { agent: { ...MockWorkspaceAgent, status: "disconnected" } },
	parameters: { pixel: { exclude: false } },
};

export const InvalidWildcardHost: Story = {
	args: {
		// Chromium percent-encodes spaces in hosts instead of rejecting them,
		// so use a forbidden host code point that actually fails URL parsing.
		host: "bad^host",
	},
	parameters: { pixel: { exclude: false } },
};

// What the overlay inside the preview would post to the dashboard.
function postFromPreview(
	canvasElement: HTMLElement,
	data: AnnotatorToHostMessage,
) {
	const frame =
		within(canvasElement).getByTitle<HTMLIFrameElement>("Preview :3000");
	window.dispatchEvent(
		new MessageEvent("message", {
			data,
			origin: new URL(frame.src).origin,
			source: frame.contentWindow,
		}),
	);
}

export const CanAnnotate: Story = {
	args: { canAnnotate: true, onSendToolMessage: async () => "sent" },
};

export const AnnotatePicking: Story = {
	args: { canAnnotate: true, onSendToolMessage: async () => "sent" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Annotate elements" }),
		);
		postFromPreview(canvasElement, { type: "coder-annotator:ready" });
		postFromPreview(canvasElement, {
			type: "coder-annotator:state",
			picking: true,
		});
	},
};

export const AnnotateUnavailable: Story = {
	args: {
		canAnnotate: true,
		annotatorReadyTimeoutMs: 0,
		onSendToolMessage: async () => "sent",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Annotate elements" }),
		);
		canvas
			.getByTitle<HTMLIFrameElement>("Preview :3000")
			.dispatchEvent(new Event("load"));
		await new Promise((resolve) => setTimeout(resolve, 50));
	},
};
