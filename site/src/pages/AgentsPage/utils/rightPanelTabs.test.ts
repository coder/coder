import { beforeEach, describe, expect, it } from "vitest";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { AGENT_BROWSER_APP_SLUG } from "#/modules/apps/apps";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import {
	chatDefaultTerminalHiddenStorage,
	chatRightPanelTabsStorage,
	clearChatStorage,
} from "../storage";
import {
	isUserRightPanelTab,
	type UserRightPanelTab,
	validateUserRightPanelTabs,
} from "./rightPanelTabs";

// Mirrors how AgentChatPageView reads persisted tabs: raw descriptors
// narrowed so stale shapes from older builds are dropped on read.
const readPersistedTabs = (chatID: string): UserRightPanelTab[] =>
	chatRightPanelTabsStorage.forId(chatID).get().filter(isUserRightPanelTab);

type TerminalRightPanelTab = Extract<UserRightPanelTab, { kind: "terminal" }>;

const terminalTab = (
	overrides: Partial<TerminalRightPanelTab> = {},
): TerminalRightPanelTab => ({
	id: "terminal-2",
	kind: "terminal",
	reconnectionToken: "11111111-1111-4111-8111-111111111111",
	...overrides,
});

describe("right-panel tab validation", () => {
	const tabs: UserRightPanelTab[] = [
		terminalTab(),
		{
			id: "app-preview",
			kind: "workspace_app",
			label: "Preview",
			agentId: MockWorkspaceAgent.id,
			appId: MockWorkspaceApp.id,
		},
		{
			id: "port-3000",
			kind: "port",
			label: "Port 3000",
			agentId: MockWorkspaceAgent.id,
			port: 3000,
			protocol: "http",
		},
	];

	it("keeps tabs that still match the workspace and wildcard host", () => {
		expect(
			validateUserRightPanelTabs(tabs, {
				workspace: MockWorkspace,
				workspaceAgent: MockWorkspaceAgent,
				wildcardHostname: "*.apps.example.com",
			}),
		).toEqual(tabs);
	});

	it("drops terminal tabs when there is no workspace agent", () => {
		const validated = validateUserRightPanelTabs(tabs, {
			workspace: MockWorkspace,
			workspaceAgent: undefined,
			wildcardHostname: "*.apps.example.com",
		});

		expect(validated).toEqual(tabs.filter((tab) => tab.kind !== "terminal"));
	});

	it("drops port tabs when wildcard access is unavailable", () => {
		const validated = validateUserRightPanelTabs(tabs, {
			workspace: MockWorkspace,
			workspaceAgent: MockWorkspaceAgent,
			wildcardHostname: "",
		});

		expect(validated).toEqual(tabs.filter((tab) => tab.kind !== "port"));
	});

	it("drops port tabs when the agent no longer exposes the port forwarding helper", () => {
		const agent: WorkspaceAgent = {
			...MockWorkspaceAgent,
			display_apps: MockWorkspaceAgent.display_apps.filter(
				(app) => app !== "port_forwarding_helper",
			),
		};

		const validated = validateUserRightPanelTabs(tabs, {
			workspace: buildWorkspace([agent]),
			workspaceAgent: agent,
			wildcardHostname: "*.apps.example.com",
		});

		expect(validated).toEqual(tabs.filter((tab) => tab.kind !== "port"));
	});

	it("drops app tabs when the app no longer exists", () => {
		const validated = validateUserRightPanelTabs(
			[
				{
					id: "missing-app",
					kind: "workspace_app",
					label: "Missing",
					agentId: MockWorkspaceAgent.id,
					appId: "missing-app",
				},
			],
			{
				workspace: MockWorkspace,
				workspaceAgent: MockWorkspaceAgent,
				wildcardHostname: "*.apps.example.com",
			},
		);

		expect(validated).toEqual([]);
	});

	it("drops app tabs when the app is no longer embeddable", () => {
		const commandApp = buildApp("command-app", { command: "run-preview" });
		const workspace = buildWorkspace([buildAgent("agent-1", [commandApp])]);
		const appTab: UserRightPanelTab = {
			id: "command-app-tab",
			kind: "workspace_app",
			label: "Command",
			agentId: "agent-1",
			appId: "command-app",
		};

		const validated = validateUserRightPanelTabs([appTab], {
			workspace,
			workspaceAgent: workspace.latest_build.resources[0].agents?.[0],
			wildcardHostname: "*.apps.example.com",
		});

		expect(validated).toEqual([]);
	});

	it("drops agent-browser app tabs in favor of the built-in Browser tab", () => {
		const browserApp = buildApp("browser-app", {
			slug: AGENT_BROWSER_APP_SLUG,
		});
		const workspace = buildWorkspace([buildAgent("agent-1", [browserApp])]);
		const appTab: UserRightPanelTab = {
			id: "browser-app-tab",
			kind: "workspace_app",
			label: "agent-browser",
			agentId: "agent-1",
			appId: "browser-app",
		};

		const validated = validateUserRightPanelTabs([appTab], {
			workspace,
			workspaceAgent: workspace.latest_build.resources[0].agents?.[0],
			wildcardHostname: "*.apps.example.com",
		});

		expect(validated).toEqual([]);
	});
});

