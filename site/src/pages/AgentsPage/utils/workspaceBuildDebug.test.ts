import { describe, expect, it } from "vitest";
import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";
import { MockFailedWorkspace } from "#/testHelpers/entities";
import {
	debugWorkspaceBuildLogsMaxBytes,
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
} from "./workspaceBuildDebug";

const mockFailedBuild: WorkspaceBuild = {
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

const mockLogs: ProvisionerJobLog[] = [
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

const logLines = (count: number, output: (index: number) => string) =>
	Array.from(
		{ length: count },
		(_, index): ProvisionerJobLog => ({
			id: index,
			created_at: "2024-01-01T00:00:00.000Z",
			log_source: "provisioner",
			log_level: "info",
			stage: "Starting workspace",
			output: output(index),
		}),
	);

const byteLength = (text: string) => new TextEncoder().encode(text).length;

describe("debugWorkspaceBuildPrompt", () => {
	it("names the workspace and the failed transition", () => {
		const prompt = debugWorkspaceBuildPrompt({
			...mockFailedBuild,
			transition: "stop",
		});

		expect(
			prompt.startsWith("Workspace dfraley/my-workspace failed to stop."),
		).toBe(true);
		expect(prompt.match(/failed to stop/g)).toHaveLength(2);
		expect(prompt).not.toContain("failed to start");
	});
});

describe("formatWorkspaceBuildLogsForDebug", () => {
	it("includes the build summary, job error, and logs grouped by stage", () => {
		const text = formatWorkspaceBuildLogsForDebug(mockFailedBuild, mockLogs);

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

	it("keeps the newest logs within the byte budget", () => {
		// Three-byte characters so the cut can land inside one.
		const text = formatWorkspaceBuildLogsForDebug(
			mockFailedBuild,
			logLines(3000, (index) => `${index} ${"│".repeat(32)}`),
		);

		expect(byteLength(text)).toBeLessThanOrEqual(
			debugWorkspaceBuildLogsMaxBytes,
		);
		expect(text).toContain("Job error: terraform plan: exit status 1");
		expect(text).toContain(
			"[earlier output omitted to fit the attachment size limit]",
		);
		expect(text).not.toContain("\uFFFD");
		expect(text).not.toContain("[info] 0 │");
		expect(text).toContain(`[info] 2999 ${"│".repeat(32)}`);
	});

	it("caps an oversized job error in the header", () => {
		const text = formatWorkspaceBuildLogsForDebug(
			{
				...mockFailedBuild,
				job: {
					...mockFailedBuild.job,
					error: `summary first ${"│".repeat(debugWorkspaceBuildLogsMaxBytes)}`,
				},
			},
			mockLogs,
		);

		expect(byteLength(text)).toBeLessThanOrEqual(
			debugWorkspaceBuildLogsMaxBytes,
		);
		expect(text).toContain("Job error: summary first │");
		expect(text).toContain("│ (error truncated)");
		expect(text).not.toContain("\uFFFD");
		expect(text).toContain("[error] Error: Invalid value for variable");
	});
});
