import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn } from "storybook/test";
import {
	MockDeletedWorkspace,
	MockFailedWorkspace,
	MockOutdatedStoppedWorkspaceRequireActiveVersion,
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockWorkspace,
} from "#/testHelpers/entities";
import { DesktopPanelView, type DesktopPanelViewProps } from "./DesktopPanel";

const defaults: DesktopPanelViewProps = {
	status: "idle",
	workspace: MockWorkspace,
	agentStatus: "connected",
	onStartWorkspace: fn(),
	isStartingWorkspace: false,
	reconnect: fn(),
	attach: fn(),
	scaleMode: "native",
	onScaleModeChange: fn(),
	isControlling: false,
	onTakeControl: fn(),
	onReleaseControl: fn(),
	onPopOut: fn(),
};

const meta: Meta<typeof DesktopPanelView> = {
	title: "pages/AgentsPage/DesktopPanel",
	component: DesktopPanelView,
	args: defaults,
	decorators: [
		(Story) => (
			<div
				style={{
					height: 400,
					width: 480,
					border: "1px solid #333",
					background:
						"linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)",
				}}
			>
				<Story />
			</div>
		),
	],
	render: function RenderComponent(args) {
		const [isControlling, setIsControlling] = useState(args.isControlling);
		return (
			<DesktopPanelView
				{...args}
				isControlling={isControlling}
				onTakeControl={() => setIsControlling(true)}
				onReleaseControl={() => setIsControlling(false)}
			/>
		);
	},
};
export default meta;
type Story = StoryObj<typeof DesktopPanelView>;

export const Idle: Story = {};

export const Connecting: Story = {
	args: { status: "connecting" },
};

export const Connected: Story = {
	args: { status: "connected" },
};

export const ConnectedControlling: Story = {
	args: { status: "connected", isControlling: true },
};

export const Disconnected: Story = {
	args: { status: "disconnected" },
};

export const ErrorState: Story = {
	args: { status: "error" },
};

export const WorkspaceStopped: Story = {
	args: { workspace: MockStoppedWorkspace, agentStatus: undefined },
};

export const WorkspaceStarting: Story = {
	args: {
		workspace: MockStoppedWorkspace,
		agentStatus: undefined,
		isStartingWorkspace: true,
	},
};

export const WorkspaceStoppedRequiresUpdate: Story = {
	args: {
		workspace: MockOutdatedStoppedWorkspaceRequireActiveVersion,
		agentStatus: undefined,
	},
};

export const WorkspaceFailedStart: Story = {
	args: { workspace: MockFailedWorkspace, agentStatus: undefined },
};

export const WorkspaceFailedStop: Story = {
	args: {
		workspace: {
			...MockFailedWorkspace,
			latest_build: {
				...MockFailedWorkspace.latest_build,
				transition: "stop",
			},
		},
		agentStatus: undefined,
	},
};

export const WorkspaceBuildStarting: Story = {
	args: { workspace: MockStartingWorkspace, agentStatus: undefined },
};

export const WorkspaceDeleted: Story = {
	args: { workspace: MockDeletedWorkspace, agentStatus: undefined },
};

export const AgentConnecting: Story = {
	args: { agentStatus: "connecting" },
};
