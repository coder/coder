import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { experimentsKey } from "#/api/queries/experiments";
import { workspaceByOwnerAndNameKey } from "#/api/queries/workspaces";
import type { Workspace, WorkspaceAgentLifecycle } from "#/api/typesGenerated";
import { AuthProvider } from "#/contexts/auth/AuthProvider";
import { RequireAuth } from "#/contexts/auth/RequireAuth";
import { permissionChecks } from "#/modules/permissions";
import {
	MockAppearanceConfig,
	MockAuthMethodsAll,
	MockBuildInfo,
	MockDefaultOrganization,
	MockDeploymentConfig,
	MockEntitlements,
	MockExperiments,
	MockUserAppearanceSettings,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { withWebSocket } from "#/testHelpers/storybook";
import terminalSymbolGlyphSets from "#/theme/fonts/terminal-symbols-glyphs.json";
import { isMac } from "#/utils/platform";
import TerminalPage from "./TerminalPage";

const createWorkspaceWithAgent = (lifecycle: WorkspaceAgentLifecycle) => {
	return {
		key: workspaceByOwnerAndNameKey(
			MockWorkspace.owner_name,
			MockWorkspace.name,
		),
		data: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				resources: [
					{
						...MockWorkspace.latest_build.resources[0],
						agents: [{ ...MockWorkspaceAgent, lifecycle_state: lifecycle }],
					},
				],
			},
		} satisfies Workspace,
	};
};

const GLYPH_CHART_COLUMNS = 32;

const renderGlyphChart = () => {
	const columnHeader = Array.from(
		{ length: GLYPH_CHART_COLUMNS },
		(_, column) => column.toString(16).toUpperCase().padStart(2, "0"),
	).join(" ");
	const lines = [
		"\x1b[1mTerminal Symbols glyph chart\x1b[0m",
		// scripts/gen_terminal_symbols_font.sh leaves these out of the font.
		"Left to system fonts: \u26a1 U+26A1  \u2665 U+2665",
	];
	for (const { name, ranges } of terminalSymbolGlyphSets) {
		const included = new Set<number>();
		for (const [start, end] of ranges) {
			for (let codepoint = start; codepoint <= end; codepoint++) {
				included.add(codepoint);
			}
		}
		const rowStarts = new Set(
			[...included].map(
				(codepoint) => codepoint - (codepoint % GLYPH_CHART_COLUMNS),
			),
		);
		lines.push(
			"",
			`\x1b[1;36m${name}\x1b[0m (${included.size} glyphs)`,
			`\x1b[2m${" ".repeat(9)}${columnHeader}\x1b[0m`,
		);
		for (const rowStart of rowStarts) {
			const label = `U+${rowStart.toString(16).toUpperCase().padStart(4, "0")}`;
			let row = `\x1b[2m${label.padEnd(9)}\x1b[0m`;
			for (let column = 0; column < GLYPH_CHART_COLUMNS; column++) {
				const codepoint = rowStart + column;
				row += included.has(codepoint) ? String.fromCodePoint(codepoint) : " ";
				// Nerd Font glyphs can overflow their own cell.
				row += "  ";
			}
			lines.push(row.trimEnd());
		}
	}
	return lines.join("\r\n");
};

const meta = {
	title: "pages/Terminal",
	component: RequireAuth,
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: `@${MockWorkspace.owner_name}`,
					workspace: MockWorkspace.name,
				},
			},
			routing: reactRouterOutlet(
				{
					path: "/:username/:workspace/terminal",
				},
				<TerminalPage />,
			),
		}),
		queries: [
			{ key: ["me"], data: MockUserOwner },
			{ key: ["authMethods"], data: MockAuthMethodsAll },
			{ key: ["hasFirstUser"], data: true },
			{ key: ["buildInfo"], data: MockBuildInfo },
			{ key: ["entitlements"], data: MockEntitlements },
			{ key: experimentsKey(MockUserOwner.id), data: MockExperiments },
			{ key: ["appearance"], data: MockAppearanceConfig },
			{ key: ["organizations"], data: [MockDefaultOrganization] },
			{
				key: getAuthorizationKey({ checks: permissionChecks }),
				data: { editWorkspaceProxies: true },
			},
			{ key: ["me", "appearance"], data: MockUserAppearanceSettings },
			{
				key: ["deployment", "config"],
				data: {
					...MockDeploymentConfig,
					config: {
						...MockDeploymentConfig.config,
						web_terminal_renderer: "canvas",
					},
				},
			},
		],
	},
	decorators: [
		(Story) => (
			<AuthProvider>
				<div style={{ width: 1170, height: 880 }}>
					<Story />
				</div>
			</AuthProvider>
		),
	],
} satisfies Meta<typeof TerminalPage>;

