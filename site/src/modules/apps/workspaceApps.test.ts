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
	findWorkspaceAgent,
	findWorkspaceAppWithAgent,
	getAllAppsWithAgent,
	getWorkspaceAgents,
	isAppBlockedByMissingWildcard,
	isWorkspaceAppEmbeddable,
} from "./workspaceApps";

describe("workspaceApps", () => {
	describe("getWorkspaceAgents", () => {
		it("flattens agents across workspace resources", () => {
			const workspace = buildWorkspace([
				[buildAgent("agent-1", [buildApp("app-1")])],
				[buildAgent("agent-2", [buildApp("app-2")])],
			]);

			expect(getWorkspaceAgents(workspace).map((agent) => agent.id)).toEqual([
				"agent-1",
				"agent-2",
			]);
		});
	});

	describe("getAllAppsWithAgent", () => {
		it("flattens workspace apps with their owning agent", () => {
			const workspace = buildWorkspace([
				[buildAgent("agent-1", [buildApp("app-1")])],
				[buildAgent("agent-2", [buildApp("app-2")])],
			]);

			expect(
				getAllAppsWithAgent(workspace).map((app) => ({
					appId: app.id,
					agentId: app.agent.id,
				})),
			).toEqual([
				{ appId: "app-1", agentId: "agent-1" },
				{ appId: "app-2", agentId: "agent-2" },
			]);
		});

		it("returns an empty list when the workspace has no agents", () => {
			const workspace = buildWorkspace([]);

			expect(getAllAppsWithAgent(workspace)).toEqual([]);
		});
	});

	describe("findWorkspaceAgent", () => {
		it("returns the matching agent", () => {
			const workspace = buildWorkspace([
				[buildAgent("agent-1", [buildApp("app-1")])],
			]);

			expect(findWorkspaceAgent(workspace, "agent-1")?.name).toBe("agent-1");
			expect(findWorkspaceAgent(workspace, "missing")).toBeUndefined();
		});
	});

	describe("findWorkspaceAppWithAgent", () => {
		it("returns the matching app with its owning agent", () => {
			const workspace = buildWorkspace([
				[buildAgent("agent-1", [buildApp("app-1")])],
				[buildAgent("agent-2", [buildApp("app-2")])],
			]);

			expect(
				findWorkspaceAppWithAgent(workspace, "agent-2", "app-2"),
			).toMatchObject({
				id: "app-2",
				agent: { id: "agent-2" },
			});
			expect(
				findWorkspaceAppWithAgent(workspace, "agent-1", "app-2"),
			).toBeUndefined();
		});
	});

	describe("isWorkspaceAppEmbeddable", () => {
		it("returns true for visible path-based apps", () => {
			expect(isWorkspaceAppEmbeddable(buildApp("app-1"))).toBe(true);
		});

		it("returns false for command apps, hidden apps, and external apps", () => {
			expect(
				isWorkspaceAppEmbeddable(
					buildApp("command-app", { command: "run-preview" }),
				),
			).toBe(false);
			expect(
				isWorkspaceAppEmbeddable(buildApp("hidden-app", { hidden: true })),
			).toBe(false);
			expect(
				isWorkspaceAppEmbeddable(
					buildApp("external-app", {
						external: true,
						url: "https://example.com",
					}),
				),
			).toBe(false);
		});
	});

	describe("isAppBlockedByMissingWildcard", () => {
		it("blocks subdomain apps when no wildcard host is configured", () => {
			const subdomainApp = buildApp("app-1", { subdomain: true });

			expect(isAppBlockedByMissingWildcard(subdomainApp, "")).toBe(true);
			expect(isAppBlockedByMissingWildcard(subdomainApp, undefined)).toBe(true);
			expect(
				isAppBlockedByMissingWildcard(subdomainApp, "*.apps.example.com"),
			).toBe(false);
		});

		it("never blocks path-based apps", () => {
			const pathApp = buildApp("app-1", { subdomain: false });

			expect(isAppBlockedByMissingWildcard(pathApp, "")).toBe(false);
			expect(isAppBlockedByMissingWildcard(pathApp, undefined)).toBe(false);
		});
	});
});

function buildWorkspace(
	resourceAgents: readonly WorkspaceAgent[][],
): Workspace {
	const resourceTemplate = MockWorkspace.latest_build.resources[0];
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: resourceAgents.map((agents) => ({
				...resourceTemplate,
				agents,
			})),
		},
	};
}

function buildAgent(id: string, apps: WorkspaceApp[]): WorkspaceAgent {
	return {
		...MockWorkspaceAgent,
		id,
		name: id,
		apps,
	};
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
