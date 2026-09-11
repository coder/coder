import {
	VIM_NAVIGATION_MODIFIER_STORAGE_KEY,
	VIM_NAVIGATION_STORAGE_KEY,
} from "#/pages/AgentsPage/hooks/useVimNavigation";
import type { VimModifier } from "#/pages/AgentsPage/utils/keyboardShortcuts";

/**
 * Story `beforeEach` that turns the vim navigation preference on with
 * the given modifier and clears both keys on cleanup.
 */
export const withVimNavigationPreference = (modifier: VimModifier) => () => {
	localStorage.setItem(VIM_NAVIGATION_STORAGE_KEY, "true");
	localStorage.setItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY, modifier);
	return () => {
		localStorage.removeItem(VIM_NAVIGATION_STORAGE_KEY);
		localStorage.removeItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY);
	};
};
