import { useEffect, useRef, useSyncExternalStore } from "react";
import { toast } from "sonner";

/**
 * Easter egg. Typing "iddqd" anywhere in the Agents UI (including while the
 * composer has focus) swaps the Thinking indicator for a small slot machine;
 * typing it again turns it off. The toggle is persisted in localStorage so it
 * survives reloads.
 */
export const slotMachineStorageKey = "agents.slot-machine";
export const slotMachineKeySequence = "iddqd";

/**
 * Reports whether a keydown event contributes a printable character that the
 * sequence buffer should record. Ctrl, Meta, and Alt chords and keystrokes
 * that are part of an IME composition are skipped. Shift is allowed because
 * it still produces a printable key.
 */
export function isPrintableKeyEvent(event: {
	key: string;
	ctrlKey: boolean;
	metaKey: boolean;
	altKey: boolean;
	isComposing?: boolean;
}): boolean {
	if (event.ctrlKey || event.metaKey || event.altKey || event.isComposing) {
		return false;
	}
	// Named keys such as "Enter" or "ArrowLeft" are longer than one code point.
	return [...event.key].length === 1;
}

/**
 * Appends a key to the buffer, lowercased, and keeps only the trailing
 * `maxLength` characters.
 */
export function appendToKeyBuffer(
	buffer: string,
	key: string,
	maxLength: number,
): string {
	return (buffer + key.toLowerCase()).slice(-maxLength);
}

/** Reports whether the buffer ends with the sequence. */
export function matchesKeySequence(buffer: string, sequence: string): boolean {
	return sequence.length > 0 && buffer.endsWith(sequence);
}

// Used only when localStorage is unreadable or unwritable, so the toggle
// still works for the current page in that case.
let fallbackEnabled = false;
const listeners = new Set<() => void>();

function getSlotMachineEnabled(): boolean {
	try {
		return localStorage.getItem(slotMachineStorageKey) === "true";
	} catch {
		return fallbackEnabled;
	}
}

function notifyListeners(): void {
	for (const listener of listeners) {
		listener();
	}
}

function setSlotMachineEnabled(enabled: boolean): void {
	fallbackEnabled = enabled;
	try {
		localStorage.setItem(slotMachineStorageKey, String(enabled));
	} catch {
		// Storage can be unavailable (private mode, quota); fallbackEnabled
		// carries the value instead.
	}
	notifyListeners();
}

function handleStorageEvent(event: StorageEvent): void {
	// A null key means the whole store was cleared.
	if (event.key === null || event.key === slotMachineStorageKey) {
		notifyListeners();
	}
}

function subscribeSlotMachineEnabled(listener: () => void): () => void {
	listeners.add(listener);
	window.addEventListener("storage", handleStorageEvent);
	return () => {
		listeners.delete(listener);
		window.removeEventListener("storage", handleStorageEvent);
	};
}

/** Whether the slot machine indicator is currently enabled. */
export function useSlotMachineEnabled(): boolean {
	return useSyncExternalStore(
		subscribeSlotMachineEnabled,
		getSlotMachineEnabled,
	);
}

/**
 * Installs a document-level keydown listener that watches for the toggle
 * sequence. Events are only observed, never prevented or stopped, so typed
 * characters still reach whichever element has focus.
 */
export function useSlotMachineEasterEggListener(): void {
	const bufferRef = useRef("");

	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			if (!isPrintableKeyEvent(event)) {
				return;
			}
			bufferRef.current = appendToKeyBuffer(
				bufferRef.current,
				event.key,
				slotMachineKeySequence.length,
			);
			if (!matchesKeySequence(bufferRef.current, slotMachineKeySequence)) {
				return;
			}
			bufferRef.current = "";
			const enabled = !getSlotMachineEnabled();
			setSlotMachineEnabled(enabled);
			toast(enabled ? "Slot machine mode on" : "Slot machine mode off");
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, []);
}
