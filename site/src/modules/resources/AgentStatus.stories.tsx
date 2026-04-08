import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { MockWorkspaceAgent } from "#/testHelpers/entities";
import {
	agentConnectionMessages,
	agentScriptMessages,
} from "../workspaces/health";
import { AgentStatus } from "./AgentStatus";

const meta: Meta<typeof AgentStatus> = {
	title: "modules/resources/AgentStatus",
	component: AgentStatus,
};

export default meta;
type Story = StoryObj<typeof AgentStatus>;

/**
 * Shared play helper that hovers the status icon, then asserts the
 * tooltip contains the expected title and detail text, plus a
 * troubleshoot link when the agent has a troubleshooting URL.
 */
async function expectTooltip(
	ariaLabel: string,
	title: string,
	detail: string,
	hasTroubleshootLink: boolean,
) {
	const icon = screen.getByRole("status", { name: ariaLabel });
	await userEvent.click(icon);
	await waitFor(() => {
		const tooltip = screen.getByRole("dialog");
		expect(tooltip).toHaveTextContent(title);
		expect(tooltip).toHaveTextContent(detail);
		if (hasTroubleshootLink) {
			expect(
				within(tooltip).getByRole("link", { name: "Troubleshoot" }),
			).toBeInTheDocument();
		} else {
			expect(
				within(tooltip).queryByRole("link", { name: "Troubleshoot" }),
			).not.toBeInTheDocument();
		}
	});
}

export const Ready: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connected",
			lifecycle_state: "ready",
		},
	},
};

export const StartupScriptFailed: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connected",
			lifecycle_state: "start_error",
		},
	},
	play: async () => {
		await expectTooltip(
			"Startup script failed",
			agentScriptMessages.start_error.title,
			agentScriptMessages.start_error.detail,
			true,
		);
	},
};

export const StartupScriptTimeout: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connected",
			lifecycle_state: "start_timeout",
		},
	},
	play: async () => {
		await expectTooltip(
			"Startup script timeout",
			agentScriptMessages.start_timeout.title,
			agentScriptMessages.start_timeout.detail,
			true,
		);
	},
};

export const ShutdownScriptFailed: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connected",
			lifecycle_state: "shutdown_error",
		},
	},
	play: async () => {
		await expectTooltip(
			"Shutdown script failed",
			agentScriptMessages.shutdown_error.title,
			agentScriptMessages.shutdown_error.detail,
			true,
		);
	},
};

export const ShutdownScriptTimeout: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connected",
			lifecycle_state: "shutdown_timeout",
		},
	},
	play: async () => {
		await expectTooltip(
			"Shutdown script timeout",
			agentScriptMessages.shutdown_timeout.title,
			agentScriptMessages.shutdown_timeout.detail,
			true,
		);
	},
};

export const ConnectionTimeout: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "timeout",
		},
	},
	play: async () => {
		await expectTooltip(
			"Timeout",
			agentConnectionMessages.timeout.title,
			agentConnectionMessages.timeout.detail,
			true,
		);
	},
};

export const StartupScriptFailedNoTroubleshootURL: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connected",
			lifecycle_state: "start_error",
			troubleshooting_url: "",
		},
	},
	play: async () => {
		await expectTooltip(
			"Startup script failed",
			agentScriptMessages.start_error.title,
			agentScriptMessages.start_error.detail,
			false,
		);
	},
};

export const Disconnected: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "disconnected",
		},
	},
};

export const Connecting: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
		},
	},
};
