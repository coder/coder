import type { SerpentGroup, SerpentOption } from "api/typesGenerated";
import { useMemo } from "react";

/**
 * Looks up deployment options by their CLI flag (e.g., "access-url").
 * Using flags instead of display names ensures lookups are resilient to
 * UI text changes.
 *
 * @throws Error if any flag is not found in the options array
 */
const deploymentOptions = (
	options: SerpentOption[],
	...flags: string[]
): SerpentOption[] => {
	const found: SerpentOption[] = [];
	for (const flag of flags) {
		const option = options.find((o) => o.flag === flag);
		if (option) {
			found.push(option);
		} else {
			throw new Error(`Deployment option with flag "${flag}" not found`);
		}
	}
	return found;
};

export const useDeploymentOptions = (
	options: SerpentOption[],
	...flags: string[]
): SerpentOption[] => {
	return useMemo(() => deploymentOptions(options, ...flags), [options, flags]);
};

export const deploymentGroupHasParent = (
	group: SerpentGroup | undefined,
	parent: string,
): boolean => {
	if (!group) {
		return false;
	}
	if (group.name === parent) {
		return true;
	}
	if (group.parent) {
		return deploymentGroupHasParent(group.parent, parent);
	}
	return false;
};
