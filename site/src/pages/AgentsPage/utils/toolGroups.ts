import type { ChatToolGroup } from "#/api/typesGenerated";

/** The tool groups a chat owner can switch from the composer menu. */
type SwitchableToolGroup = Extract<
	ChatToolGroup,
	"workspace" | "subagents" | "skills" | "memory"
>;

export type ToolGroupValues = Record<SwitchableToolGroup, boolean>;

/** Stored deviations from the defaults, as the API sends them. */
type StoredToolGroups = Partial<Record<ChatToolGroup, boolean>>;

type ToolGroupCatalogEntry = {
	id: SwitchableToolGroup;
	label: string;
	description: string;
	defaultOn: boolean;
};

// Hand-copied from codersdk/chattoolgroups.go for the prototype.
export const switchableToolGroups: readonly ToolGroupCatalogEntry[] = [
	{
		id: "workspace",
		label: "Workspace tools",
		description: "Files, commands and workspaces",
		defaultOn: true,
	},
	{
		id: "subagents",
		label: "Sub-agents",
		description: "Start and manage helper agents",
		defaultOn: true,
	},
	{
		id: "skills",
		label: "Skills",
		description: "Read skills",
		defaultOn: true,
	},
	{
		id: "memory",
		label: "Memory",
		description: "Save and read project memory",
		defaultOn: true,
	},
];

const defaultOf = (group: SwitchableToolGroup): boolean => {
	const entry = switchableToolGroups.find((g) => g.id === group);
	if (!entry) {
		throw new Error(`unknown tool group: ${group}`);
	}
	return entry.defaultOn;
};

export const defaultToolGroupValues = (): ToolGroupValues => ({
	workspace: defaultOf("workspace"),
	subagents: defaultOf("subagents"),
	skills: defaultOf("skills"),
	memory: defaultOf("memory"),
});

/** Merges the stored choices of a chat over the defaults. */
export const getEffectiveToolGroups = (
	stored: StoredToolGroups | undefined,
): ToolGroupValues => {
	const values = defaultToolGroupValues();
	for (const { id } of switchableToolGroups) {
		const choice = stored?.[id];
		if (typeof choice === "boolean") {
			values[id] = choice;
		}
	}
	return values;
};

/**
 * Returns the PATCH value for one change: null resets the group to its
 * default, so the chat stores only real deviations.
 */
export const toolGroupPatchValue = (
	group: SwitchableToolGroup,
	enabled: boolean,
): boolean | null => (enabled === defaultOf(group) ? null : enabled);

/** Returns only the groups that differ from the defaults, for create. */
export const changedToolGroups = (
	values: ToolGroupValues,
): StoredToolGroups | undefined => {
	const changed: StoredToolGroups = {};
	let hasChange = false;
	for (const { id, defaultOn } of switchableToolGroups) {
		if (values[id] !== defaultOn) {
			changed[id] = values[id];
			hasChange = true;
		}
	}
	return hasChange ? changed : undefined;
};

/** State for the Tools section of the composer + menu. */
export type ToolSetsMenuState = {
	values: ToolGroupValues;
	onChange: (group: SwitchableToolGroup, enabled: boolean) => void;
	// When set, the tool group and MCP switches are disabled and this hint
	// explains why.
	lockedHint?: string;
};
