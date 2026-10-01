import { describe, expect, it } from "vitest";
import {
	getWorkspaceToolOutcome,
	type WorkspaceToolOutcome,
} from "./workspaceToolOutcome";

describe("getWorkspaceToolOutcome", () => {
	it.each<{
		case: string;
		result: unknown;
		outcome: WorkspaceToolOutcome | undefined;
	}>([
		{ case: "no result", result: undefined, outcome: undefined },
		{ case: "agent ready", result: { started: true }, outcome: undefined },
		{
			case: "agent did not connect",
			result: { agent_status: "not_ready", agent_error: "dial timeout" },
			outcome: {
				failure: {
					labelSuffix: "could not connect to workspace agent",
					tooltip: "dial timeout",
				},
			},
		},
		{
			case: "agent did not connect, no error text",
			result: { agent_status: "not_ready" },
			outcome: {
				failure: {
					labelSuffix: "could not connect to workspace agent",
					tooltip: "The chat could not connect to the workspace agent.",
				},
			},
		},
		{
			case: "no agent",
			result: { agent_status: "no_agent" },
			outcome: {
				failure: {
					labelSuffix: "no workspace agent",
					tooltip: "This workspace has no agent, so the chat cannot use it.",
				},
			},
		},
		{
			case: "agent selection failed",
			result: {
				agent_status: "selection_error",
				agent_error: "multiple agents match",
			},
			outcome: {
				failure: {
					labelSuffix: "could not choose a workspace agent",
					tooltip: "multiple agents match",
				},
			},
		},
		{
			case: "startup script error",
			result: {
				startup_scripts: "startup_scripts_failed",
				lifecycle_state: "start_error",
			},
			outcome: {
				notice:
					"A startup script or dev container failed. Skills, instructions, or tools they set up may be missing.",
			},
		},
		{
			case: "startup script hit its time limit",
			result: {
				startup_scripts: "startup_scripts_failed",
				lifecycle_state: "start_timeout",
			},
			outcome: {
				notice:
					"Startup scripts ran past their time limit and were stopped. Skills, instructions, or tools they set up may be missing.",
			},
		},
		{
			case: "startup scripts ended in another state",
			result: {
				startup_scripts: "startup_scripts_failed",
				lifecycle_state: "off",
			},
			outcome: {
				notice:
					"Startup scripts did not finish. Skills, instructions, or tools they set up may be missing.",
			},
		},
		{
			case: "chat stopped waiting for startup scripts",
			result: { startup_scripts: "startup_scripts_timeout" },
			outcome: {
				notice:
					"Startup scripts were still running when the chat stopped waiting for them. Skills, instructions, or tools they set up may not be available yet.",
			},
		},
		{
			case: "wait ended for another reason",
			result: { startup_scripts: "startup_scripts_unknown" },
			outcome: undefined,
		},
	])("$case", ({ result, outcome }) => {
		expect(getWorkspaceToolOutcome(result)).toEqual(outcome);
	});
});
