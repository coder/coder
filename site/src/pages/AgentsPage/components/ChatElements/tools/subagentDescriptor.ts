import { asString } from "../runtimeTypeUtils";
import { parseArgs } from "./utils";

export type SubagentAction =
	| "spawn"
	| "wait"
	| "message"
	| "interrupt"
	| "list";
export type SubagentVariant = "general" | "explore" | "computer_use";
export type SubagentIconKind = "bot" | "monitor";

export type SubagentDescriptor = {
	action: SubagentAction;
	variant: SubagentVariant;
	iconKind: SubagentIconKind;
	title: string;
	fallbackTitle: string;
	supportsDesktopAffordance: boolean;
	/** Set only when the spawn args explicitly selected a model. */
	modelConfigId?: string;
	/** Set only when the spawn args explicitly pinned a reasoning effort. */
	reasoningEffort?: string;
};

const subagentCatalog: Record<
	SubagentVariant,
	{
		fallbackTitle: string;
		iconKind: SubagentIconKind;
		supportsDesktopAffordance: boolean;
	}
> = {
	general: {
		fallbackTitle: "sub-agent",
		iconKind: "bot",
		supportsDesktopAffordance: false,
	},
	explore: {
		fallbackTitle: "Explore agent",
		iconKind: "bot",
		supportsDesktopAffordance: false,
	},
	computer_use: {
		fallbackTitle: "Computer use sub-agent",
		iconKind: "monitor",
		supportsDesktopAffordance: true,
	},
};

const actionByToolName: Record<string, SubagentAction> = {
	spawn_agent: "spawn",
	spawn_explore_agent: "spawn",
	spawn_computer_use_agent: "spawn",
	// Legacy persisted tool name from the pre-rename unified contract.
	spawn_subagent: "spawn",
	wait_agent: "wait",
	message_agent: "message",
	// Legacy persisted tool name kept so old chat histories still render.
	close_agent: "interrupt",
	interrupt_agent: "interrupt",
	// list_agents is a subagent tool but renders through
	// ListAgentsRenderer, not SubagentRenderer. The "list" action
	// exists for isSubagentToolName classification and ToolIcon
	// dispatch, not for the SubagentRenderer label machinery.
	list_agents: "list",
};

const variantBySpawnToolName: Record<string, SubagentVariant> = {
	spawn_explore_agent: "explore",
	spawn_computer_use_agent: "computer_use",
};

const normalizeSubagentVariant = (
	value: unknown,
): SubagentVariant | undefined => {
	switch (asString(value).trim().toLowerCase()) {
		case "general":
			return "general";
		case "explore":
			return "explore";
		case "computer_use":
			return "computer_use";
		default:
			return undefined;
	}
};

const getSubagentAction = (name: string): SubagentAction | undefined =>
	actionByToolName[name];

const getVariantFromName = (name: string): SubagentVariant | undefined =>
	variantBySpawnToolName[name as keyof typeof variantBySpawnToolName];

export const isSubagentToolName = (name: string): boolean =>
	getSubagentAction(name) !== undefined;

export const getSubagentChatId = ({
	args,
	result,
}: {
	args?: unknown;
	result?: unknown;
}): string => {
	const resultRecord = parseArgs(result);
	const argsRecord = parseArgs(args);
	return (
		asString(resultRecord?.chat_id).trim() ||
		asString(argsRecord?.chat_id).trim()
	);
};

export const getProvidedSubagentTitle = ({
	args,
	result,
}: {
	args?: unknown;
	result?: unknown;
}): string => {
	const resultRecord = parseArgs(result);
	const argsRecord = parseArgs(args);
	return (
		asString(resultRecord?.title).trim() || asString(argsRecord?.title).trim()
	);
};

export const getSubagentDescriptor = ({
	name,
	args,
	result,
	inferredVariant,
}: {
	name: string;
	args?: unknown;
	result?: unknown;
	inferredVariant?: SubagentVariant;
}): SubagentDescriptor | null => {
	const action = getSubagentAction(name);
	if (!action) {
		return null;
	}

	const resultRecord = parseArgs(result);
	const argsRecord = parseArgs(args);
	const variant =
		normalizeSubagentVariant(resultRecord?.type) ??
		normalizeSubagentVariant(argsRecord?.type) ??
		// Legacy persisted payloads used subagent_type.
		normalizeSubagentVariant(resultRecord?.subagent_type) ??
		normalizeSubagentVariant(argsRecord?.subagent_type) ??
		getVariantFromName(name) ??
		inferredVariant ??
		"general";
	const catalogEntry = subagentCatalog[variant];
	const title =
		getProvidedSubagentTitle({ args: argsRecord, result: resultRecord }) ||
		catalogEntry.fallbackTitle;
	const modelConfigId =
		action === "spawn" ? asString(argsRecord?.model_config_id).trim() : "";
	const reasoningEffort =
		action === "spawn"
			? asString(argsRecord?.reasoning_effort).trim().toLowerCase()
			: "";

	return {
		action,
		variant,
		iconKind: catalogEntry.iconKind,
		title,
		fallbackTitle: catalogEntry.fallbackTitle,
		supportsDesktopAffordance: catalogEntry.supportsDesktopAffordance,
		...(modelConfigId ? { modelConfigId } : {}),
		...(reasoningEffort ? { reasoningEffort } : {}),
	};
};
