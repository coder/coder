import { describe, expect, it } from "vitest";
import type { Workspace } from "#/api/typesGenerated";
import { MockWorkspace } from "#/testHelpers/entities";
import { abilitiesByWorkspaceStatus, canCancelWorkspaceBuild } from "./actions";

const buildWorkspace = (
	status: Workspace["latest_build"]["status"],
	overrides: Partial<Workspace> = {},
): Workspace => ({
	...MockWorkspace,
	...overrides,
	latest_build: {
		...MockWorkspace.latest_build,
		status,
		...overrides.latest_build,
	},
});

describe("canCancelWorkspaceBuild", () => {
	it("allows pending builds for non-owners", () => {
		const workspace = buildWorkspace("pending", {
			template_allow_user_cancel_workspace_jobs: false,
		});

		expect(canCancelWorkspaceBuild(workspace, { isOwner: false })).toBe(true);
	});

	it("denies deleting builds for non-owners without template permission", () => {
		const workspace = buildWorkspace("deleting", {
			template_allow_user_cancel_workspace_jobs: false,
		});

		expect(canCancelWorkspaceBuild(workspace, { isOwner: false })).toBe(false);
	});

	it("allows deleting builds when the template permits user cancels", () => {
		const workspace = buildWorkspace("deleting", {
			template_allow_user_cancel_workspace_jobs: true,
		});

		expect(canCancelWorkspaceBuild(workspace, { isOwner: false })).toBe(true);
	});

	it("allows deleting builds for owners", () => {
		const workspace = buildWorkspace("deleting", {
			template_allow_user_cancel_workspace_jobs: false,
		});

		expect(canCancelWorkspaceBuild(workspace, { isOwner: true })).toBe(true);
	});
});

describe("abilitiesByWorkspaceStatus", () => {
	it("matches deleting cancel permissions for non-owners", () => {
		const workspace = buildWorkspace("deleting", {
			template_allow_user_cancel_workspace_jobs: false,
		});

		expect(
			abilitiesByWorkspaceStatus(workspace, {
				canDebug: false,
				isOwner: false,
			}).canCancel,
		).toBe(false);
	});
});
