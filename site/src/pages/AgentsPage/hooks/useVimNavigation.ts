import { useSyncExternalStore } from "react";
import {
	getDefaultVimModifier,
	isVimModifier,
	type VimModifier,
} from "../utils/keyboardShortcuts";

export const VIM_NAVIGATION_STORAGE_KEY = "agents.vim-navigation";
export const VIM_NAVIGATION_MODIFIER_STORAGE_KEY =
	"agents.vim-navigation-modifier";

// In-tab subscribers keyed by storage key. The native "storage" event
// only fires cross-tab, so `writeKey` notifies same-tab subscribers
// directly.
const listenersByKey = new Map<string, Set<() => void>>();

function subscribeToKey(key: string, callback: () => void): () => void {
	let listeners = listenersByKey.get(key);
	if (!listeners) {
		listeners = new Set();
		listenersByKey.set(key, listeners);
	}
	listeners.add(callback);

	const onStorage = (e: StorageEvent) => {
		if (e.key === key) {
			callback();
		}
	};
	window.addEventListener("storage", onStorage);

	return () => {
		listeners.delete(callback);
		window.removeEventListener("storage", onStorage);
	};
}

function writeKey(key: string, value: string) {
	localStorage.setItem(key, value);
	for (const fn of listenersByKey.get(key) ?? []) {
		fn();
	}
}

const subscribeEnabled = (callback: () => void) =>
	subscribeToKey(VIM_NAVIGATION_STORAGE_KEY, callback);

const getEnabledSnapshot = (): boolean =>
	localStorage.getItem(VIM_NAVIGATION_STORAGE_KEY) === "true";

/**
 * Reactive hook for the stored vim-style chat navigation preference.
 * This is the raw user setting; it does not account for the
 * deployment experiment.
 */
export function useVimNavigationSetting(): [boolean, (v: boolean) => void] {
	const enabled = useSyncExternalStore(subscribeEnabled, getEnabledSnapshot);

	const setEnabled = (value: boolean) => {
		writeKey(VIM_NAVIGATION_STORAGE_KEY, String(value));
	};

	return [enabled, setEnabled];
}

const subscribeModifier = (callback: () => void) =>
	subscribeToKey(VIM_NAVIGATION_MODIFIER_STORAGE_KEY, callback);

const getModifierSnapshot = (): VimModifier => {
	const stored = localStorage.getItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY);
	return isVimModifier(stored) ? stored : getDefaultVimModifier();
};

/**
 * Reactive hook for the modifier key used by vim-style chat
 * navigation. An unset or unrecognized stored value resolves to
 * the platform default.
 */
export function useVimNavigationModifier(): [
	VimModifier,
	(v: VimModifier) => void,
] {
	const modifier = useSyncExternalStore(subscribeModifier, getModifierSnapshot);

	const setModifier = (value: VimModifier) => {
		writeKey(VIM_NAVIGATION_MODIFIER_STORAGE_KEY, value);
	};

	return [modifier, setModifier];
}
