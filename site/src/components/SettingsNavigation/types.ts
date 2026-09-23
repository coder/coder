import type { MouseEvent as ReactMouseEvent } from "react";
import { matchPath } from "react-router";

export type SettingsNavigationItem = {
	id: string;
	label: string;
	href: string;
	end?: boolean;
	matchPatterns?: readonly string[];
};

export type SettingsNavigationSection = {
	id: string;
	label?: string;
	items: readonly SettingsNavigationItem[];
};

export type LocatedSettingsNavigationItem = {
	section: SettingsNavigationSection;
	item: SettingsNavigationItem;
};

export const shouldUseOptimisticSelection = (
	event: Pick<
		ReactMouseEvent<HTMLAnchorElement>,
		"altKey" | "button" | "ctrlKey" | "metaKey" | "shiftKey"
	>,
): boolean =>
	event.button === 0 &&
	!event.altKey &&
	!event.ctrlKey &&
	!event.metaKey &&
	!event.shiftKey;

const itemMatchesPath = (
	item: SettingsNavigationItem,
	pathname: string,
): boolean => {
	if (item.matchPatterns) {
		return item.matchPatterns.some((pattern) => matchPath(pattern, pathname));
	}
	return Boolean(
		matchPath({ path: item.href, end: item.end ?? false }, pathname),
	);
};

export const findActiveSettingsNavigationItem = (
	sections: readonly SettingsNavigationSection[],
	pathname: string,
): LocatedSettingsNavigationItem | undefined => {
	for (const section of sections) {
		for (const item of section.items) {
			if (itemMatchesPath(item, pathname)) {
				return { section, item };
			}
		}
	}
	return undefined;
};
