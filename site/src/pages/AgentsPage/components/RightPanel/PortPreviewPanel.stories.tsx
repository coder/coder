import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, userEvent, within } from "storybook/test";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import {
	ComposerProvider,
	useRegisterComposer,
} from "../../context/ComposerContext";
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

const Composer: FC = () => {
	useRegisterComposer({ send: () => undefined });
	return null;
};

const withComposer: Decorator = (Story) => (
	<ComposerProvider>
		<Composer />
		<Story />
	</ComposerProvider>
);

export const CanAnnotate: Story = {
	args: { canAnnotate: true },
	decorators: [withComposer],
};

export const AnnotatePicking: Story = {
	args: { canAnnotate: true },
	decorators: [withComposer],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Annotate elements" }),
		);
		const frame = canvas.getByTitle<HTMLIFrameElement>("Preview :3000");
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "coder-annotator:state", picking: true },
				origin: new URL(frame.src).origin,
				source: frame.contentWindow,
			}),
		);
	},
};

export const AnnotateUnavailable: Story = {
	args: { canAnnotate: true, annotatorReadyTimeoutMs: 0 },
	decorators: [withComposer],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Annotate elements" }),
		);
		canvas
			.getByTitle<HTMLIFrameElement>("Preview :3000")
			.dispatchEvent(new Event("load"));
		await new Promise((resolve) => setTimeout(resolve, 50));
		// The disabled button has pointer-events: none; its wrapper span is
		// the tooltip trigger.
		const wrapper = canvas.getByRole("button", {
			name: "Annotate elements",
		}).parentElement;
		if (wrapper) {
			await userEvent.hover(wrapper);
		}
	},
};