function buildWorkspace(resourceAgents: readonly WorkspaceAgent[]): Workspace {
	const resourceTemplate = MockWorkspace.latest_build.resources[0];
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: [{ ...resourceTemplate, agents: resourceAgents }],
		},
	};
}

function buildAgent(id: string, apps: WorkspaceApp[]): WorkspaceAgent {
	return { ...MockWorkspaceAgent, id, name: id, apps };
}

function buildApp(
	id: string,
	overrides: Partial<WorkspaceApp> = {},
): WorkspaceApp {
	return {
		...MockWorkspaceApp,
		id,
		slug: id,
		display_name: id,
		health: "healthy",
		statuses: [],
		...overrides,
	};
}

describe("right-panel tab storage", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("round-trips every persisted tab kind per chat", () => {
		const tabs: UserRightPanelTab[] = [
			terminalTab(),
			terminalTab({
				id: "terminal-claude",
				label: "Claude Code",
				initialCommand: "claude",
				sourceAppId: MockWorkspaceApp.id,
			}),
			{
				id: "app-preview",
				kind: "workspace_app",
				label: "Preview",
				agentId: MockWorkspaceAgent.id,
				appId: MockWorkspaceApp.id,
			},
			{
				id: "port-3000",
				kind: "port",
				label: "Port 3000",
				agentId: MockWorkspaceAgent.id,
				port: 3000,
				protocol: "http",
			},
		];

		chatRightPanelTabsStorage.forId("chat-1").set(tabs);

		expect(readPersistedTabs("chat-1")).toEqual(tabs);
		expect(readPersistedTabs("chat-2")).toEqual([]);
	});

	it("clears all persisted right-panel state for a chat", () => {
		const tabs: UserRightPanelTab[] = [terminalTab()];

		chatRightPanelTabsStorage.forId("chat-1").set(tabs);
		chatDefaultTerminalHiddenStorage.forId("chat-1").set(true);
		chatRightPanelTabsStorage.forId("chat-2").set(tabs);
		chatDefaultTerminalHiddenStorage.forId("chat-2").set(true);

		clearChatStorage("chat-1");

		expect(readPersistedTabs("chat-1")).toEqual([]);
		expect(chatDefaultTerminalHiddenStorage.forId("chat-1").get()).toBe(false);
		expect(readPersistedTabs("chat-2")).toEqual(tabs);
		expect(chatDefaultTerminalHiddenStorage.forId("chat-2").get()).toBe(true);
	});

	it("ignores invalid stored values", () => {
		localStorage.setItem(
			`${chatRightPanelTabsStorage.prefix}chat-1`,
			JSON.stringify([{ id: "bad-tab", kind: "port" }]),
		);

		expect(readPersistedTabs("chat-1")).toEqual([]);
	});

	it("ignores port tabs with out-of-range ports", () => {
		localStorage.setItem(
			`${chatRightPanelTabsStorage.prefix}chat-1`,
			JSON.stringify([
				{
					id: "port-70000",
					kind: "port",
					label: "Port 70000",
					agentId: MockWorkspaceAgent.id,
					port: 70000,
					protocol: "http",
				},
			]),
		);

		expect(readPersistedTabs("chat-1")).toEqual([]);
	});

	it("restores stored terminal tabs with string reconnect tokens", () => {
		const tabs = [terminalTab({ reconnectionToken: "opaque-token" })];
		localStorage.setItem(
			`${chatRightPanelTabsStorage.prefix}chat-1`,
			JSON.stringify(tabs),
		);

		expect(readPersistedTabs("chat-1")).toEqual(tabs);
	});

	it("treats malformed default-terminal-hidden values as visible", () => {
		localStorage.setItem(
			`${chatDefaultTerminalHiddenStorage.prefix}chat-1`,
			"yes",
		);

		expect(chatDefaultTerminalHiddenStorage.forId("chat-1").get()).toBe(false);
	});
});
