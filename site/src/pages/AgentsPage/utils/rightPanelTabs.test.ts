import { beforeEach, describe, expect, it } from "vitest";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import {
	clearPersistedRightPanelState,
	getPersistedDefaultTerminalHidden,
	getPersistedRightPanelTabs,
	rightPanelTabStorageKeyPrefix,
	savePersistedDefaultTerminalHidden,
	savePersistedRightPanelTabs,
} from "./rightPanelTabStorage";
import {
	type UserRightPanelTab,
	validateUserRightPanelTabs,
} from "./rightPanelTabs";

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

	it("persists tabs per chat", () => {
		const tabs: UserRightPanelTab[] = [terminalTab()];

		savePersistedRightPanelTabs("chat-1", tabs);

		expect(getPersistedRightPanelTabs("chat-1")).toEqual(tabs);
		expect(getPersistedRightPanelTabs("chat-2")).toEqual([]);
	});

	it("persists command-app terminal tabs", () => {
		const tabs: UserRightPanelTab[] = [
			terminalTab({
				id: "terminal-claude",
				label: "Claude Code",
				initialCommand: "claude",
				sourceAppId: MockWorkspaceApp.id,
			}),
		];

		savePersistedRightPanelTabs("chat-1", tabs);

		expect(getPersistedRightPanelTabs("chat-1")).toEqual(tabs);
	});

	it("clears all persisted right-panel state for a chat", () => {
		const tabs: UserRightPanelTab[] = [terminalTab()];

		savePersistedRightPanelTabs("chat-1", tabs);
		savePersistedDefaultTerminalHidden("chat-1", true);
		savePersistedRightPanelTabs("chat-2", tabs);
		savePersistedDefaultTerminalHidden("chat-2", true);

		clearPersistedRightPanelState("chat-1");

		expect(getPersistedRightPanelTabs("chat-1")).toEqual([]);
		expect(getPersistedDefaultTerminalHidden("chat-1")).toBe(false);
		expect(getPersistedRightPanelTabs("chat-2")).toEqual(tabs);
		expect(getPersistedDefaultTerminalHidden("chat-2")).toBe(true);
	});

	it("persists workspace_app tabs", () => {
		const tabs: UserRightPanelTab[] = [
			{
				id: "app-preview",
				kind: "workspace_app",
				label: "Preview",
				agentId: MockWorkspaceAgent.id,
				appId: MockWorkspaceApp.id,
			},
		];

		savePersistedRightPanelTabs("chat-1", tabs);

		expect(getPersistedRightPanelTabs("chat-1")).toEqual(tabs);
	});

	it("persists port tabs", () => {
		const tabs: UserRightPanelTab[] = [
			{
				id: "port-3000",
				kind: "port",
				label: "Port 3000",
				agentId: MockWorkspaceAgent.id,
				port: 3000,
				protocol: "http",
			},
		];

		savePersistedRightPanelTabs("chat-1", tabs);

		expect(getPersistedRightPanelTabs("chat-1")).toEqual(tabs);
	});

	it("ignores invalid stored values", () => {
		localStorage.setItem(
			`${rightPanelTabStorageKeyPrefix}chat-1`,
			JSON.stringify([{ id: "bad-tab", kind: "port" }]),
		);

		expect(getPersistedRightPanelTabs("chat-1")).toEqual([]);
	});

	it("ignores port tabs with out-of-range ports", () => {
		localStorage.setItem(
			`${rightPanelTabStorageKeyPrefix}chat-1`,
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

		expect(getPersistedRightPanelTabs("chat-1")).toEqual([]);
	});

	it("restores stored terminal tabs with string reconnect tokens", () => {
		const tabs = [terminalTab({ reconnectionToken: "opaque-token" })];
		localStorage.setItem(
			`${rightPanelTabStorageKeyPrefix}chat-1`,
			JSON.stringify(tabs),
		);

		expect(getPersistedRightPanelTabs("chat-1")).toEqual(tabs);
	});
});

describe("default terminal hidden storage", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("round trips a hidden terminal flag", () => {
		savePersistedDefaultTerminalHidden("chat-1", true);

		expect(getPersistedDefaultTerminalHidden("chat-1")).toBe(true);
		expect(getPersistedDefaultTerminalHidden("chat-2")).toBe(false);
	});

	it("removes the stored flag when saving false", () => {
		savePersistedDefaultTerminalHidden("chat-1", true);

		savePersistedDefaultTerminalHidden("chat-1", false);

		expect(getPersistedDefaultTerminalHidden("chat-1")).toBe(false);
		expect(localStorage.length).toBe(0);
	});

	it("ignores undefined chat IDs", () => {
		savePersistedDefaultTerminalHidden(undefined, true);

		expect(getPersistedDefaultTerminalHidden(undefined)).toBe(false);
		expect(localStorage.length).toBe(0);
	});

	it("treats malformed values as visible", () => {
		savePersistedDefaultTerminalHidden("chat-1", true);
		const key = localStorage.key(0);
		if (!key) {
			throw new Error("expected default terminal hidden key to be stored");
		}
		localStorage.setItem(key, "yes");

		expect(getPersistedDefaultTerminalHidden("chat-1")).toBe(false);
	});
});
