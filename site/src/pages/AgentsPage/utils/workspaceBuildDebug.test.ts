import { afterEach, describe, expect, it, vi } from "vitest";
import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";
import { MockFailedWorkspace } from "#/testHelpers/entities";
import {
	buildDebugWorkspaceBuildPath,
	clearDebugWorkspaceBuildIntent,
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildLogsMaxChars,
	debugWorkspaceBuildPrompt,
	debugWorkspaceBuildSearchParam,
	formatWorkspaceBuildLogsForDebug,
	hasDebugWorkspaceBuildIntent,
	storeDebugWorkspaceBuildIntent,
} from "./workspaceBuildDebug";

const failedBuild: WorkspaceBuild = {
	...MockFailedWorkspace.latest_build,
	workspace_owner_name: "dfraley",
	workspace_name: "my-workspace",
	template_version_name: "v1.2.3",
	build_number: 7,
	transition: "start",
	reason: "initiator",
	status: "failed",
	job: {
		...MockFailedWorkspace.latest_build.job,
		status: "failed",
		error: "terraform plan: exit status 1",
		error_code: "REQUIRED_TEMPLATE_VARIABLES",
		logs_overflowed: false,
	},
};

const logs: ProvisionerJobLog[] = [
	{
		id: 1,
		created_at: "2024-01-01T00:00:00.000Z",
		log_source: "provisioner_daemon",
		log_level: "info",
		stage: "Setting up",
		output: "",
	},
	{
		id: 2,
		created_at: "2024-01-01T00:00:01.000Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Planning infrastructure",
		output: "Initializing the backend...",
	},
	{
		id: 3,
		created_at: "2024-01-01T00:00:02.000Z",
		log_source: "provisioner",
		log_level: "error",
		stage: "Planning infrastructure",
		output: "Error: Invalid value for variable",
	},
];

afterEach(() => {
	localStorage.clear();
	vi.restoreAllMocks();
});

describe("buildDebugWorkspaceBuildPath", () => {
	it("links to the agents create page with the build id", () => {
		const path = buildDebugWorkspaceBuildPath("build id/with?chars");
		const url = new URL(path, "https://coder.example.com");

		expect(url.pathname).toBe("/agents");
		expect(url.searchParams.get(debugWorkspaceBuildSearchParam)).toBe(
			"build id/with?chars",
		);
	});
});

describe("debugWorkspaceBuildPrompt", () => {
	it("names the workspace and the failed transition", () => {
		const prompt = debugWorkspaceBuildPrompt({
			...failedBuild,
			transition: "stop",
		});

		expect(
			prompt.startsWith("Workspace dfraley/my-workspace failed to stop."),
		).toBe(true);
		expect(prompt.match(/failed to stop/g)).toHaveLength(2);
		expect(prompt).not.toContain("failed to start");
	});
});

describe("debugWorkspaceBuildLogsFileName", () => {
	it("names the attachment after the workspace and build number", () => {
		expect(debugWorkspaceBuildLogsFileName(failedBuild)).toBe(
			"workspace-build-logs-dfraley-my-workspace-7.txt",
		);
	});
});

describe("formatWorkspaceBuildLogsForDebug", () => {
	it("includes the build summary, job error, and logs grouped by stage", () => {
		const text = formatWorkspaceBuildLogsForDebug(failedBuild, logs);

		expect(text).toBe(
			[
				"Workspace: dfraley/my-workspace",
				"Template version: v1.2.3",
				"Build: #7 (start, reason: initiator)",
				"Build status: failed",
				"Job error code: REQUIRED_TEMPLATE_VARIABLES",
				"Job error: terraform plan: exit status 1",
				"",
				"Build logs:",
				"",
				"=== Setting up (2024-01-01T00:00:00.000Z) ===",
				"[info] ",
				"",
				"=== Planning infrastructure (2024-01-01T00:00:01.000Z) ===",
				"[debug] Initializing the backend...",
				"[error] Error: Invalid value for variable",
				"",
			].join("\n"),
		);
	});

	it("notes when no logs were recorded", () => {
		const text = formatWorkspaceBuildLogsForDebug(failedBuild, []);

		expect(text).toContain("(no build logs were recorded)");
	});

	it("notes when the provisioner log limit truncated the build", () => {
		const text = formatWorkspaceBuildLogsForDebug(
			{ ...failedBuild, job: { ...failedBuild.job, logs_overflowed: true } },
			logs,
		);

		expect(text).toContain("Note: the build hit the provisioner log limit");
	});

	it("keeps the header and the most recent lines within the size budget", () => {
		const line = "x".repeat(99);
		const manyLogs: ProvisionerJobLog[] = Array.from(
			{ length: 3000 },
			(_, index) => ({
				id: index,
				created_at: "2024-01-01T00:00:00.000Z",
				log_source: "provisioner",
				log_level: "info",
				stage: "Starting workspace",
				output: `${index} ${line}`,
			}),
		);

		const text = formatWorkspaceBuildLogsForDebug(failedBuild, manyLogs);

		expect(text.length).toBeLessThanOrEqual(
			debugWorkspaceBuildLogsMaxChars + 80,
		);
		expect(text).toContain("Job error: terraform plan: exit status 1");
		expect(text).toMatch(
			/\[\d+ earlier lines omitted to fit the attachment size limit\]/,
		);
		expect(text).not.toContain("[info] 0 x");
		expect(text).toContain(`[info] 2999 ${line}`);
	});
});

describe("debug workspace build intent", () => {
	it("is present only for the clicked build until cleared", () => {
		storeDebugWorkspaceBuildIntent("build-a");

		expect(hasDebugWorkspaceBuildIntent("build-b")).toBe(false);
		expect(hasDebugWorkspaceBuildIntent("build-a")).toBe(true);

		clearDebugWorkspaceBuildIntent();
		expect(hasDebugWorkspaceBuildIntent("build-a")).toBe(false);
	});

	it("expires", () => {
		const now = Date.now();
		vi.spyOn(Date, "now").mockReturnValue(now);
		storeDebugWorkspaceBuildIntent("build-a");
		vi.spyOn(Date, "now").mockReturnValue(now + 6 * 60 * 1000);

		expect(hasDebugWorkspaceBuildIntent("build-a")).toBe(false);
	});
});
