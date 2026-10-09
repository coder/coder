import { createContext } from "react";
import type { MergedTool } from "../../ChatConversation/types";
import { ACPToolNames, isACPToolName } from "./acpToolNames";
import { asRecord, asString, parseArgs } from "./utils";

type ACPSessionDescriptor = { displayName: string; prompt: string };

export const ACPContext = createContext<
	ReadonlyMap<string, ACPSessionDescriptor>
>(new Map());

/** Recovers session labels from tools already present in the parent chat. */
export function buildACPSessionDescriptors(tools: readonly MergedTool[]) {
	const sessions = new Map<string, ACPSessionDescriptor>();
	for (const tool of tools) {
		if (!isACPToolName(tool.name)) continue;
		const input = parseArgs(tool.args);
		const output = parseArgs(tool.result);
		const outputs =
			tool.name === ACPToolNames.ListAgents && Array.isArray(output?.agents)
				? output.agents.map(asRecord)
				: [output];
		for (const value of outputs) {
			const sessionId =
				asString(value?.session_id) || asString(input?.session_id);
			if (!sessionId) continue;
			const previous = sessions.get(sessionId);
			sessions.set(sessionId, {
				displayName:
					asString(value?.harness_display_name) || previous?.displayName || "",
				prompt:
					(tool.name === ACPToolNames.SpawnAgent
						? asString(input?.prompt)
						: "") ||
					previous?.prompt ||
					"",
			});
		}
	}
	return sessions;
}
