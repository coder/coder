import { createContext } from "react";
import type { MergedTool } from "../../ChatConversation/types";
import { ACPToolNames, isACPToolName } from "./acpToolNames";
import { asRecord, asString, parseArgs } from "./utils";

type ACPSessionDescriptor = { displayName: string; prompt: string };

/** Shares native session labels across lifecycle calls in a chat. */
export const ACPContext = createContext<
	ReadonlyMap<string, ACPSessionDescriptor>
>(new Map());

/** Encodes the full native identity without collisions between its fields. */
export function acpSessionKey(args: unknown, result: unknown): string {
	const input = parseArgs(args);
	const output = parseArgs(result);
	const agent = asRecord(input?.agent);
	const harnessId =
		asString(output?.harness_id) ||
		asString(input?.harness_id) ||
		asString(agent?.harness_id);
	const directory =
		asString(output?.working_directory) || asString(input?.working_directory);
	const sessionId = asString(output?.session_id) || asString(input?.session_id);
	return harnessId && directory && sessionId
		? JSON.stringify([harnessId, directory, sessionId])
		: "";
}

/** Recovers session labels from tools already present in the parent chat. */
export function buildACPSessionDescriptors(tools: readonly MergedTool[]) {
	const sessions = new Map<string, ACPSessionDescriptor>();
	for (const tool of tools) {
		if (!isACPToolName(tool.name)) continue;
		const input = parseArgs(tool.args);
		const output = parseArgs(tool.result);
		const key = acpSessionKey(tool.args, tool.result);
		if (!key) continue;
		const previous = sessions.get(key);
		sessions.set(key, {
			displayName:
				asString(output?.harness_display_name) || previous?.displayName || "",
			prompt:
				(tool.name === ACPToolNames.SpawnAgent
					? asString(input?.prompt)
					: "") ||
				previous?.prompt ||
				"",
		});
	}
	return sessions;
}