export default meta;
type Story = StoryObj<typeof TerminalPage>;

export const Starting: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				// Copied and pasted this from browser
				data: "[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("starting")],
	},
};

export const FontFiraCode: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				// Copied and pasted this from browser
				data: "[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗",
			},
		],
		queries: [
			...meta.parameters.queries.filter(
				(q) =>
					!(
						Array.isArray(q.key) &&
						q.key[0] === "me" &&
						q.key[1] === "appearance"
					),
			),
			{
				key: ["me", "appearance"],
				data: {
					...MockUserAppearanceSettings,
					terminal_font: "fira-code",
				},
			},
			createWorkspaceWithAgent("ready"),
		],
	},
};

export const Ready: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				// Copied and pasted this from browser
				data: "[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
};

export const NerdFontGlyphs: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				// A prompt with glyphs from the Powerline, Octicons, Devicons,
				// Codicons, Font Awesome, Font Logos, and Material Design subsets,
				// which each load from their own font file.
				data: "\x1b[34m\ue0b6\x1b[30;44m \uf418 main \x1b[34;42m\ue0b0\x1b[30m \ue718 22 \x1b[32;43m\ue0b0\x1b[30m \ueb99 \uf17c \uf31b \u{f0004} \x1b[33;49m\ue0b0\x1b[0m ",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
};

export const NerdFontGlyphChart: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [{ event: "message", data: renderGlyphChart() }],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
	// xterm writes the chart over several frames and overrides earlier scrolls,
	// so wait for the scrollback to stop growing before scrolling to the top.
	play: async ({ canvasElement }) => {
		let previousScrollHeight = 0;
		const viewport = await waitFor(
			() => {
				const element = canvasElement.querySelector(".xterm-viewport");
				const scrollHeight = element?.scrollHeight ?? 0;
				const isGrowing = scrollHeight !== previousScrollHeight;
				previousScrollHeight = scrollHeight;
				if (!element || element.scrollTop === 0 || isGrowing) {
					throw new Error("terminal is still writing the chart");
				}
				return element;
			},
			{ interval: 200, timeout: 10_000 },
		);
		viewport.scrollTop = 0;
	},
};

export const RightClickMenu: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				data: "$ echo hello",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
	play: async ({ canvasElement }) => {
		if (isMac()) {
			return;
		}

		const terminal = await waitFor(() => {
			const element = canvasElement.querySelector<HTMLElement>(".xterm");
			if (!element) {
				throw new Error("terminal has not rendered yet");
			}
			return element;
		});

		const rect = terminal.getBoundingClientRect();
		terminal.dispatchEvent(
			new MouseEvent("contextmenu", {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.top + rect.height / 2,
			}),
		);

		const body = within(document.body);
		await expect(
			await body.findByRole("menuitem", { name: "Copy" }),
		).toBeInTheDocument();
		await expect(
			body.getByRole("menuitem", { name: "Paste" }),
		).toBeInTheDocument();
	},
};

export const StartError: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [],
		queries: [
			...meta.parameters.queries,
			createWorkspaceWithAgent("start_error"),
		],
	},
};

export const ConnectionError: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "error",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
};

// Check if the terminal is not getting hide when the bottom message is shown
// together with the error message
export const BottomMessage: Story = {
	decorators: [withWebSocket],

	parameters: {
		...meta.parameters,

		webSocket: [
			{
				event: "message",
				// This outputs text in the bottom left and right corners of the terminal.
				data: "\x1b[1000BLEFT\x1b[1000C\x1b[4DRIGHT",
			},
			{
				event: "close",
			},
		],

		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},

	globals: {
		viewport: {
			value: "terminal",
			isRotated: false,
		},
	},
};

export const CommandConfirmation: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: `@${MockWorkspace.owner_name}`,
					workspace: MockWorkspace.name,
				},
				searchParams: {
					command: "curl https://example.com/install.sh | bash",
				},
			},
			routing: reactRouterOutlet(
				{
					path: "/:username/:workspace/terminal",
				},
				<TerminalPage />,
			),
		}),
		webSocket: [
			{
				event: "message",
				data: "\x1b[H\x1b[2J\x1b[1m\x1b[32m\u27a4  \x1b[36mcoder\x1b[C\x1b[34mgit:(\x1b[31mbq/refactor-web-term-notifications\x1b[34m) \x1b[33m\u2717",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
};
