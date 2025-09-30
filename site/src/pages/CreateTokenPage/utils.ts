import type {
	APIAllowListTarget,
	APIKeyScope,
	ScopeCatalog,
} from "api/typesGenerated";

export const NANO_HOUR = 3600000000000;

export type ScopeSelectionMode = "composite" | "low_level";

export interface CreateTokenData {
	name: string;
	lifetime: number;
	scopeMode: ScopeSelectionMode;
	compositeScopes: string[];
	lowLevelScopes: string[];
	allowList: string[];
}

export const buildCompositeExpansionMap = (catalog?: ScopeCatalog) => {
	const map = new Map<string, string[]>();
	if (!catalog) {
		return map;
	}
	for (const composite of catalog.composites ?? []) {
		map.set(composite.name, [...(composite.expands_to ?? [])]);
	}
	return map;
};

export const expandCompositeScopes = (
	selected: readonly string[],
	expansionMap: Map<string, string[]>,
) => {
	const expanded = new Set<string>();
	for (const scope of selected) {
		const matches = expansionMap.get(scope) ?? [];
		for (const match of matches) {
			expanded.add(match);
		}
	}
	return Array.from(expanded).sort((a, b) => a.localeCompare(b));
};

export const sortScopes = (scopes: readonly string[]) => {
	return [...scopes].sort((a, b) => a.localeCompare(b));
};

export const buildRequestScopes = (
	selectedComposites: string[],
	selectedLowLevel: string[],
): APIKeyScope[] => {
	const next = new Set<string>();
	selectedComposites.forEach((scope) => next.add(scope));
	selectedLowLevel.forEach((scope) => next.add(scope));
	return Array.from(next) as APIKeyScope[];
};

export const serializeAllowList = (
	entries: string[],
): APIAllowListTarget[] | undefined => {
	if (entries.length === 0) {
		return undefined;
	}
	const uniqueEntries = Array.from(new Set(entries));
	const targets: APIAllowListTarget[] = [];
	const seen = new Set<string>();
	for (const entry of uniqueEntries) {
		const trimmed = entry.trim();
		if (trimmed === "") {
			continue;
		}
		const [typeRaw, idRaw] = trimmed.split(":", 2);
		const type = typeRaw && typeRaw !== "" ? typeRaw : "*";
		const id = idRaw && idRaw !== "" ? idRaw : "*";
		const key = `${type}:${id}`;
		if (seen.has(key)) {
			continue;
		}
		seen.add(key);
		targets.push({
			type: type as APIAllowListTarget["type"],
			id: id as APIAllowListTarget["id"],
		});
	}
	return targets.length > 0 ? targets : undefined;
};

export const allowListTargetsToStrings = (
	targets?: readonly APIAllowListTarget[],
): string[] => {
	if (!targets || targets.length === 0) {
		return [];
	}
	return targets.map((target) => `${target.type}:${target.id}`);
};

export interface LifetimeDay {
	label: string;
	value: number | string;
}

export const lifetimeDayPresets: LifetimeDay[] = [
	{
		label: "7 days",
		value: 7,
	},
	{
		label: "30 days",
		value: 30,
	},
	{
		label: "60 days",
		value: 60,
	},
	{
		label: "90 days",
		value: 90,
	},
];

export const customLifetimeDay: LifetimeDay = {
	label: "Custom",
	value: "custom",
};

export const filterByMaxTokenLifetime = (
	maxTokenLifetime?: number,
): LifetimeDay[] => {
	// if maxTokenLifetime hasn't been set, return the full array of options
	if (!maxTokenLifetime) {
		return lifetimeDayPresets;
	}

	// otherwise only return options that are less than or equal to the max lifetime
	return lifetimeDayPresets.filter(
		(lifetime) => Number(lifetime.value) <= maxTokenLifetime / NANO_HOUR / 24,
	);
};

export const determineDefaultLtValue = (
	maxTokenLifetime?: number,
): string | number => {
	const filteredArr = filterByMaxTokenLifetime(maxTokenLifetime);

	// default to a lifetime of 30 days if within the maxTokenLifetime
	const thirtyDayDefault = filteredArr.find((lt) => lt.value === 30);
	if (thirtyDayDefault) {
		return thirtyDayDefault.value;
	}

	// otherwise default to the first preset option
	if (filteredArr[0]) {
		return filteredArr[0].value;
	}

	// if no preset options are within the maxTokenLifetime, default to "custom"
	return "custom";
};
