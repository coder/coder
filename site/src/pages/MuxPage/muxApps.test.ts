import type { Workspace, WorkspaceApp } from "#/api/typesGenerated";
import {
	MockPrebuiltWorkspace,
	MockStoppedWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceResource,
} from "#/testHelpers/entities";
import {
	filterMuxWorkspaces,
	getMuxCandidatesFromWorkspace,
	pickPreferredMuxApp,
} from "./muxApps";

const muxApp = (overrides: Partial<WorkspaceApp> = {}): WorkspaceApp => ({
	...MockWorkspaceApp,
	id: "mux-app",
	slug: "mux",
	display_name: "Mux",
	icon: "/icon/mux.svg",
	health: "healthy",
	open_in: "tab",
	...overrides,
});

const workspaceWithApps = (
	workspace: Workspace,
	apps: readonly WorkspaceApp[],
): Workspace => ({
	...workspace,
	latest_build: {
		...workspace.latest_build,
		resources: [
			{
				...MockWorkspaceResource,
				agents: [
					{
						...MockWorkspaceAgent,
						apps,
					},
				],
			},
		],
	},
});

describe("filterMuxWorkspaces", () => {
	it("includes a running workspace with the default Mux app", () => {
		const workspace = workspaceWithApps(MockWorkspace, [muxApp()]);

		expect(filterMuxWorkspaces([workspace])).toEqual([workspace]);
	});

	it("includes a stopped workspace with the default Mux app", () => {
		const workspace = workspaceWithApps(MockStoppedWorkspace, [muxApp()]);

		expect(filterMuxWorkspaces([workspace])).toEqual([workspace]);
	});

	it("excludes dormant workspaces", () => {
		const workspace = workspaceWithApps(
			{
				...MockWorkspace,
				dormant_at: "2024-01-01T00:00:00.000Z",
			},
			[muxApp()],
		);

		expect(filterMuxWorkspaces([workspace])).toEqual([]);
	});

	it("excludes prebuild workspaces", () => {
		const workspace = workspaceWithApps(MockPrebuiltWorkspace, [muxApp()]);

		expect(filterMuxWorkspaces([workspace])).toEqual([]);
	});

	it("excludes workspaces with only non-Mux apps", () => {
		const workspace = workspaceWithApps(MockWorkspace, [
			{
				...MockWorkspaceApp,
				slug: "not-mux",
				icon: "/icon/code.svg",
			},
		]);

		expect(filterMuxWorkspaces([workspace])).toEqual([]);
	});

	it("includes a workspace with a custom slug and the Mux icon", () => {
		const workspace = workspaceWithApps(MockWorkspace, [
			muxApp({ id: "custom", slug: "custom-mux" }),
		]);

		expect(filterMuxWorkspaces([workspace])).toEqual([workspace]);
	});
});

describe("pickPreferredMuxApp", () => {
	it("prefers an app with the mux slug", () => {
		const workspace = workspaceWithApps(MockWorkspace, [
			muxApp({ id: "z-mux", slug: "z-mux" }),
			muxApp({ id: "default-mux", slug: "mux" }),
		]);

		const preferred = pickPreferredMuxApp(
			getMuxCandidatesFromWorkspace(workspace),
		);

		expect(preferred?.app.id).toBe("default-mux");
	});

	it("sorts custom Mux apps by slug when no default app exists", () => {
		const workspace = workspaceWithApps(MockWorkspace, [
			muxApp({ id: "z-mux", slug: "z-mux" }),
			muxApp({ id: "a-mux", slug: "a-mux" }),
		]);

		const preferred = pickPreferredMuxApp(
			getMuxCandidatesFromWorkspace(workspace),
		);

		expect(preferred?.app.id).toBe("a-mux");
	});
});
