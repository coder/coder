import { useCallback, useSyncExternalStore } from "react";
import { useChatRuntimeContext } from "./ChatRuntimeProvider";

/** @public Preference accessors scoped to the shared chat provider. */
export type UseChatPreferencesResult = {
	get: <T>(key: string, fallback: T) => T;
	set: <T>(key: string, value: T) => void;
	selectedModel: string | undefined;
	setSelectedModel: (model: string | undefined) => void;
};

/** @public Reads and updates shared chat preferences. */
export const useChatPreferences = (): UseChatPreferencesResult => {
	const { preferenceStore } = useChatRuntimeContext();

	const subscribeToSelectedModel = useCallback(
		(listener: () => void) =>
			preferenceStore.subscribe("selectedModel", listener),
		[preferenceStore],
	);
	const getSelectedModelSnapshot = useCallback(
		() => preferenceStore.get<string | undefined>("selectedModel", undefined),
		[preferenceStore],
	);

	const selectedModel = useSyncExternalStore(
		subscribeToSelectedModel,
		getSelectedModelSnapshot,
		getSelectedModelSnapshot,
	);

	const get = useCallback(
		<T>(key: string, fallback: T): T => preferenceStore.get(key, fallback),
		[preferenceStore],
	);
	const set = useCallback(
		<T>(key: string, value: T): void => {
			preferenceStore.set(key, value);
		},
		[preferenceStore],
	);
	const setSelectedModel = useCallback(
		(model: string | undefined): void => {
			preferenceStore.set("selectedModel", model);
		},
		[preferenceStore],
	);

	return { get, set, selectedModel, setSelectedModel };
};
