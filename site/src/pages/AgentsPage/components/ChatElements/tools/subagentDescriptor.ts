import { asRecord, asString } from "../runtimeTypeUtils";

export type SubagentAction = "spawn" | "wait" | "message" | "close";
export type SubagentVariant = "general" | "explore" | "computer_use";
export type SubagentIconKind = "bot" | "monitor";

export type SubagentDescriptor = {
	action: SubagentAction;
	variant: SubagentVariant;
	iconKind: SubagentIconKind;
	title: string;
	fallbackTitle: string;
	supportsDesktopAffordance: boolean;
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
	spawn_subagent: "spawn",
	wait_agent: "wait",
	message_agent: "message",
	close_agent: "close",
};

const variantBySpawnToolName: Record<string, SubagentVariant> = {
	spawn_agent: "general",
	spawn_explore_agent: "explore",
	spawn_computer_use_agent: "computer_use",
};

const parseRecord = (value: unknown): Record<string, unknown> | null => {
	if (typeof value === "string") {
		try {
			return asRecord(JSON.parse(value));
		} catch {
			return null;
		}
	}
	return asRecord(value);
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
	const resultRecord = parseRecord(result);
	const argsRecord = parseRecord(args);
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
	const resultRecord = parseRecord(result);
	const argsRecord = parseRecord(args);
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

	const resultRecord = parseRecord(result);
	const argsRecord = parseRecord(args);
	const variant =
		getVariantFromName(name) ??
		normalizeSubagentVariant(resultRecord?.subagent_type) ??
		normalizeSubagentVariant(argsRecord?.subagent_type) ??
		inferredVariant ??
		"general";
	const catalogEntry = subagentCatalog[variant];
	const title =
		getProvidedSubagentTitle({ args: argsRecord, result: resultRecord }) ||
		catalogEntry.fallbackTitle;

	return {
		action,
		variant,
		iconKind: catalogEntry.iconKind,
		title,
		fallbackTitle: catalogEntry.fallbackTitle,
		supportsDesktopAffordance: catalogEntry.supportsDesktopAffordance,
	};
};
