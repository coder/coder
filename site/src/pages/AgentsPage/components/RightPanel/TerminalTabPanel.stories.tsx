import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import { deploymentConfigQueryKey } from "#/api/queries/deployment";
import { myAppearanceKey } from "#/api/queries/users";
import {
	MockDeploymentConfig,
	MockUserAppearanceSettings,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { withProxyProvider, withWebSocket } from "#/testHelpers/storybook";
import { TerminalClientSessionContext } from "../../context/TerminalClientSessionContext";
import { type TerminalChip, TerminalTabPanel } from "./TerminalTabPanel";

const terminalQueries = [
	{
		key: deploymentConfigQueryKey,
		data: {
			...MockDeploymentConfig,
			config: {
				...MockDeploymentConfig.config,
				web_terminal_renderer: "canvas",
			},
		},
	},
	{ key: myAppearanceKey, data: MockUserAppearanceSettings },
];

const promptMessage =
	"\u001b[H\u001b[2J\u001b[1m\u001b[32m➜  \u001b[36mcoder\u001b[C\u001b[34mgit:(\u001b[31mmain\u001b[34m) \u001b[33m✗";

const terminals: TerminalChip[] = [
	{ id: "terminal", label: "Terminal 1", reconnectionToken: "chat-id" },
	{ id: "terminal-2", label: "Terminal 2", reconnectionToken: "token-2" },
	{
		id: "terminal-claude",
		label: "Claude Code",
		reconnectionToken: "token-3",
		initialCommand: "claude",
	},
];

const meta = {
	title: "pages/AgentsPage/TerminalTabPanel",
	component: TerminalTabPanel,
	args: {
		chatId: "b5a8832c-72db-4679-8393-9a48dff20a20",
		workspace: MockWorkspace,
		workspaceAgent: MockWorkspaceAgent,
		terminals,
		activeTerminalId: "terminal-2",
		pendingTerminalId: null,
		isVisible: true,
		canCreateTerminal: true,
		onActiveTerminalChange: fn(),
		onCloseTerminal: fn(),
		onNewTerminal: fn(),
		onTerminalReady: fn(),
	},
	parameters: {
		queries: terminalQueries,
		webSocket: [{ event: "message", data: promptMessage }],
	},
	decorators: [
		withProxyProvider(),
		withWebSocket,
		(Story) => (
			<TerminalClientSessionContext value="0123456789abcdef0123456789abcdef">
				<div style={{ width: 480, height: 600 }}>
					<Story />
				</div>
			</TerminalClientSessionContext>
		),
	],
} satisfies Meta<typeof TerminalTabPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const MultipleTerminals: Story = {
	// The xterm canvas paints asynchronously, so the screenshot is not stable.
	// SubTabStrip.test.tsx covers chip selection and close behavior.
	parameters: { pixel: { exclude: true } },
};

export const EmptyState: Story = {
	args: {
		terminals: [],
		activeTerminalId: null,
	},
};

export const EmptyStateWorkspaceStopped: Story = {
	args: {
		terminals: [],
		activeTerminalId: null,
		canCreateTerminal: false,
	},
};

/** Closing the active chip selects its neighbor, matching the page's handler. */
export const CloseActiveChip: Story = {
	parameters: { pixel: { exclude: true } },
	render: function CloseActiveChip(args) {
		const [openTerminals, setOpenTerminals] = useState(terminals);
		const [activeId, setActiveId] = useState<string | null>("terminal-2");
		const handleClose = (id: string) => {
			const ids = openTerminals.map((terminal) => terminal.id);
			const remaining = ids.filter((terminalId) => terminalId !== id);
			setOpenTerminals(openTerminals.filter((terminal) => terminal.id !== id));
			if (activeId === id) {
				setActiveId(
					remaining[Math.min(ids.indexOf(id), remaining.length - 1)] ?? null,
				);
			}
		};
		return (
			<TerminalTabPanel
				{...args}
				terminals={openTerminals}
				activeTerminalId={activeId}
				onActiveTerminalChange={setActiveId}
				onCloseTerminal={handleClose}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Close Terminal 2" }),
		);
	},
};
