import { asRecord, asString } from "../runtimeTypeUtils";

export type WorkspaceToolOutcome =
	| { failure: { labelSuffix: string; tooltip: string } }
	| { notice: string };

const missingSetup =
	"Skills, instructions, or tools they set up may be missing.";

/**
 * Reads how the agent wait of a successful start_workspace or
 * create_workspace call ended. A failure means the chat cannot use the
 * workspace; a notice means it can, but scripted setup may be missing.
 *
 * `agent_status: not_ready` is set when connecting to the agent timed
 * out, not from the agent lifecycle. `startup_scripts` is only set
 * after a successful connection, so a result never has both.
 */
export const getWorkspaceToolOutcome = (
	result: unknown,
): WorkspaceToolOutcome | undefined => {
	const rec = asRecord(result);
	if (!rec) {
		return undefined;
	}
	const agentError = asString(rec.agent_error);
	switch (asString(rec.agent_status)) {
		case "not_ready":
			return {
				failure: {
					labelSuffix: "could not connect to workspace agent",
					tooltip:
						agentError || "The chat could not connect to the workspace agent.",
				},
			};
		case "no_agent":
			return {
				failure: {
					labelSuffix: "no workspace agent",
					tooltip: "This workspace has no agent, so the chat cannot use it.",
				},
			};
		case "selection_error":
			return {
				failure: {
					labelSuffix: "could not choose a workspace agent",
					tooltip: agentError || "The chat could not choose a workspace agent.",
				},
			};
	}
	switch (asString(rec.startup_scripts)) {
		case "startup_scripts_failed":
			switch (asString(rec.lifecycle_state)) {
				case "start_error":
					return {
						notice: `A startup script or dev container failed. ${missingSetup}`,
					};
				case "start_timeout":
					return {
						notice: `Startup scripts ran past their time limit and were stopped. ${missingSetup}`,
					};
				default:
					return { notice: `Startup scripts did not finish. ${missingSetup}` };
			}
		case "startup_scripts_timeout":
			return {
				notice:
					"Startup scripts were still running when the chat stopped waiting for them. Skills, instructions, or tools they set up may not be available yet.",
			};
	}
	return undefined;
};
