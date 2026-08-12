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
	findWorkspaceAppWithAgent,
	getAllAppsWithAgent,
} from "./workspaceApps";

describe("workspaceApps", () => {
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
