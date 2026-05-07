import { useSyncExternalStore } from "react";

export type AgentChatSendShortcut = "enter" | "modifier-enter";

const storageKeyPrefix = "agents.chat-send-shortcut.";
const modifierEnterValue: AgentChatSendShortcut = "modifier-enter";
const defaultShortcut: AgentChatSendShortcut = "enter";

const listeners = new Set<() => void>();
let isListeningForStorage = false;

export function getAgentChatSendShortcutStorageKey(userId: string): string {
	return `${storageKeyPrefix}${userId}`;
}

function readShortcut(storageKey: string): AgentChatSendShortcut {
	return localStorage.getItem(storageKey) === modifierEnterValue
		? modifierEnterValue
		: defaultShortcut;
}

function notify(): void {
	for (const callback of listeners) {
		callback();
	}
}

function handleStorage(event: StorageEvent): void {
	if (event.key === null || event.key.startsWith(storageKeyPrefix)) {
		notify();
	}
}

function subscribe(callback: () => void): () => void {
	listeners.add(callback);
	if (!isListeningForStorage) {
		addEventListener("storage", handleStorage);
		isListeningForStorage = true;
	}

	return () => {
		listeners.delete(callback);
		if (listeners.size === 0 && isListeningForStorage) {
			removeEventListener("storage", handleStorage);
			isListeningForStorage = false;
		}
	};
}

export function useAgentChatSendShortcut(
	userId: string,
): [AgentChatSendShortcut, (value: AgentChatSendShortcut) => void] {
	const storageKey = getAgentChatSendShortcutStorageKey(userId);
	const shortcut = useSyncExternalStore(subscribe, () =>
		readShortcut(storageKey),
	);

	const setShortcut = (value: AgentChatSendShortcut) => {
		if (value === defaultShortcut) {
			localStorage.removeItem(storageKey);
		} else {
			localStorage.setItem(storageKey, value);
		}
		notify();
	};

	return [shortcut, setShortcut];
}
